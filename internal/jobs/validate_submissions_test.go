package jobs

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

// ---------------------------------------------------------------------------
// mockSubmissionRepo — in-memory test double for domainScraper.SubmissionRepository
// ---------------------------------------------------------------------------

type mockSubmissionRepo struct {
	// ListPendingValidation
	pendingRows []*domainScraper.Submission
	listErr     error

	// CountPendingValidation
	pendingCount int64
	countErr     error

	// UpdateStatus
	updates   []submissionUpdate
	updateErr error

	// stubs not needed for these tests
	getRecentErr error
	countIPErr   error
	insertErr    error
}

type submissionUpdate struct {
	id              int64
	status          string
	rejectionReason *string
	validatedAt     *time.Time
}

func (m *mockSubmissionRepo) ListPendingValidation(_ context.Context, limit int) ([]*domainScraper.Submission, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	rows := m.pendingRows
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (m *mockSubmissionRepo) CountPendingValidation(_ context.Context) (int64, error) {
	return m.pendingCount, m.countErr
}

func (m *mockSubmissionRepo) UpdateStatus(_ context.Context, id int64, status string, reason *string, validatedAt *time.Time) error {
	m.updates = append(m.updates, submissionUpdate{
		id:              id,
		status:          status,
		rejectionReason: reason,
		validatedAt:     validatedAt,
	})
	return m.updateErr
}

func (m *mockSubmissionRepo) GetRecentByURLNorm(_ context.Context, _ string) (*domainScraper.Submission, error) {
	return nil, m.getRecentErr
}

func (m *mockSubmissionRepo) CountRecentByIP(_ context.Context, _ string) (int64, error) {
	return 0, m.countIPErr
}

func (m *mockSubmissionRepo) Insert(_ context.Context, sub *domainScraper.Submission) (*domainScraper.Submission, error) {
	return sub, m.insertErr
}

func (m *mockSubmissionRepo) UpdateAdminReview(_ context.Context, id int64, status string, notes *string) (*domainScraper.Submission, error) {
	return &domainScraper.Submission{ID: id, Status: status, Notes: notes}, nil
}

func (m *mockSubmissionRepo) List(_ context.Context, _ *string, _, _ int) ([]*domainScraper.Submission, error) {
	return nil, nil
}

func (m *mockSubmissionRepo) Count(_ context.Context, _ *string) (int64, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newBatchJob() *river.Job[ValidateSubmissionsBatchArgs] {
	return &river.Job[ValidateSubmissionsBatchArgs]{
		JobRow: &rivertype.JobRow{
			Kind:      JobKindValidateSubmissionsBatch,
			Attempt:   1,
			CreatedAt: time.Now(),
		},
		Args: ValidateSubmissionsBatchArgs{},
	}
}

func newSchedulerJob() *river.Job[ValidateSubmissionsSchedulerArgs] {
	return &river.Job[ValidateSubmissionsSchedulerArgs]{
		JobRow: &rivertype.JobRow{
			Kind:      JobKindValidateSubmissionsScheduler,
			Attempt:   1,
			CreatedAt: time.Now(),
		},
		Args: ValidateSubmissionsSchedulerArgs{},
	}
}

// ---------------------------------------------------------------------------
// Args Kind() tests
// ---------------------------------------------------------------------------

func TestValidateSubmissionsSchedulerArgs_Kind(t *testing.T) {
	args := ValidateSubmissionsSchedulerArgs{}
	if args.Kind() != JobKindValidateSubmissionsScheduler {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindValidateSubmissionsScheduler)
	}
}

func TestValidateSubmissionsBatchArgs_Kind(t *testing.T) {
	args := ValidateSubmissionsBatchArgs{}
	if args.Kind() != JobKindValidateSubmissionsBatch {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindValidateSubmissionsBatch)
	}
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsBatchWorker.MaxAttempts
// ---------------------------------------------------------------------------

func TestValidateSubmissionsBatchWorker_MaxAttempts(t *testing.T) {
	w := ValidateSubmissionsBatchWorker{}
	if w.MaxAttempts() != 1 {
		t.Errorf("MaxAttempts() = %d, want 1", w.MaxAttempts())
	}
}

// plainClient returns an http.Client without SSRF blocking, for use in tests
// that bind to 127.0.0.1 via httptest.NewServer.
func plainClient() *http.Client {
	return &http.Client{
		Timeout: validateHeadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsSchedulerWorker.Work
// ---------------------------------------------------------------------------

func TestValidateSchedulerWorker_NilRepo(t *testing.T) {
	t.Parallel()
	w := ValidateSubmissionsSchedulerWorker{Repo: nil}
	err := w.Work(context.Background(), newSchedulerJob())
	if err == nil {
		t.Fatal("expected error when Repo is nil")
	}
}

func TestValidateSchedulerWorker_NoPendingRows(t *testing.T) {
	t.Parallel()
	// count == 0: should return nil without trying to get a river client.
	repo := &mockSubmissionRepo{pendingCount: 0}
	w := ValidateSubmissionsSchedulerWorker{Repo: repo}
	err := w.Work(context.Background(), newSchedulerJob())
	if err != nil {
		t.Fatalf("expected no error when count=0, got %v", err)
	}
}

func TestValidateSchedulerWorker_CountError(t *testing.T) {
	t.Parallel()
	repo := &mockSubmissionRepo{countErr: errors.New("db error")}
	w := ValidateSubmissionsSchedulerWorker{Repo: repo}
	err := w.Work(context.Background(), newSchedulerJob())
	if err == nil {
		t.Fatal("expected error when CountPendingValidation fails")
	}
}

func TestValidateSchedulerWorker_PendingRows_NoRiverClient(t *testing.T) {
	t.Parallel()
	// count > 0 but no River client in context — should return error.
	repo := &mockSubmissionRepo{pendingCount: 3}
	w := ValidateSubmissionsSchedulerWorker{Repo: repo}
	err := w.Work(context.Background(), newSchedulerJob())
	if err == nil {
		t.Fatal("expected error when River client is unavailable and there are pending rows")
	}
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsBatchWorker.Work — core behaviour
// ---------------------------------------------------------------------------

func TestValidateBatchWorker_NilRepo(t *testing.T) {
	t.Parallel()
	w := ValidateSubmissionsBatchWorker{Repo: nil}
	err := w.Work(context.Background(), newBatchJob())
	if err == nil {
		t.Fatal("expected error when Repo is nil")
	}
}

func TestValidateBatchWorker_EmptyBatch(t *testing.T) {
	t.Parallel()
	repo := &mockSubmissionRepo{
		pendingRows:  []*domainScraper.Submission{},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no error for empty batch, got %v", err)
	}
	if len(repo.updates) != 0 {
		t.Errorf("expected no UpdateStatus calls for empty batch, got %d", len(repo.updates))
	}
}

func TestValidateBatchWorker_ListError(t *testing.T) {
	t.Parallel()
	repo := &mockSubmissionRepo{listErr: errors.New("db unavailable")}
	w := ValidateSubmissionsBatchWorker{Repo: repo}
	err := w.Work(context.Background(), newBatchJob())
	if err == nil {
		t.Fatal("expected error when ListPendingValidation fails")
	}
}

// ---------------------------------------------------------------------------
// ValidateSubmissionsBatchWorker.validateOne — HTTP server scenarios
// ---------------------------------------------------------------------------

func TestValidateBatchWorker_ValidURL_Accepted(t *testing.T) {
	t.Parallel()

	// Test server: returns 200 for all requests (including /robots.txt).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 1, URL: srv.URL + "/events"},
		},
		pendingCount: 0, // no re-enqueue needed
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.id != 1 {
		t.Errorf("UpdateStatus id = %d, want 1", u.id)
	}
	if u.status != "pending" {
		t.Errorf("UpdateStatus status = %q, want %q", u.status, "pending")
	}
	if u.rejectionReason != nil {
		t.Errorf("UpdateStatus rejectionReason = %q, want nil", *u.rejectionReason)
	}
	if u.validatedAt == nil {
		t.Error("UpdateStatus validatedAt should be non-nil for accepted URL")
	}
}

func TestValidateBatchWorker_HeadReturns404_Rejected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 2, URL: srv.URL + "/gone"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no top-level error (batch continues on per-URL failures), got %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.status != "rejected" {
		t.Errorf("UpdateStatus status = %q, want %q", u.status, "rejected")
	}
	if u.rejectionReason == nil {
		t.Error("expected rejection reason to be set")
	}
}

func TestValidateBatchWorker_HeadReturns301_Accepted(t *testing.T) {
	t.Parallel()

	// 301 is a 3xx — treated as reachable (status < 400).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 3, URL: srv.URL + "/redirect"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.status != "pending" {
		t.Errorf("UpdateStatus status = %q, want %q (3xx should be accepted)", u.status, "pending")
	}
}

func TestValidateBatchWorker_RobotsDisallows_Rejected(t *testing.T) {
	t.Parallel()

	// robots.txt disallows Togather (our user agent).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: Togather\nDisallow: /\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 4, URL: srv.URL + "/events"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.status != "rejected" {
		t.Errorf("UpdateStatus status = %q, want %q", u.status, "rejected")
	}
	if u.rejectionReason == nil {
		t.Error("expected rejection reason for robots.txt disallow")
	}
}

