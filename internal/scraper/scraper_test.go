package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// newTestScraper builds a Scraper with a real IngestClient pointing at
// baseURL. queries is nil — DB tracking is skipped.
func newTestScraper(t *testing.T, ingestBaseURL string) *Scraper {
	t.Helper()
	client := NewIngestClient(ingestBaseURL, "test-api-key")
	logger := zerolog.Nop()
	return NewScraper(client, nil, logger)
}

// serveFile starts an httptest server that serves the given testdata file at
// every path and also responds 404 to /robots.txt so the robots check passes.
func serveFile(t *testing.T, filename string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", filename, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newMockIngestServer starts an httptest server that returns a successful
// batch ingest response. It records whether it was called.
func newMockIngestServer(t *testing.T, called *bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		if r.URL.Path != "/api/v1/events:batch" {
			t.Errorf("mock ingest: unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(IngestResult{
			BatchID:       "test-batch",
			EventsCreated: 1,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestScrapeURL_SingleEvent verifies that a page with one JSON-LD event is
// found and dry-run submitted.
func TestScrapeURL_SingleEvent(t *testing.T) {
	pageSrv := serveFile(t, "single_event.html")
	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeURL(t.Context(), pageSrv.URL+"/events", ScrapeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ScrapeURL returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound != 1 {
		t.Errorf("EventsFound = %d, want 1", result.EventsFound)
	}
	if result.EventsSubmitted != 1 {
		t.Errorf("EventsSubmitted = %d, want 1", result.EventsSubmitted)
	}
}

// TestScrapeURL_NoJSONLD verifies that a page with no JSON-LD yields zero events
// and no error.
func TestScrapeURL_NoJSONLD(t *testing.T) {
	pageSrv := serveFile(t, "no_jsonld.html")
	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeURL(t.Context(), pageSrv.URL+"/events", ScrapeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ScrapeURL returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound != 0 {
		t.Errorf("EventsFound = %d, want 0", result.EventsFound)
	}
}

// TestScrapeURL_WithLimit verifies that Limit caps the number of events
// submitted while EventsFound still reflects all events on the page.
func TestScrapeURL_WithLimit(t *testing.T) {
	// graph_events.html contains 2 valid events.
	pageSrv := serveFile(t, "graph_events.html")
	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeURL(t.Context(), pageSrv.URL+"/events", ScrapeOptions{
		DryRun: true,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("ScrapeURL returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound != 2 {
		t.Errorf("EventsFound = %d, want 2", result.EventsFound)
	}
	if result.EventsSubmitted != 1 {
		t.Errorf("EventsSubmitted = %d, want 1 (limit applied)", result.EventsSubmitted)
	}
}

// TestScrapeURL_IngestSubmits verifies that a non-dry-run scrape actually POSTs
// to the ingest endpoint.
func TestScrapeURL_IngestSubmits(t *testing.T) {
	pageSrv := serveFile(t, "single_event.html")

	var ingestCalled bool
	ingestSrv := newMockIngestServer(t, &ingestCalled)

	scraper := newTestScraper(t, ingestSrv.URL)

	result, err := scraper.ScrapeURL(t.Context(), pageSrv.URL+"/events", ScrapeOptions{DryRun: false})
	if err != nil {
		t.Fatalf("ScrapeURL returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if !ingestCalled {
		t.Error("expected ingest endpoint to be called, but it was not")
	}
	if result.EventsCreated != 1 {
		t.Errorf("EventsCreated = %d, want 1", result.EventsCreated)
	}
}

// TestScrapeAll verifies that ScrapeAll iterates over enabled configs and
// returns one result per source.
func TestScrapeAll(t *testing.T) {
	pageSrv := serveFile(t, "single_event.html")

	// Build a temporary sources directory with one valid Tier 0 config.
	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: test-venue
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
`, pageSrv.URL)
	if err := os.WriteFile(filepath.Join(dir, "test-venue.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	results, err := scraper.ScrapeAll(t.Context(), ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeAll returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].SourceName != "test-venue" {
		t.Errorf("SourceName = %q, want %q", results[0].SourceName, "test-venue")
	}
	if results[0].EventsFound != 1 {
		t.Errorf("EventsFound = %d, want 1", results[0].EventsFound)
	}
}
