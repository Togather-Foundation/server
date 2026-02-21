package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// newMockIngestServer starts an httptest server that simulates the async
// batch ingest API: POST /api/v1/events:batch returns 202 with a status_url,
// and GET /api/v1/batch-status/test-batch returns the completed result.
// It records whether the batch POST was called.
func newMockIngestServer(t *testing.T, called *bool) *httptest.Server {
	t.Helper()
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events:batch":
			if called != nil {
				*called = true
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id":   "test-batch",
				"status":     "processing",
				"status_url": srvURL + "/api/v1/batch-status/test-batch",
			})

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batch-status/test-batch":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id":   "test-batch",
				"status":     "completed",
				"created":    1,
				"duplicates": 0,
				"failed":     0,
			})

		default:
			t.Errorf("mock ingest: unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	// Capture the URL after the server is created.
	srvURL = srv.URL
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

// TestScrapeTier0_FollowEventURLs verifies that when FollowEventURLs is true the
// scraper fetches individual event detail pages to replace truncated descriptions.
func TestScrapeTier0_FollowEventURLs(t *testing.T) {
	// Serve a detail page that contains a full description in .tribe-events-content p
	const fullDescription = "This is the full and complete event description from the Tribe Events content area, definitely more than fifty characters."

	detailSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<html><body><div class="tribe-events-content"><p>%s</p></div></body></html>`, fullDescription)
	}))
	t.Cleanup(detailSrv.Close)

	// Build a listing page with one JSON-LD event whose description is truncated
	// and whose url points to the detail server.
	listingHTML := fmt.Sprintf(`<!DOCTYPE html>
<html><head><script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Event",
  "name": "Botany Workshop",
  "startDate": "2026-06-15T10:00:00-04:00",
  "endDate": "2026-06-15T12:00:00-04:00",
  "description": "Join us for a hands-on botany workshop\u2026",
  "url": "%s/event/botany-workshop",
  "location": {
    "@type": "Place",
    "name": "Toronto Botanical Garden",
    "address": {
      "@type": "PostalAddress",
      "addressLocality": "Toronto",
      "addressRegion": "ON",
      "addressCountry": "CA"
    }
  }
}
</script></head><body></body></html>`, detailSrv.URL)

	listingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(listingHTML))
	}))
	t.Cleanup(listingSrv.Close)

	var ingestBody []byte
	var ingestSrvURL string
	ingestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events:batch":
			var err error
			ingestBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading ingest body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id":   "test-batch",
				"status":     "processing",
				"status_url": ingestSrvURL + "/api/v1/batch-status/test-batch",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batch-status/test-batch":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"batch_id": "test-batch", "status": "completed",
				"created": 1, "duplicates": 0, "failed": 0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	ingestSrvURL = ingestSrv.URL
	t.Cleanup(ingestSrv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: test-tbg
url: %s
tier: 0
enabled: true
trust_level: 7
schedule: manual
follow_event_urls: true
`, listingSrv.URL)
	if err := os.WriteFile(filepath.Join(dir, "test-tbg.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, ingestSrv.URL)

	results, err := scraper.ScrapeAll(t.Context(), ScrapeOptions{
		DryRun:     false,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeAll error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.Error != nil {
		t.Fatalf("result.Error = %v, want nil", res.Error)
	}
	if res.EventsFound != 1 {
		t.Errorf("EventsFound = %d, want 1", res.EventsFound)
	}
	if res.EventsSubmitted != 1 {
		t.Errorf("EventsSubmitted = %d, want 1", res.EventsSubmitted)
	}

	// Verify the full description was sent to the ingest endpoint
	if !bytes.Contains(ingestBody, []byte(fullDescription)) {
		t.Errorf("ingest body does not contain full description\nbody: %s", ingestBody)
	}
}
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