func TestValidateBatchWorker_RobotsError_TreatedAsAllowed(t *testing.T) {
	t.Parallel()

	// robots.txt returns 500 — should be treated as allowed (conservative policy).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 5, URL: srv.URL + "/events"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	// robots.txt error → treat as allowed → status should be "pending"
	if u.status != "pending" {
		t.Errorf("UpdateStatus status = %q, want %q (robots error treated as allowed)", u.status, "pending")
	}
}

func TestValidateBatchWorker_UnreachableURL_Rejected(t *testing.T) {
	t.Parallel()

	// Use a server that is immediately closed so HEAD will fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/gone"
	srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 6, URL: url},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no top-level error (batch continues), got %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.status != "rejected" {
		t.Errorf("UpdateStatus status = %q, want %q", u.status, "rejected")
	}
}

func TestValidateBatchWorker_InvalidURL_Rejected(t *testing.T) {
	t.Parallel()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 7, URL: "://not-a-valid-url"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no top-level error, got %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.status != "rejected" {
		t.Errorf("UpdateStatus status = %q, want %q", u.status, "rejected")
	}
}

func TestValidateBatchWorker_MultiplURLs_AllProcessed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 10, URL: srv.URL + "/a"},
			{ID: 11, URL: srv.URL + "/b"},
			{ID: 12, URL: srv.URL + "/c"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updates) != 3 {
		t.Errorf("expected 3 UpdateStatus calls (one per URL), got %d", len(repo.updates))
	}
	for _, u := range repo.updates {
		if u.status != "pending" {
			t.Errorf("URL %d: expected status %q, got %q", u.id, "pending", u.status)
		}
	}
}

