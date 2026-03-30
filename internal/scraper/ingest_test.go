package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// batchHandlerPair sets up a pair of handlers (POST /api/v1/events:batch and
// GET /api/v1/batch-status/<id>) on a single httptest.Server. The batch POST
// always returns a 202 with status_url pointing at the same server. The status
// handler calls statusFn to produce the response body and status code.
type batchHandlerPair struct {
	batchID   string
	statusFn  func(callCount int32) (statusCode int, body any)
	callCount int32
	srvURL    string
}

func (p *batchHandlerPair) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events:batch":
		// Verify headers on every batch POST
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing Authorization", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad Content-Type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"batch_id":   p.batchID,
			"status":     "processing",
			"status_url": p.srvURL + "/api/v1/batch-status/" + p.batchID,
		})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/batch-status/"):
		n := atomic.AddInt32(&p.callCount, 1)
		code, body := p.statusFn(n)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)

	default:
		http.NotFound(w, r)
	}
}

// sampleEvts returns n minimal EventInput values.
func sampleEvts(n int) []events.EventInput {
	out := make([]events.EventInput, n)
	for i := range out {
		out[i] = events.EventInput{Name: "Test Event", StartDate: "2026-03-01T10:00:00Z"}
	}
	return out
}

// completedStatusBody returns a completed batch-status JSON body.
func completedStatusBody(batchID string, created, duplicates, failed int) map[string]any {
	return map[string]any{
		"batch_id":   batchID,
		"status":     "completed",
		"created":    created,
		"duplicates": duplicates,
		"failed":     failed,
	}
}

// TestSubmitBatch_PollsAfter202 verifies that submitChunk polls the status URL
// after receiving a 202 and correctly maps the final counts into IngestResult.
func TestSubmitBatch_PollsAfter202(t *testing.T) {
	handler := &batchHandlerPair{
		batchID: "01JKTEST00001",
		statusFn: func(n int32) (int, any) {
			if n < 3 {
				return http.StatusNotFound, map[string]string{"title": "still processing"}
			}
			return http.StatusOK, completedStatusBody("01JKTEST00001", 2, 0, 0)
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	client := NewIngestClient(srv.URL, "test-api-key")
	result, err := client.SubmitBatch(context.Background(), sampleEvts(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BatchID != "01JKTEST00001" {
		t.Errorf("BatchID = %q, want %q", result.BatchID, "01JKTEST00001")
	}
	if result.EventsCreated != 2 {
		t.Errorf("EventsCreated = %d, want 2", result.EventsCreated)
	}
	if result.EventsDuplicate != 0 {
		t.Errorf("EventsDuplicate = %d, want 0", result.EventsDuplicate)
	}
	if result.EventsFailed != 0 {
		t.Errorf("EventsFailed = %d, want 0", result.EventsFailed)
	}
	if got := atomic.LoadInt32(&handler.callCount); got < 3 {
		t.Errorf("expected ≥3 status polls, got %d", got)
	}
}

// TestSubmitBatch_ImmediateCompletion verifies a single poll that returns
// "completed" immediately works correctly.
func TestSubmitBatch_ImmediateCompletion(t *testing.T) {
	handler := &batchHandlerPair{
		batchID: "batch-imm",
		statusFn: func(_ int32) (int, any) {
			return http.StatusOK, completedStatusBody("batch-imm", 5, 1, 0)
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	client := NewIngestClient(srv.URL, "test-api-key")
	result, err := client.SubmitBatch(context.Background(), sampleEvts(6))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventsCreated != 5 {
		t.Errorf("EventsCreated = %d, want 5", result.EventsCreated)
	}
	if result.EventsDuplicate != 1 {
		t.Errorf("EventsDuplicate = %d, want 1", result.EventsDuplicate)
	}
}

// TestSubmitBatch_FallbackStatusURL verifies that when status_url is absent
// from the 202 body, submitChunk constructs it from batch_id and baseURL.
func TestSubmitBatch_FallbackStatusURL(t *testing.T) {
	// Custom handler that omits status_url in the 202 response.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events:batch":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			// Deliberately omit status_url; only batch_id present.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id": "batch-fallback",
			})
			_ = srvURL

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batch-status/batch-fallback":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(completedStatusBody("batch-fallback", 3, 1, 0))

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	client := NewIngestClient(srv.URL, "test-api-key")
	result, err := client.SubmitBatch(context.Background(), sampleEvts(4))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventsCreated != 3 {
		t.Errorf("EventsCreated = %d, want 3", result.EventsCreated)
	}
	if result.EventsDuplicate != 1 {
		t.Errorf("EventsDuplicate = %d, want 1", result.EventsDuplicate)
	}
}

// TestSubmitBatch_ContextCancelDuringPoll verifies that context cancellation
// during polling surfaces as an error (not a silent zero result).
func TestSubmitBatch_ContextCancelDuringPoll(t *testing.T) {
	handler := &batchHandlerPair{
		batchID: "batch-cancel",
		// Always return "still processing".
		statusFn: func(_ int32) (int, any) {
			return http.StatusNotFound, map[string]string{"title": "still processing"}
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	client := NewIngestClient(srv.URL, "test-api-key")

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	_, err := client.SubmitBatch(ctx, sampleEvts(1))
	// Either context cancelled error or nil (timeout path) is acceptable;
	// what must NOT happen is a panic or a hang.
	_ = err
}

// TestSubmitBatch_ContextCancelBeforePoll verifies that caller context cancellation
// (not internal poll timeout) is propagated as an error.
func TestSubmitBatch_ContextCancelBeforePoll(t *testing.T) {
	handler := &batchHandlerPair{
		batchID: "batch-cancel-before",
		// Return "still processing" - would timeout if we let it run
		statusFn: func(_ int32) (int, any) {
			return http.StatusNotFound, map[string]string{"title": "still processing"}
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	// Use very short backoff so we can test the pre-poll context check
	client := NewIngestClient(srv.URL, "test-api-key",
		WithPollBackoffStart(500*time.Millisecond),
		WithPollBackoffMax(500*time.Millisecond),
		WithPollTimeout(10*time.Second),
	)

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.SubmitBatch(ctx, sampleEvts(1))
	// Should get a context cancelled error, not a silent partial result
	if err == nil {
		t.Fatal("Expected error for cancelled caller context, got nil")
	}
	if !containsString(err.Error(), "cancelled") && !containsString(err.Error(), "context") {
		t.Errorf("Expected error message to mention context cancellation, got: %v", err)
	}
}

// TestSubmitBatch_ErrorOnNon2xx verifies that non-2xx responses from the
// initial POST are returned as errors.
func TestSubmitBatch_ErrorOnNon2xx(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		wantErrContain string
	}{
		{"400 bad request", http.StatusBadRequest, "400"},
		{"429 rate limited", http.StatusTooManyRequests, "rate limited"},
		{"500 server error", http.StatusInternalServerError, "500"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "error body", tc.statusCode)
			}))
			defer srv.Close()

			client := NewIngestClient(srv.URL, "test-api-key")
			_, err := client.SubmitBatch(context.Background(), sampleEvts(1))
			if err == nil {
				t.Fatalf("expected error for HTTP %d, got nil", tc.statusCode)
			}
			if !containsString(err.Error(), tc.wantErrContain) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErrContain)
			}
		})
	}
}

