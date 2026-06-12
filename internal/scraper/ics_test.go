package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// serveICSFile starts an httptest server that serves the named ICS fixture
// from tests/testdata/ics/ with Content-Type text/calendar.
func serveICSFile(t *testing.T, filename string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", "ics", filename))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", filename, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestICSExtractor_BasicEvent verifies that a single-event ICS fixture is
// fetched, parsed, and mapped to a single EventInput.
func TestICSExtractor_BasicEvent(t *testing.T) {
	srv := serveICSFile(t, "parse-basic-event.ics")

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name:       "test-ics",
		URL:        srv.URL,
		TrustLevel: 5,
		License:    "CC0-1.0",
	}
	icsCfg := config.ICSConfig{
		HorizonDays:    90,
		MaxOccurrences: 100,
	}

	results, warnings, err := extractor.Extract(t.Context(), cfg, icsCfg)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}

	evt := results[0]
	if evt.Name != "Community Meetup" {
		t.Errorf("name = %q, want %q", evt.Name, "Community Meetup")
	}
	if evt.Description == "" {
		t.Error("expected non-empty description")
	}
	if evt.StartDate == "" {
		t.Error("expected non-empty start date")
	}
	if evt.Location == nil {
		t.Error("expected location to be set")
	} else if evt.Location.Name != "Toronto Public Library" {
		t.Errorf("location name = %q, want %q", evt.Location.Name, "Toronto Public Library")
	}

	// Should have no warnings for a well-formed feed.
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
}

// TestICSExtractor_MultiEvent verifies that a multi-event ICS fixture returns
// multiple EventInputs.
func TestICSExtractor_MultiEvent(t *testing.T) {
	srv := serveICSFile(t, "parse-multi-event.ics")

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name:       "test-multi",
		URL:        srv.URL,
		TrustLevel: 5,
	}
	icsCfg := config.ICSConfig{
		HorizonDays:    90,
		MaxOccurrences: 100,
	}

	results, _, err := extractor.Extract(t.Context(), cfg, icsCfg)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(results) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(results))
	}
}

// TestICSExtractor_EmptyFeed verifies that an empty calendar returns zero
// events and no error.
func TestICSExtractor_EmptyFeed(t *testing.T) {
	srv := serveICSFile(t, "parse-empty-calendar.ics")

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name:       "test-empty",
		URL:        srv.URL,
		TrustLevel: 5,
	}
	icsCfg := config.ICSConfig{}

	results, _, err := extractor.Extract(t.Context(), cfg, icsCfg)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 events from empty feed, got %d", len(results))
	}
}

// TestICSExtractor_HTTP404 verifies that a 404 response produces an error.
func TestICSExtractor_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name: "test-404",
		URL:  srv.URL,
	}

	_, _, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

// TestICSExtractor_BodyTooLarge verifies that a feed exceeding maxBodyBytes
// produces an error.
func TestICSExtractor_BodyTooLarge(t *testing.T) {
	// Serve a response that is larger than our limit.
	const limit int64 = 256
	largeBody := strings.Repeat("X", int(limit)+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(largeBody))
	}))
	t.Cleanup(srv.Close)

	extractor := NewICSExtractor(http.DefaultClient, limit, false, zerolog.Nop())
	cfg := SourceConfig{
		Name: "test-large",
		URL:  srv.URL,
	}

	_, _, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{})
	if err == nil {
		t.Fatal("expected error for body too large, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention exceeds, got: %v", err)
	}
}

// TestICSExtractor_RedirectFollowed verifies that HTTP redirects are followed.
func TestICSExtractor_RedirectFollowed(t *testing.T) {
	// Target server serves the ICS data.
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", "ics", "parse-basic-event.ics"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write(data)
	}))
	t.Cleanup(target.Close)

	// Redirect server sends 301 to the target.
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))
	t.Cleanup(redirect.Close)

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name: "test-redirect",
		URL:  redirect.URL,
	}

	results, _, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{
		HorizonDays:    90,
		MaxOccurrences: 100,
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 event after redirect, got %d", len(results))
	}
}