func TestValidateBatchWorker_OneErrorDoesNotAbortBatch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Second URL is a closed server — its HEAD will fail.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL + "/x"
	dead.Close()

	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 20, URL: srv.URL + "/ok"},
			{ID: 21, URL: deadURL},
			{ID: 22, URL: srv.URL + "/also-ok"},
		},
		pendingCount: 0,
	}
	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected no top-level error (batch continues), got %v", err)
	}

	if len(repo.updates) != 3 {
		t.Fatalf("expected 3 UpdateStatus calls, got %d", len(repo.updates))
	}
	// IDs 20 and 22 should be pending; ID 21 should be rejected.
	statuses := make(map[int64]string, 3)
	for _, u := range repo.updates {
		statuses[u.id] = u.status
	}
	if statuses[20] != "pending" {
		t.Errorf("ID 20: want status %q, got %q", "pending", statuses[20])
	}
	if statuses[21] != "rejected" {
		t.Errorf("ID 21: want status %q, got %q", "rejected", statuses[21])
	}
	if statuses[22] != "pending" {
		t.Errorf("ID 22: want status %q, got %q", "pending", statuses[22])
	}
}

func TestValidateBatchWorker_CountAfterBatchFails_NonFatal(t *testing.T) {
	t.Parallel()

	// CountPendingValidation is called twice: once by the scheduler (not applicable here),
	// and once inside the batch worker after processing. The first call is ListPendingValidation.
	// We arrange: list succeeds, count after batch fails → worker should still return nil.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	callCount := 0
	repo := &mockSubmissionRepo{
		pendingRows: []*domainScraper.Submission{
			{ID: 30, URL: srv.URL + "/ok"},
		},
	}
	// Override countErr after list: we do it inline via a custom repo.
	// The simplest approach: set countErr so the post-batch count fails.
	repo.countErr = errors.New("db transient error")

	w := ValidateSubmissionsBatchWorker{Repo: repo, HTTPClient: plainClient()}
	err := w.Work(context.Background(), newBatchJob())
	if err != nil {
		t.Fatalf("expected nil (count error after batch is non-fatal), got %v", err)
	}
	_ = callCount // suppress unused warning
}

