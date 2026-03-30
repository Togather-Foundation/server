package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// IngestClientConfig holds configuration for the IngestClient.
type IngestClientConfig struct {
	// PollBackoffStart is the initial delay before the first status poll.
	// Default: 200ms
	PollBackoffStart time.Duration

	// PollBackoffMax is the maximum delay between status polls.
	// Default: 2s
	PollBackoffMax time.Duration

	// PollTimeout is the maximum total time spent polling for a single chunk's result.
	// Default: 30s
	PollTimeout time.Duration

	// HTTPClientTimeout is the timeout for HTTP requests.
	// Default: 30s
	HTTPClientTimeout time.Duration
}

// IngestClientOption configures an IngestClient.
type IngestClientOption func(*IngestClientConfig)

// WithPollBackoffStart sets the initial backoff delay for polling.
func WithPollBackoffStart(d time.Duration) IngestClientOption {
	return func(c *IngestClientConfig) { c.PollBackoffStart = d }
}

// WithPollBackoffMax sets the maximum backoff delay for polling.
func WithPollBackoffMax(d time.Duration) IngestClientOption {
	return func(c *IngestClientConfig) { c.PollBackoffMax = d }
}

// WithPollTimeout sets the total polling timeout.
func WithPollTimeout(d time.Duration) IngestClientOption {
	return func(c *IngestClientConfig) { c.PollTimeout = d }
}

// WithHTTPClientTimeout sets the HTTP client timeout.
func WithHTTPClientTimeout(d time.Duration) IngestClientOption {
	return func(c *IngestClientConfig) { c.HTTPClientTimeout = d }
}

// IngestClient submits event batches to a SEL server via the batch ingest API.
type IngestClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string

	pollBackoffStart time.Duration
	pollBackoffMax   time.Duration
	pollTimeout      time.Duration
}

// IngestResult holds the parsed response from a batch ingest submission.
type IngestResult struct {
	BatchID         string        `json:"batch_id"`
	EventsCreated   int           `json:"events_created"`
	EventsDuplicate int           `json:"events_duplicate"`
	EventsFailed    int           `json:"events_failed"`
	Errors          []IngestError `json:"errors,omitempty"`
}

// IngestError describes a per-event error within a batch.
type IngestError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// batchAcceptedResponse is the immediate 202 response from POST /api/v1/events:batch.
type batchAcceptedResponse struct {
	BatchID   string `json:"batch_id"`
	StatusURL string `json:"status_url"`
}

// batchStatusResponse is the response from GET /api/v1/batch-status/<id>
// when the job has completed.
type batchStatusResponse struct {
	BatchID    string `json:"batch_id"`
	Status     string `json:"status"`
	Created    int    `json:"created"`
	Duplicates int    `json:"duplicates"`
	Failed     int    `json:"failed"`
}

// NewIngestClient constructs an IngestClient targeting baseURL with the given
// API key. A 30-second HTTP timeout and a descriptive User-Agent are set by
// default. Use options to override default polling behavior.
func NewIngestClient(baseURL, apiKey string, opts ...IngestClientOption) *IngestClient {
	cfg := IngestClientConfig{
		PollBackoffStart:  200 * time.Millisecond,
		PollBackoffMax:    2 * time.Second,
		PollTimeout:       30 * time.Second,
		HTTPClientTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Defensive clamping: ensure durations are positive and bounded.
	// Zero or negative values would cause issues; clamp to safe minimums.
	if cfg.PollBackoffStart <= 0 {
		cfg.PollBackoffStart = 200 * time.Millisecond
	}
	if cfg.PollBackoffMax <= 0 {
		cfg.PollBackoffMax = 2 * time.Second
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 30 * time.Second
	}
	if cfg.HTTPClientTimeout <= 0 {
		cfg.HTTPClientTimeout = 30 * time.Second
	}
	// Ensure PollBackoffMax >= PollBackoffStart
	if cfg.PollBackoffMax < cfg.PollBackoffStart {
		cfg.PollBackoffMax = cfg.PollBackoffStart
	}
	// Ensure PollTimeout >= PollBackoffMax (sanity bound)
	if cfg.PollTimeout < cfg.PollBackoffMax {
		cfg.PollTimeout = cfg.PollBackoffMax
	}

	return &IngestClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: cfg.HTTPClientTimeout,
		},
		userAgent:        "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)",
		pollBackoffStart: cfg.PollBackoffStart,
		pollBackoffMax:   cfg.PollBackoffMax,
		pollTimeout:      cfg.PollTimeout,
	}
}