// TestICSExtractor_ContentTypeHTML verifies that a text/html Content-Type
// produces a warning.
func TestICSExtractor_ContentTypeHTML(t *testing.T) {
	// Serve valid ICS data but with text/html Content-Type.
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", "ics", "parse-basic-event.ics"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name: "test-html-ct",
		URL:  srv.URL,
	}

	_, warnings, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{
		HorizonDays:    90,
		MaxOccurrences: 100,
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "text/html") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a warning about text/html Content-Type")
	}
}

// TestICSExtractor_ContextCancelled verifies that a cancelled context propagates.
func TestICSExtractor_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — let context cancellation take effect.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{
		Name: "test-cancel",
		URL:  srv.URL,
	}

	_, _, err := extractor.Extract(ctx, cfg, config.ICSConfig{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestIsCalendarContentType verifies content type detection.
func TestIsCalendarContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/calendar", true},
		{"text/calendar; charset=utf-8", true},
		{"TEXT/CALENDAR", true},
		{"application/ics", true},
		{"text/html", false},
		{"application/json", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isCalendarContentType(tt.ct)
			if got != tt.want {
				t.Errorf("isCalendarContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

// TestScrapeICS_ViaDispatch verifies that scrapeICS is dispatched correctly via
// ScrapeSource when ExtractionMethod is "ics". Uses dry-run mode to avoid
// needing a real ingest server.
func TestScrapeICS_ViaDispatch(t *testing.T) {
	// Serve multi-event fixture.
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", "ics", "parse-multi-event.ics"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	// Write a temporary YAML config file with extraction_method: ics.
	cfgYAML := fmt.Sprintf(`
name: test-ics-dispatch
url: %s
tier: 0
trust_level: 5
enabled: true
extraction_method: ics
schedule: manual
`, srv.URL)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test-ics.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Build scraper with mock ingest (won't be called in dry-run).
	ingest := NewIngestClient("http://unused", "test-key")
	s := NewScraper(ingest, nil, zerolog.Nop())
	s.SetICSConfig(config.ICSConfig{
		HorizonDays:    90,
		MaxOccurrences: 100,
	})

	result, err := s.ScrapeSource(t.Context(), "test-ics-dispatch", ScrapeOptions{
		DryRun:     true,
		Verbose:    true,
		SourceFile: cfgPath,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("ScrapeResult.Error: %v", result.Error)
	}
	if result.EventsFound < 3 {
		t.Errorf("expected at least 3 events found, got %d", result.EventsFound)
	}
	if len(result.DryRunEvents) < 3 {
		t.Errorf("expected at least 3 dry-run events, got %d", len(result.DryRunEvents))
	}
}

// TestICSExtractor_AcceptHeader verifies that the Accept: text/calendar header
// is sent on ICS feed requests.
func TestICSExtractor_AcceptHeader(t *testing.T) {
	var receivedAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/calendar")
		// Serve minimal valid ICS.
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nEND:VCALENDAR\r\n"))
	}))
	t.Cleanup(srv.Close)

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{Name: "test-accept", URL: srv.URL}

	_, _, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if receivedAccept != "text/calendar" {
		t.Errorf("Accept header = %q, want %q", receivedAccept, "text/calendar")
	}
}

// TestICSExtractor_HTTP204 verifies that a 204 No Content response returns
// zero events and no error (empty calendar).
func TestICSExtractor_HTTP204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	extractor := NewICSExtractor(http.DefaultClient, 0, false, zerolog.Nop())
	cfg := SourceConfig{Name: "test-204", URL: srv.URL}

	results, warnings, err := extractor.Extract(t.Context(), cfg, config.ICSConfig{})
	if err != nil {
		t.Fatalf("expected no error for 204, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 events for 204, got %d", len(results))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for 204, got %d: %v", len(warnings), warnings)
	}
}
