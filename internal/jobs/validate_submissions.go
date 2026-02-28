package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/scraper"
)

const (
	JobKindValidateSubmissionsScheduler = "validate_submissions_scheduler"
	JobKindValidateSubmissionsBatch     = "validate_submissions_batch"

	// validateBatchSize is the maximum number of submissions processed per batch run.
	validateBatchSize = 20

	// validateHeadTimeout is the per-URL timeout for HEAD reachability checks.
	validateHeadTimeout = 5 * time.Second

	// validateSchedulerInterval is how often the scheduler checks for pending work.
	validateSchedulerInterval = 5 * time.Minute

	// validateUserAgent is the User-Agent used for HEAD and robots.txt requests.
	validateUserAgent = "Togather"
)

// ---------------------------------------------------------------------------
// Args types
// ---------------------------------------------------------------------------

// ValidateSubmissionsSchedulerArgs is the periodic job that wakes up every 5
// minutes and enqueues a batch-validation job if there are pending rows.
type ValidateSubmissionsSchedulerArgs struct{}

func (ValidateSubmissionsSchedulerArgs) Kind() string {
	return JobKindValidateSubmissionsScheduler
}

// ValidateSubmissionsBatchArgs is the on-demand job that processes up to
// validateBatchSize pending_validation rows and optionally re-enqueues itself.
type ValidateSubmissionsBatchArgs struct{}