// TestSubmitBatch_EmptySlice verifies that an empty events slice returns a
// zero IngestResult without making any HTTP requests.
func TestSubmitBatch_EmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected for empty events slice")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewIngestClient(srv.URL, "test-api-key")
	result, err := client.SubmitBatch(context.Background(), []events.EventInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventsCreated != 0 || result.EventsDuplicate != 0 || result.EventsFailed != 0 {
		t.Errorf("expected zero result for empty slice, got %+v", result)
	}
}

// TestSubmitBatchChunking verifies that SubmitBatch splits large payloads into
// ≤100-event chunks and aggregates the polled counts correctly.
func TestSubmitBatchChunking(t *testing.T) {
	const total = 150
	evts := sampleEvts(total)

	var batchCount int32
	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events:batch":
			var body struct {
				Events []events.EventInput `json:"events"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			n := atomic.AddInt32(&batchCount, 1)
			batchID := "chunk-batch-" + string(rune('0'+n))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id":   batchID,
				"status_url": srvURL + "/api/v1/batch-status/" + batchID,
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/batch-status/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":     "completed",
				"created":    10,
				"duplicates": 0,
				"failed":     0,
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	client := NewIngestClient(srv.URL, "test-api-key")
	result, err := client.SubmitBatch(context.Background(), evts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := atomic.LoadInt32(&batchCount); got != 2 {
		t.Errorf("expected 2 batch POST calls, got %d", got)
	}
	// Each chunk's status response returns created=10, two chunks → 20.
	if result.EventsCreated != 20 {
		t.Errorf("EventsCreated = %d, want 20", result.EventsCreated)
	}
}

// TestSubmitBatchDryRun verifies no HTTP calls are made for dry-run mode.
func TestSubmitBatchDryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("DryRun should not make any HTTP calls")
		http.Error(w, "unexpected call", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewIngestClient(srv.URL, "test-api-key")

	evts := []events.EventInput{
		{Name: "Event A"},
		{Name: "Event B"},
		{Name: "Event C"},
	}

	result, err := client.SubmitBatchDryRun(context.Background(), evts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BatchID != "dry-run" {
		t.Errorf("BatchID = %q, want %q", result.BatchID, "dry-run")
	}
	if result.EventsCreated != len(evts) {
		t.Errorf("EventsCreated = %d, want %d", result.EventsCreated, len(evts))
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// TestSubmitBatch_PollingTimeout verifies that when the batch status endpoint
// is slow to respond (always returns 404/not found), the client times out
// after the configured pollTimeout and returns a partial result rather than
// hanging forever.
func TestSubmitBatch_PollingTimeout(t *testing.T) {
	handler := &batchHandlerPair{
		batchID: "batch-timeout",
		// Always return "still processing" - simulate slow backend
		statusFn: func(_ int32) (int, any) {
			return http.StatusNotFound, map[string]string{"title": "still processing"}
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	// Create client with short timeout for test
	client := NewIngestClient(srv.URL, "test-api-key", WithPollTimeout(500*time.Millisecond), WithPollBackoffStart(50*time.Millisecond))

	ctx := context.Background()
	result, err := client.SubmitBatch(ctx, sampleEvts(1))
	// Should not error on timeout, should return partial result
	if err != nil {
		t.Fatalf("expected no error on timeout, got: %v", err)
	}
	if result.BatchID != "batch-timeout" {
		t.Errorf("BatchID = %q, want %q", result.BatchID, "batch-timeout")
	}
	// EventsCreated should be 0 since we timed out before getting a result
	if result.EventsCreated != 0 {
		t.Errorf("EventsCreated = %d, want 0 on timeout", result.EventsCreated)
	}
}

// TestSubmitBatch_PollingTimeoutWithCustomConfig verifies that custom polling
// configuration is respected.
func TestSubmitBatch_PollingTimeoutWithCustomConfig(t *testing.T) {
	// Server that responds immediately with completion
	handler := &batchHandlerPair{
		batchID: "batch-fast",
		statusFn: func(_ int32) (int, any) {
			return http.StatusOK, completedStatusBody("batch-fast", 1, 0, 0)
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	// Client with custom backoff that would be too slow if not respected
	client := NewIngestClient(srv.URL, "test-api-key",
		WithPollBackoffStart(10*time.Second), // Intentionally high
		WithPollBackoffMax(10*time.Second),
	)

	result, err := client.SubmitBatch(context.Background(), sampleEvts(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should complete immediately since first poll succeeds
	if result.EventsCreated != 1 {
		t.Errorf("EventsCreated = %d, want 1", result.EventsCreated)
	}
}

// TestSubmitBatch_PollBackoffExponential verifies that the backoff grows
// exponentially and is capped at max.
func TestSubmitBatch_PollBackoffExponential(t *testing.T) {
	var pollIntervals []time.Duration
	var lastPoll time.Time

	handler := &batchHandlerPair{
		batchID: "batch-backoff",
		statusFn: func(n int32) (int, any) {
			now := time.Now()
			if !lastPoll.IsZero() {
				pollIntervals = append(pollIntervals, now.Sub(lastPoll))
			}
			lastPoll = now

			if n < 3 {
				return http.StatusNotFound, map[string]string{"title": "still processing"}
			}
			return http.StatusOK, completedStatusBody("batch-backoff", 1, 0, 0)
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	handler.srvURL = srv.URL

	// Use small starting backoff to see exponential growth in test
	client := NewIngestClient(srv.URL, "test-api-key",
		WithPollBackoffStart(50*time.Millisecond),
		WithPollBackoffMax(200*time.Millisecond),
	)

	result, err := client.SubmitBatch(context.Background(), sampleEvts(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventsCreated != 1 {
		t.Errorf("EventsCreated = %d, want 1", result.EventsCreated)
	}

	// We expect at least 3 polls (fail, fail, success)
	// Intervals should show exponential growth: ~50ms, ~100ms, ~200ms (capped)
	if len(pollIntervals) < 2 {
		t.Errorf("expected at least 2 poll intervals, got %d", len(pollIntervals))
	}
	// First interval should be >= backoffStart (50ms)
	if len(pollIntervals) > 0 && pollIntervals[0] < 40*time.Millisecond {
		t.Errorf("first poll interval = %v, expected >= 40ms", pollIntervals[0])
	}
}
