package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/Togather-Foundation/server/internal/sse"
)

// flusherRecorder is an httptest.ResponseRecorder that also implements http.Flusher.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flusherRecorder) Flush() { f.flushed++ }

// makeScrapeSourceEvent builds a *river.Event for a scrape_source job.
func makeScrapeSourceEvent(kind river.EventKind, jobID int64, sourceName string) *river.Event {
	args, _ := json.Marshal(map[string]string{"source_name": sourceName})
	return &river.Event{
		Kind: kind,
		Job: &rivertype.JobRow{
			ID:          jobID,
			Kind:        "scrape_source",
			EncodedArgs: args,
		},
	}
}

// makeOtherEvent builds a *river.Event for a non-scrape_source job.
func makeOtherEvent(kind river.EventKind, jobID int64, jobKind string) *river.Event {
	return &river.Event{
		Kind: kind,
		Job: &rivertype.JobRow{
			ID:          jobID,
			Kind:        jobKind,
			EncodedArgs: []byte(`{}`),
		},
	}
}

// newCancelableRequest creates a request with a cancelable context.
func newCancelableRequest(t *testing.T, method, target string) (*http.Request, context.CancelFunc) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	ctx, cancel := context.WithCancel(req.Context())
	return req.WithContext(ctx), cancel
}

// TestAdminEventsSSEHandler_SetsSSEHeaders verifies SSE headers and retry directive.
func TestAdminEventsSSEHandler_SetsSSEHeaders(t *testing.T) {
	broker := sse.NewBroker()
	h := &AdminEventsSSEHandler{Broker: broker, Env: "test"}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req, cancelReq := newCancelableRequest(t, http.MethodGet, "/api/v1/admin/scraper/events")
	defer cancelReq()

	// Run handler in a goroutine so we can cancel it after checking headers
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	// Cancel and wait for handler to fully stop before reading recorder state.
	// Reading rec.Result() concurrently with the handler writing to rec is a data race.
	cancelReq()
	<-done

	// Now safe to read recorder — handler goroutine has exited.
	result := rec.Result()
	if got := result.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", got, "text/event-stream")
	}
	if got := result.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
	if got := result.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want %q", got, "no")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "retry: 5000") {
		t.Errorf("body missing retry directive, got: %q", body)
	}
}

// TestAdminEventsSSEHandler_ForwardsScrapeSourceEvent verifies that a scrape_source event is forwarded.
func TestAdminEventsSSEHandler_ForwardsScrapeSourceEvent(t *testing.T) {
	broker := sse.NewBroker()
	subCh := make(chan *river.Event, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker.Start(ctx, subCh)

	h := &AdminEventsSSEHandler{Broker: broker, Env: "test"}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req, cancelReq := newCancelableRequest(t, http.MethodGet, "/api/v1/admin/scraper/events")
	defer cancelReq()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	// Give handler time to subscribe and write initial retry line
	time.Sleep(50 * time.Millisecond)

	// Send a scrape_source completed event
	ev := makeScrapeSourceEvent(river.EventKindJobCompleted, 42, "my-source")
	subCh <- ev

	// Wait for handler to flush the event
	time.Sleep(100 * time.Millisecond)

	// Cancel request to stop handler
	cancelReq()
	<-done

	body := rec.Body.String()

	// Must contain a data: line
	if !strings.Contains(body, "data:") {
		t.Fatalf("body missing data line: %q", body)
	}

	// Extract the data line and parse JSON
	var dataLine string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			dataLine = strings.TrimPrefix(line, "data:")
			dataLine = strings.TrimSpace(dataLine)
			break
		}
	}
	if dataLine == "" {
		t.Fatalf("no data line found in body: %q", body)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
		t.Fatalf("failed to parse data JSON: %v, data: %q", err, dataLine)
	}

	if got := payload["kind"]; got != "job_completed" {
		t.Errorf("kind = %v, want job_completed", got)
	}
	if got := payload["job_kind"]; got != "scrape_source" {
		t.Errorf("job_kind = %v, want scrape_source", got)
	}
	if got := payload["source_name"]; got != "my-source" {
		t.Errorf("source_name = %v, want my-source", got)
	}
	if got, ok := payload["job_id"].(float64); !ok || int64(got) != 42 {
		t.Errorf("job_id = %v, want 42", payload["job_id"])
	}

	// Must also contain id: line
	if !strings.Contains(body, "id: 42") {
		t.Errorf("body missing id: 42 line, got: %q", body)
	}
}

// TestAdminEventsSSEHandler_FiltersNonScrapeEvents verifies non-scrape events are not forwarded.
func TestAdminEventsSSEHandler_FiltersNonScrapeEvents(t *testing.T) {
	broker := sse.NewBroker()
	subCh := make(chan *river.Event, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker.Start(ctx, subCh)

	h := &AdminEventsSSEHandler{Broker: broker, Env: "test"}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req, cancelReq := newCancelableRequest(t, http.MethodGet, "/api/v1/admin/scraper/events")
	defer cancelReq()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	// Give handler time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Send a non-scrape event
	ev := makeOtherEvent(river.EventKindJobCompleted, 7, "geocode_address")
	subCh <- ev

	// Wait briefly for any forwarding
	time.Sleep(100 * time.Millisecond)

	// Cancel request
	cancelReq()
	<-done

	body := rec.Body.String()

	// Should NOT contain a data: line (only retry: and maybe keepalive comments)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			t.Errorf("got unexpected data line for non-scrape event: %q", line)
		}
	}
}

// TestAdminEventsSSEHandler_DisconnectCleanup verifies handler exits when request context is cancelled.
func TestAdminEventsSSEHandler_DisconnectCleanup(t *testing.T) {
	broker := sse.NewBroker()
	h := &AdminEventsSSEHandler{Broker: broker, Env: "test"}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req, cancelReq := newCancelableRequest(t, http.MethodGet, "/api/v1/admin/scraper/events")

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	// Give handler time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the request context
	cancelReq()

	// Handler must return promptly
	select {
	case <-done:
		// pass
	case <-time.After(time.Second):
		t.Error("handler did not return after context cancellation")
	}
}

// noFlusherWriter is a ResponseWriter that deliberately does NOT implement http.Flusher.
type noFlusherWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func (w *noFlusherWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlusherWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *noFlusherWriter) WriteHeader(code int) {
	w.code = code
}

// TestAdminEventsSSEHandler_NoFlusher_Returns500 verifies 500 when ResponseWriter doesn't implement Flusher.
func TestAdminEventsSSEHandler_NoFlusher_Returns500(t *testing.T) {
	broker := sse.NewBroker()
	h := &AdminEventsSSEHandler{Broker: broker, Env: "test"}

	// noFlusherWriter does NOT implement http.Flusher
	rec := &noFlusherWriter{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/events", nil)

	h.ServeHTTP(rec, req)

	if rec.code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.code)
	}
}