func (ValidateSubmissionsBatchArgs) Kind() string {
	return JobKindValidateSubmissionsBatch
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsSchedulerWorker
// ---------------------------------------------------------------------------

// ValidateSubmissionsSchedulerWorker fires every 5 minutes. If there are any
// rows in pending_validation status it enqueues a ValidateSubmissionsBatchArgs
// job with UniqueOpts so that only one batch job runs at a time.
type ValidateSubmissionsSchedulerWorker struct {
	river.WorkerDefaults[ValidateSubmissionsSchedulerArgs]
	Repo   domainScraper.SubmissionRepository
	Logger *slog.Logger
}

func (w ValidateSubmissionsSchedulerWorker) Work(ctx context.Context, job *river.Job[ValidateSubmissionsSchedulerArgs]) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if w.Repo == nil {
		return fmt.Errorf("validate_submissions_scheduler: repo not configured")
	}

	count, err := w.Repo.CountPendingValidation(ctx)
	if err != nil {
		return fmt.Errorf("validate_submissions_scheduler: count pending: %w", err)
	}

	if count == 0 {
		logger.DebugContext(ctx, "validate_submissions_scheduler: no pending rows, exiting")
		return nil
	}

	logger.InfoContext(ctx, "validate_submissions_scheduler: enqueuing batch job", "pending_count", count)

	riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("validate_submissions_scheduler: river client unavailable: %w", err)
	}

	// UniqueOpts: unique by Kind within the "available" + "running" + "scheduled"
	// + "retryable" states — prevents double-enqueue while a batch is in flight.
	_, err = riverClient.Insert(ctx, ValidateSubmissionsBatchArgs{}, &river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs:   false,
			ByPeriod: 5 * time.Minute,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("validate_submissions_scheduler: insert batch job: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsBatchWorker
// ---------------------------------------------------------------------------

// ValidateSubmissionsBatchWorker processes up to validateBatchSize rows that
// are in pending_validation status. For each URL it performs:
//  1. A HEAD request (5-second timeout, redirects disabled).
//  2. A robots.txt check via scraper.RobotsAllowed.
//
// After processing the batch, if more pending rows remain the worker re-enqueues
// itself immediately (job chaining). Max attempts is 1 — the scheduler handles
// re-enqueueing on the next tick if something goes wrong.
type ValidateSubmissionsBatchWorker struct {
	river.WorkerDefaults[ValidateSubmissionsBatchArgs]
	Repo   domainScraper.SubmissionRepository
	Logger *slog.Logger

	// HTTPClient overrides the default SSRF-blocking client. Set in tests only.
	HTTPClient *http.Client
}

// MaxAttempts returns 1 so that the job is not retried on failure; the scheduler
// re-enqueues on the next 5-minute tick.
func (ValidateSubmissionsBatchWorker) MaxAttempts() int { return 1 }

func (w ValidateSubmissionsBatchWorker) Work(ctx context.Context, job *river.Job[ValidateSubmissionsBatchArgs]) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if w.Repo == nil {
		return fmt.Errorf("validate_submissions_batch: repo not configured")
	}

	rows, err := w.Repo.ListPendingValidation(ctx, validateBatchSize)
	if err != nil {
		return fmt.Errorf("validate_submissions_batch: list pending: %w", err)
	}

	logger.InfoContext(ctx, "validate_submissions_batch: processing rows", "count", len(rows))

	// Create a single HTTP client for the whole batch to enable TCP connection
	// reuse across HEAD requests to the same host (srv-b22wi).
	// The transport blocks connections to private/loopback/link-local addresses
	// to prevent SSRF via submitted URLs (srv-exv8k).
	// In tests, HTTPClient may be injected to bypass SSRF blocking.
	batchClient := w.HTTPClient
	if batchClient == nil {
		batchClient = &http.Client{
			Timeout:   validateHeadTimeout,
			Transport: newSSRFBlockingTransport(),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	for _, sub := range rows {
		if err := w.validateOne(ctx, logger, sub, batchClient); err != nil {
			// Log but don't abort the batch — process remaining rows.
			logger.WarnContext(ctx, "validate_submissions_batch: error validating URL",
				"id", sub.ID,
				"url", sub.URL,
				"error", err,
			)
		}
	}

	// Job chaining: re-enqueue if more rows remain.
	remaining, err := w.Repo.CountPendingValidation(ctx)
	if err != nil {
		// Non-fatal: log and exit; scheduler will re-enqueue.
		logger.WarnContext(ctx, "validate_submissions_batch: count after batch failed", "error", err)
		return nil
	}

	if remaining > 0 {
		riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx)
		if err != nil {
			logger.WarnContext(ctx, "validate_submissions_batch: river client unavailable for chaining", "error", err)
			return nil
		}
		if _, err = riverClient.Insert(ctx, ValidateSubmissionsBatchArgs{}, &river.InsertOpts{
			MaxAttempts: 1,
			// Mirror the scheduler's UniqueOpts so a chained re-enqueue cannot
			// produce a second concurrent batch if the scheduler fires at the
			// same moment (srv-ltv8c).
			UniqueOpts: river.UniqueOpts{
				ByArgs:   false,
				ByPeriod: 5 * time.Minute,
				ByState: []rivertype.JobState{
					rivertype.JobStateAvailable,
					rivertype.JobStateRunning,
					rivertype.JobStateScheduled,
					rivertype.JobStatePending,
					rivertype.JobStateRetryable,
				},
			},
		}); err != nil {
			logger.WarnContext(ctx, "validate_submissions_batch: re-enqueue failed", "error", err)
		} else {
			logger.InfoContext(ctx, "validate_submissions_batch: re-enqueued for remaining rows", "remaining", remaining)
		}
	}

	return nil
}

// validateOne performs the HEAD + robots.txt check for a single submission and
// updates its status in the repository. client is shared across the batch for
// TCP connection reuse.
func (w ValidateSubmissionsBatchWorker) validateOne(ctx context.Context, logger *slog.Logger, sub *domainScraper.Submission, client *http.Client) error {
	// HEAD request.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sub.URL, nil)
	if err != nil {
		reason := fmt.Sprintf("invalid URL: %v", err)
		return w.Repo.UpdateStatus(ctx, sub.ID, "rejected", &reason, nil)
	}

	resp, err := client.Do(req)
	if err != nil {
		reason := "HEAD request timed out"
		if !isTimeoutError(err) {
			reason = fmt.Sprintf("HEAD request failed: %v", err)
		}
		logger.DebugContext(ctx, "validate_submissions_batch: HEAD failed",
			"id", sub.ID, "url", sub.URL, "reason", reason)
		return w.Repo.UpdateStatus(ctx, sub.ID, "rejected", &reason, nil)
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		reason := fmt.Sprintf("HEAD request returned %d", resp.StatusCode)
		logger.DebugContext(ctx, "validate_submissions_batch: HEAD non-2xx/3xx",
			"id", sub.ID, "url", sub.URL, "status", resp.StatusCode)
		return w.Repo.UpdateStatus(ctx, sub.ID, "rejected", &reason, nil)
	}

	// robots.txt check — reuse existing scraper function.
	allowed, err := scraper.RobotsAllowed(ctx, sub.URL, validateUserAgent, client)
	if err != nil {
		// Treat robots fetch errors as "allowed" per spec (conservative).
		logger.DebugContext(ctx, "validate_submissions_batch: robots.txt fetch error, treating as allowed",
			"id", sub.ID, "url", sub.URL, "error", err)
	}

	if !allowed {
		reason := "robots.txt disallows crawling"
		return w.Repo.UpdateStatus(ctx, sub.ID, "rejected", &reason, nil)
	}

	now := time.Now()
	logger.DebugContext(ctx, "validate_submissions_batch: URL validated", "id", sub.ID, "url", sub.URL)
	return w.Repo.UpdateStatus(ctx, sub.ID, "pending", nil, &now)
}

// isTimeoutError returns true when err is (or wraps) a timeout/deadline error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "context deadline")
}

// newSSRFBlockingTransport returns an http.Transport whose DialContext resolves
// each hostname and rejects connections to private, loopback, or link-local
// addresses before any data is sent.  This prevents SSRF via submitted URLs
// that target RFC-1918 ranges, loopback, or cloud metadata endpoints
// (e.g. 169.254.169.254).
//
// Blocked ranges:
//   - 127.0.0.0/8    — IPv4 loopback
//   - 10.0.0.0/8     — RFC 1918 private
//   - 172.16.0.0/12  — RFC 1918 private
//   - 192.168.0.0/16 — RFC 1918 private
//   - 169.254.0.0/16 — link-local / cloud metadata
//   - ::1/128         — IPv6 loopback
//   - fc00::/7        — IPv6 ULA
//   - fe80::/10       — IPv6 link-local
func newSSRFBlockingTransport() *http.Transport {
	blockedCIDRs := mustParseCIDRs([]string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	})

	dialer := &net.Dialer{Timeout: validateHeadTimeout}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("SSRF check: parse addr %q: %w", addr, err)
		}

		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("SSRF check: resolve %q: %w", host, err)
		}

		for _, ipStr := range ips {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			for _, blocked := range blockedCIDRs {
				if blocked.Contains(ip) {
					return nil, fmt.Errorf("SSRF check: %s resolves to blocked address %s", host, ip)
				}
			}
		}

		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	return transport
}

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %q: %v", cidr, err))
		}
		nets = append(nets, ipNet)
	}
	return nets
}