// maxBatchSize is the maximum number of events accepted by the API per request.
const maxBatchSize = 100

// SubmitBatch marshals events as {"events":[...]} and POSTs them to
// {baseURL}/api/v1/events:batch. Payloads larger than maxBatchSize are
// automatically split into sequential chunks; results are aggregated into a
// single IngestResult. It returns an IngestResult populated from the JSON
// response.
func (c *IngestClient) SubmitBatch(ctx context.Context, evts []events.EventInput) (IngestResult, error) {
	var combined IngestResult
	for i := 0; i < len(evts); i += maxBatchSize {
		end := min(i+maxBatchSize, len(evts))
		chunk := evts[i:end]

		result, err := c.submitChunk(ctx, chunk)
		if err != nil {
			return combined, err
		}
		combined.EventsCreated += result.EventsCreated
		combined.EventsDuplicate += result.EventsDuplicate
		combined.EventsFailed += result.EventsFailed
		combined.Errors = append(combined.Errors, result.Errors...)
		if combined.BatchID == "" {
			combined.BatchID = result.BatchID
		}
	}
	return combined, nil
}

// submitChunk posts a single chunk of events (must be ≤ maxBatchSize) to the
// batch ingest endpoint. The endpoint responds with HTTP 202 and a status_url;
// submitChunk polls that URL with exponential backoff until the batch is
// completed, then returns the aggregated IngestResult. If polling times out,
// it returns the partial result with a warning rather than failing the whole
// scrape.
func (c *IngestClient) submitChunk(ctx context.Context, evts []events.EventInput) (IngestResult, error) {
	payload, err := json.Marshal(map[string]any{"events": evts})
	if err != nil {
		return IngestResult{}, fmt.Errorf("marshal batch: %w", err)
	}

	url := c.baseURL + "/api/v1/events:batch"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return IngestResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return IngestResult{}, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1 MiB cap
	if err != nil {
		return IngestResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		snippet := bodySnippet(body)
		return IngestResult{}, fmt.Errorf("rate limited (HTTP 429): %s", snippet)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := bodySnippet(body)
		return IngestResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, snippet)
	}

	// HTTP 202: parse the accepted response and poll for final counts.
	if resp.StatusCode == http.StatusAccepted {
		var accepted batchAcceptedResponse
		if err := json.Unmarshal(body, &accepted); err != nil {
			return IngestResult{}, fmt.Errorf("parse 202 response: %w", err)
		}
		statusURL := accepted.StatusURL
		if statusURL == "" && accepted.BatchID != "" {
			statusURL = c.baseURL + "/api/v1/batch-status/" + accepted.BatchID
		}
		return c.pollBatchStatus(ctx, accepted.BatchID, statusURL)
	}

	// Synchronous 200 path (future-proofing / non-async servers).
	var result IngestResult
	if err := json.Unmarshal(body, &result); err != nil {
		return IngestResult{}, fmt.Errorf("parse response: %w", err)
	}
	return result, nil
}

