package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// buildEventPageHTML returns minimal HTML with a valid JSON-LD Event.
func buildEventPageHTML(name, url string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Event",
  "name": %q,
  "startDate": "2026-06-15T10:00:00-04:00",
  "endDate": "2026-06-15T12:00:00-04:00",
  "url": %q,
  "location": {
    "@type": "Place",
    "name": "Test Venue",
    "address": {
      "@type": "PostalAddress",
      "addressLocality": "Toronto",
      "addressRegion": "ON",
      "addressCountry": "CA"
    }
  },
  "description": "A test event."
}
</script>
</head><body><h1>%s</h1></body></html>`, name, url, name)
}

// TestScrapeSitemap_Tier0 verifies that a sitemap source with tier 0 discovers
// URLs via the sitemap XML, applies the filter pattern, and scrapes each
// matching page for JSON-LD events.
func TestScrapeSitemap_Tier0(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
  <url><loc>%s/events/3</loc></url>
  <url><loc>%s/about</loc></url>
  <url><loc>%s/contact</loc></url>
</urlset>`, srvURL, srvURL, srvURL, srvURL, srvURL)

		case "/events/1":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event One", srvURL+"/events/1")))

		case "/events/2":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event Two", srvURL+"/events/2")))

		case "/events/3":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event Three", srvURL+"/events/3")))

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	// Build a temporary sources directory with one sitemap-based Tier 0 config.
	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: sitemap-test-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
  rate_limit_ms: 0
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "sitemap-test.yaml"), []byte(cfg), 0o644); err != nil {
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
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.Error != nil {
		t.Fatalf("result.Error = %v, want nil", res.Error)
	}
	if res.SourceName != "sitemap-test-source" {
		t.Errorf("SourceName = %q, want %q", res.SourceName, "sitemap-test-source")
	}
	if res.EventsFound != 3 {
		t.Errorf("EventsFound = %d, want 3", res.EventsFound)
	}
	if res.EventsSubmitted != 3 {
		t.Errorf("EventsSubmitted = %d, want 3", res.EventsSubmitted)
	}
}

// TestScrapeSitemap_FilterPattern verifies that only URLs matching the filter
// pattern are scraped (non-matching URLs are excluded).
func TestScrapeSitemap_FilterPattern(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			// Only /events/1 and /events/2 match the filter pattern; /about does not.
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
  <url><loc>%s/about</loc></url>
</urlset>`, srvURL, srvURL, srvURL)

		case "/events/1":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event One", srvURL+"/events/1")))

		case "/events/2":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event Two", srvURL+"/events/2")))

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: filter-test-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
  rate_limit_ms: 0
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "filter-test.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(t.Context(), "filter-test-source", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound != 2 {
		t.Errorf("EventsFound = %d, want 2 (filter should exclude /about)", result.EventsFound)
	}
	if result.EventsSubmitted != 2 {
		t.Errorf("EventsSubmitted = %d, want 2", result.EventsSubmitted)
	}
}

// TestScrapeSitemap_MaxURLs verifies that MaxURLs caps the number of URLs scraped.
func TestScrapeSitemap_MaxURLs(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
  <url><loc>%s/events/3</loc></url>
</urlset>`, srvURL, srvURL, srvURL)

		case "/events/1":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event One", srvURL+"/events/1")))

		case "/events/2":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event Two", srvURL+"/events/2")))

		case "/events/3":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event Three", srvURL+"/events/3")))

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: maxurls-test-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
  max_urls: 2
  rate_limit_ms: 0
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "maxurls-test.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(t.Context(), "maxurls-test-source", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	// Only 2 URLs should have been scraped due to max_urls cap.
	if result.EventsSubmitted != 2 {
		t.Errorf("EventsSubmitted = %d, want 2 (capped by max_urls=2)", result.EventsSubmitted)
	}
}

// TestScrapeSitemap_SitemapFetchFail verifies that a sitemap fetch failure
// returns an error in the result (not a fatal error from ScrapeSource).
func TestScrapeSitemap_SitemapFetchFail(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: fail-sitemap-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
`, srv.URL, srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "fail-sitemap.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(t.Context(), "fail-sitemap-source", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	// ScrapeSource should not return a fatal err — the error is in result.Error.
	if err != nil {
		t.Fatalf("ScrapeSource returned fatal error: %v", err)
	}
	if result.Error == nil {
		t.Error("expected result.Error to be set on sitemap fetch failure, got nil")
	}
}

// TestScrapeSitemap_Tier1 verifies that a sitemap source with tier 1 discovers
// URLs via sitemap XML, then scrapes each detail page using CSS selectors
// (not JSON-LD). The selector config must match the HTML structure served.
//
// Note: Tier 2 (Rod/headless browser) is not tested here because it requires
// a running headless browser instance which is not available in unit tests.
func TestScrapeSitemap_Tier1(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
</urlset>`, srvURL, srvURL)

		case "/events/1":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event">
  <h1>CSS Event One</h1>
  <time datetime="2026-07-01T19:00:00">July 1</time>
  <span class="venue">Test Hall</span>
  <p class="desc">First CSS event</p>