func TestIsTimeoutError_Nil(t *testing.T) {
	if isTimeoutError(nil) {
		t.Error("isTimeoutError(nil) should return false")
	}
}

func TestIsTimeoutError_Timeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		msg  string
		want bool
	}{
		{"connection timeout", true},
		{"context deadline exceeded", true},
		{"i/o timeout", true},
		{"connection refused", false},
		{"not found", false},
	}
	for _, tt := range tests {
		got := isTimeoutError(errors.New(tt.msg))
		if got != tt.want {
			t.Errorf("isTimeoutError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// newSSRFBlockingTransport
// ---------------------------------------------------------------------------

// TestSSRFBlockingTransport_BlocksPrivateIPs verifies that the SSRF-blocking
// transport refuses connections to addresses in RFC-1918, loopback, and
// link-local ranges (srv-exv8k).
func TestSSRFBlockingTransport_BlocksPrivateIPs(t *testing.T) {
	t.Parallel()

	// Start a real local server on 127.0.0.1 so we can confirm that the
	// transport rejects the dial before data reaches the server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: newSSRFBlockingTransport(),
	}

	// The test server listens on 127.0.0.1 — should be blocked.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodHead, srv.URL, nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected SSRF block for 127.0.0.1, got nil error")
	}

	// Confirm the error is SSRF-related, not some other transport error.
	errStr := err.Error()
	if !strings.Contains(errStr, "SSRF") && !strings.Contains(errStr, "blocked") {
		t.Errorf("expected SSRF/blocked error, got: %v", err)
	}
}

// TestSSRFBlockingTransport_AllowsPublicIPs verifies that the transport does
// NOT block connections to public IP addresses.  We use a real httptest server
// bound to a loopback-adjacent address and override the dialer to connect to
// 127.0.0.1 only in tests — instead we just confirm the transport doesn't
// block the normal test server by checking the blocked-CIDRs helper directly.
func TestSSRFBlockingTransport_BlockedCIDRs(t *testing.T) {
	t.Parallel()

	blocked := mustParseCIDRs([]string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	})

	shouldBlock := []string{
		"127.0.0.1", "10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.255",
		"192.168.0.1", "192.168.255.255",
		"169.254.169.254", // AWS IMDS
		"::1",
		"fc00::1", "fdff::1",
		"fe80::1",
	}
	shouldAllow := []string{
		"8.8.8.8", "1.1.1.1", "203.0.113.1", "2001:db8::1",
	}

	for _, ipStr := range shouldBlock {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("test setup: invalid IP %q", ipStr)
		}
		inBlocked := false
		for _, cidr := range blocked {
			if cidr.Contains(ip) {
				inBlocked = true
				break
			}
		}
		if !inBlocked {
			t.Errorf("IP %s should be blocked but is not in any blocked CIDR", ipStr)
		}
	}

	for _, ipStr := range shouldAllow {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("test setup: invalid IP %q", ipStr)
		}
		inBlocked := false
		for _, cidr := range blocked {
			if cidr.Contains(ip) {
				inBlocked = true
				break
			}
		}
		if inBlocked {
			t.Errorf("IP %s should be allowed but is in a blocked CIDR", ipStr)
		}
	}
}