// pollBatchStatus polls statusURL with exponential backoff (start 200 ms, max
// 2 s) for up to pollTimeout. When the server returns status == "completed"
// the counts are mapped into an IngestResult. On internal poll timeout, a partial
// result is returned with a warning logged so the caller can continue.
// If the caller's context is cancelled or deadline exceeded, the error is propagated.
func (c *IngestClient) pollBatchStatus(ctx context.Context, batchID, statusURL string) (IngestResult, error) {
	if statusURL == "" {
		return IngestResult{BatchID: batchID}, fmt.Errorf("no status_url in 202 response for batch %s", batchID)
	}

	deadline := time.Now().Add(c.pollTimeout)
	delay := c.pollBackoffStart

	for {
		// Check caller's context before starting a new poll - propagate cancellation
		// immediately rather than treating it as a soft timeout.
		select {
		case <-ctx.Done():
			return IngestResult{BatchID: batchID}, fmt.Errorf("caller context cancelled while polling batch %s: %w", batchID, ctx.Err())
		default:
		}

		// Create a derived context with our internal poll deadline.
		// This is separate from the caller's context - we want soft timeout behavior
		// for our internal deadline, but hard error for caller cancellation.
		pollCtx, cancel := context.WithDeadline(ctx, deadline)
		result, done, err := c.fetchBatchStatus(pollCtx, batchID, statusURL)
		cancel()

		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return IngestResult{BatchID: batchID}, fmt.Errorf("caller context cancelled while polling batch %s: %w", batchID, ctxErr)
			}
			// If the error is due to our internal poll deadline exceeded, treat it
			// as a soft timeout rather than a hard error - return partial result.
			if isDeadlineExceeded(err) {
				log.Warn().
					Str("batch_id", batchID).
					Dur("poll_timeout", c.pollTimeout).
					Msg("scraper: timed out polling batch status; counts may be incomplete")
				return IngestResult{BatchID: batchID}, nil
			}
			// Hard error (network, auth, etc.) — propagate immediately.
			return IngestResult{BatchID: batchID}, err
		}
		if done {
			return result, nil
		}

		// Not yet complete — wait or bail if we're out of time.
		remaining := time.Until(deadline)
		if remaining <= 0 {
			log.Warn().
				Str("batch_id", batchID).
				Dur("poll_timeout", c.pollTimeout).
				Msg("scraper: timed out polling batch status; counts may be incomplete")
			return IngestResult{BatchID: batchID}, nil
		}

		wait := delay
		if wait > remaining {
			wait = remaining
		}

		select {
		case <-ctx.Done():
			return IngestResult{BatchID: batchID}, fmt.Errorf("caller context cancelled while polling batch %s: %w", batchID, ctx.Err())
		case <-time.After(wait):
		}

		// Exponential backoff capped at pollBackoffMax.
		delay *= 2
		if delay > c.pollBackoffMax {
			delay = c.pollBackoffMax
		}
	}
}

// fetchBatchStatus makes a single GET to statusURL.
// Returns (result, true, nil) when completed, (zero, false, nil) when still
// processing, or (zero, false, err) on a hard failure.
func (c *IngestClient) fetchBatchStatus(ctx context.Context, batchID, statusURL string) (IngestResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return IngestResult{}, false, fmt.Errorf("create status request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return IngestResult{}, false, fmt.Errorf("fetch batch status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return IngestResult{}, false, fmt.Errorf("read status response: %w", err)
	}

	// 404 means the job hasn't completed yet (server returns 404 for
	// "not found or still processing").
	if resp.StatusCode == http.StatusNotFound {
		return IngestResult{}, false, nil
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return IngestResult{}, false, fmt.Errorf("rate limited (HTTP 429) polling batch %s", batchID)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := bodySnippet(body)
		return IngestResult{}, false, fmt.Errorf("unexpected status %d polling batch %s: %s", resp.StatusCode, batchID, snippet)
	}

	var status batchStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return IngestResult{}, false, fmt.Errorf("parse batch status response: %w", err)
	}

	if status.Status != "completed" {
		// Still processing — back off and retry.
		return IngestResult{}, false, nil
	}

	return IngestResult{
		BatchID:         batchID,
		EventsCreated:   status.Created,
		EventsDuplicate: status.Duplicates,
		EventsFailed:    status.Failed,
	}, true, nil
}

// SubmitBatchDryRun returns a synthetic IngestResult without making any HTTP
// call. EventsCreated is set to len(evts) and BatchID is "dry-run".
func (c *IngestClient) SubmitBatchDryRun(_ context.Context, evts []events.EventInput) (IngestResult, error) {
	return IngestResult{
		BatchID:       "dry-run",
		EventsCreated: len(evts),
	}, nil
}

// bodySnippet returns up to 200 characters of body as a string.
func bodySnippet(body []byte) string {
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}

// isDeadlineExceeded checks if the error is due to context deadline exceeded.
func isDeadlineExceeded(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