</div></body></html>`)

		case "/events/2":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event">
  <h1>CSS Event Two</h1>
  <time datetime="2026-07-02T20:00:00">July 2</time>
  <span class="venue">Test Hall</span>
  <p class="desc">Second CSS event</p>
</div></body></html>`)

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: sitemap-tier1-test
url: %s
tier: 1
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
  rate_limit_ms: 1
selectors:
  event_list: "div.event"
  name: "h1"
  start_date: "time"
  location: "span.venue"
  description: "p.desc"
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "tier1-sitemap.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(t.Context(), "sitemap-tier1-test", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound < 2 {
		t.Errorf("EventsFound = %d, want >= 2", result.EventsFound)
	}
	if result.EventsSubmitted != 2 {
		t.Errorf("EventsSubmitted = %d, want 2", result.EventsSubmitted)
	}
}

// TestScrapeSitemap_ContextCancellation verifies that cancelling the context
// mid-scrape stops the sitemap loop and returns partial results (not all URLs).
func TestScrapeSitemap_ContextCancellation(t *testing.T) {
	t.Parallel()

	var srvURL string
	var requestCount atomic.Int32
	var cancel context.CancelFunc

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
  <url><loc>%s/events/3</loc></url>
  <url><loc>%s/events/4</loc></url>
  <url><loc>%s/events/5</loc></url>
</urlset>`, srvURL, srvURL, srvURL, srvURL, srvURL)

		case "/events/1", "/events/2", "/events/3", "/events/4", "/events/5":
			n := requestCount.Add(1)
			// Cancel the context after the second detail page is served so the
			// loop is interrupted mid-flight.
			if n == 2 {
				cancel()
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Event "+r.URL.Path[len("/events/"):], srvURL+r.URL.Path)))

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: cancel-sitemap-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap.xml
  filter_pattern: "/events/.+"
  rate_limit_ms: 1
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "cancel-sitemap.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	ctx, cancelFn := context.WithCancel(t.Context())
	cancel = cancelFn
	defer cancelFn()

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(ctx, "cancel-sitemap-source", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	// After cancellation, we expect fewer than 5 events.
	if result.EventsSubmitted >= 5 {
		t.Errorf("EventsSubmitted = %d, want < 5 (loop should have been cancelled)", result.EventsSubmitted)
	}
	if requestCount.Load() >= 5 {
		t.Errorf("requestCount = %d, want < 5 (cancellation should stop loop)", requestCount.Load())
	}
}

// TestScrapeSitemap_SitemapIndex verifies that scrapeSitemap handles a sitemap
// index (sitemapindex XML) by recursively fetching the child sitemaps and
// collecting all event URLs across them.
func TestScrapeSitemap_SitemapIndex(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)

		case "/sitemap-index.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sitemap-events-1.xml</loc></sitemap>
  <sitemap><loc>%s/sitemap-events-2.xml</loc></sitemap>
</sitemapindex>`, srvURL, srvURL)

		case "/sitemap-events-1.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/1</loc></url>
  <url><loc>%s/events/2</loc></url>
</urlset>`, srvURL, srvURL)

		case "/sitemap-events-2.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/events/3</loc></url>
</urlset>`, srvURL)

		case "/events/1":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Index Event One", srvURL+"/events/1")))

		case "/events/2":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Index Event Two", srvURL+"/events/2")))

		case "/events/3":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(buildEventPageHTML("Index Event Three", srvURL+"/events/3")))

		default:
			http.NotFound(w, r)
		}
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := fmt.Sprintf(`name: sitemap-index-source
url: %s
tier: 0
enabled: true
trust_level: 5
schedule: manual
sitemap:
  url: %s/sitemap-index.xml
  filter_pattern: "/events/.+"
  rate_limit_ms: 1
`, srvURL, srvURL)
	if err := os.WriteFile(filepath.Join(dir, "sitemap-index.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	scraper := newTestScraper(t, "http://unused")

	result, err := scraper.ScrapeSource(t.Context(), "sitemap-index-source", ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
	})
	if err != nil {
		t.Fatalf("ScrapeSource returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}
	if result.EventsFound != 3 {
		t.Errorf("EventsFound = %d, want 3 (from both child sitemaps)", result.EventsFound)
	}
	if result.EventsSubmitted != 3 {
		t.Errorf("EventsSubmitted = %d, want 3", result.EventsSubmitted)
	}
}
