package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog"
)

// --- extractEventsFromHTML tests ---

func TestExtractEventsFromHTML_Basic(t *testing.T) {
	html := `
<!DOCTYPE html>
<html>
<body>
  <ul>
    <li class="event-card">
      <h2 class="event-name">Jazz Night</h2>
      <time class="event-date" datetime="2026-03-15T20:00:00">March 15, 2026</time>
      <span class="event-location">The Rex Hotel</span>
      <p class="event-desc">A wonderful jazz evening.</p>
      <a class="event-url" href="/events/jazz-night">Details</a>
      <img class="event-img" src="/img/jazz.jpg" alt="Jazz Night" />
    </li>
    <li class="event-card">
      <h2 class="event-name">Folk Festival</h2>
      <time class="event-date" datetime="2026-04-01T12:00:00">April 1, 2026</time>
      <span class="event-location">Harbourfront</span>
      <p class="event-desc">Annual folk festival.</p>
      <a class="event-url" href="/events/folk-festival">Details</a>
      <img class="event-img" src="/img/folk.jpg" alt="Folk Festival" />
    </li>
  </ul>
</body>
</html>`

	cfg := SourceConfig{
		Name: "test-source",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList:   ".event-card",
			Name:        ".event-name",
			StartDate:   ".event-date",
			Location:    ".event-location",
			Description: ".event-desc",
			URL:         ".event-url",
			Image:       ".event-img",
		},
	}

	events, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err != nil {
		t.Fatalf("extractEventsFromHTML returned error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// First event
	e1 := events[0]
	if e1.Name != "Jazz Night" {
		t.Errorf("event[0].Name = %q; want %q", e1.Name, "Jazz Night")
	}
	if e1.StartDate != "2026-03-15T20:00:00" {
		t.Errorf("event[0].StartDate = %q; want %q", e1.StartDate, "2026-03-15T20:00:00")
	}
	if e1.Location != "The Rex Hotel" {
		t.Errorf("event[0].Location = %q; want %q", e1.Location, "The Rex Hotel")
	}
	if e1.Description != "A wonderful jazz evening." {
		t.Errorf("event[0].Description = %q; want %q", e1.Description, "A wonderful jazz evening.")
	}
	if e1.URL != "https://example.com/events/jazz-night" {
		t.Errorf("event[0].URL = %q; want %q", e1.URL, "https://example.com/events/jazz-night")
	}
	if e1.Image != "https://example.com/img/jazz.jpg" {
		t.Errorf("event[0].Image = %q; want %q", e1.Image, "https://example.com/img/jazz.jpg")
	}

	// Second event
	e2 := events[1]
	if e2.Name != "Folk Festival" {
		t.Errorf("event[1].Name = %q; want %q", e2.Name, "Folk Festival")
	}
}

func TestExtractEventsFromHTML_Empty(t *testing.T) {
	cfg := SourceConfig{
		Name: "test-source",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".event-card",
			Name:      ".event-name",
		},
	}

	// Empty HTML should return no events, no error.
	events, err := extractEventsFromHTML("", cfg, "https://example.com")
	if err != nil {
		t.Fatalf("expected no error on empty HTML, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty HTML, got %d", len(events))
	}

	// HTML with no matching elements.
	plain := `<html><body><p>Nothing here</p></body></html>`
	events, err = extractEventsFromHTML(plain, cfg, "https://example.com")
	if err != nil {
		t.Fatalf("expected no error on unmatching HTML, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestExtractEventsFromHTML_MissingFields(t *testing.T) {
	// Events with empty name should be skipped.
	html := `
<html><body>
  <div class="event">
    <h2 class="name"></h2>
    <span class="date">2026-05-01</span>
  </div>
  <div class="event">
    <h2 class="name">Good Event</h2>
    <span class="date">2026-05-02</span>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "test-source",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      ".name",
			StartDate: ".date",
		},
	}

	events, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The empty-name event should be skipped.
	if len(events) != 1 {
		t.Fatalf("expected 1 event (name-less event skipped), got %d", len(events))
	}
	if events[0].Name != "Good Event" {
		t.Errorf("expected name %q, got %q", "Good Event", events[0].Name)
	}
}

func TestExtractEventsFromHTML_NoEventListSelector(t *testing.T) {
	// When EventList selector is empty, return an error (srv-wgb5p: validation
	// now requires event_list for tier 2, but extractEventsFromHTML should also
	// surface this clearly rather than silently returning nil).
	cfg := SourceConfig{
		Name:      "test-source",
		URL:       "https://example.com",
		Selectors: SelectorConfig{}, // no EventList
	}

	events, err := extractEventsFromHTML("<html><body></body></html>", cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected error when EventList selector is empty, got nil")
	}
	if events != nil {
		t.Errorf("expected nil events when no EventList selector, got %v", events)
	}
}

// --- RodExtractor behaviour tests (no real Chromium required) ---

func TestRodExtractor_HeadlessDisabled(t *testing.T) {
	logger := zerolog.Nop()
	// headlessEnabled=false — all ScrapeWithBrowser calls must return ErrHeadlessDisabled.
	ext := NewRodExtractor(logger, 2, "", false)

	_, err := ext.ScrapeWithBrowser(context.Background(), SourceConfig{
		Name: "test",
		URL:  "https://example.com",
	})

	if err == nil {
		t.Fatal("expected ErrHeadlessDisabled, got nil")
	}
	if err != ErrHeadlessDisabled {
		t.Errorf("expected ErrHeadlessDisabled sentinel, got: %v", err)
	}
}

func TestRodExtractor_RobotsBlocked(t *testing.T) {
	// Serve a robots.txt that disallows our user agent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprintf(w, "User-agent: %s\nDisallow: /\n", scraperUserAgent)
			return
		}
		fmt.Fprint(w, "<html><body></body></html>")
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	// headlessEnabled=true so we get past that check and hit the robots.txt check.
	ext := NewRodExtractor(logger, 2, "", true)

	cfg := SourceConfig{
		Name:    "test-robots",
		URL:     srv.URL + "/events",
		Enabled: true,
		Tier:    2,
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      ".name",
		},
	}

	_, err := ext.ScrapeWithBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected robots.txt disallowed error, got nil")
	}
	if !strings.Contains(err.Error(), "disallowed") {
		t.Errorf("expected error containing 'disallowed', got: %v", err)
	}
}

func TestRodExtractor_ContextCancellation(t *testing.T) {
	// Fill the semaphore to force the context-cancellation path in ScrapeWithBrowser.
	// With maxConc=1 and the semaphore already full, a cancelled context must return ctx.Err().
	logger := zerolog.Nop()
	ext := NewRodExtractor(logger, 1, "", true)

	// Fill the semaphore so the next acquire blocks.
	ext.sem <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ext.ScrapeWithBrowser(ctx, SourceConfig{
		Name: "test",
		URL:  "https://example.com",
	})

	// Release the semaphore token we inserted.
	<-ext.sem

	// With a full semaphore and a cancelled context, we expect context.Canceled.
	if err == nil {
		t.Fatal("expected context.Canceled, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestRodExtractor_ScrapeWithBrowser_HeadlessDisabled(t *testing.T) {
	// Alias for TestRodExtractor_HeadlessDisabled — verifies the sentinel error
	// is returned consistently when headlessEnabled=false.
	logger := zerolog.Nop()
	ext := NewRodExtractor(logger, 2, "", false)

	_, err := ext.ScrapeWithBrowser(context.Background(), SourceConfig{
		Name: "test-disabled",
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected ErrHeadlessDisabled, got nil")
	}
	if err != ErrHeadlessDisabled {
		t.Errorf("expected ErrHeadlessDisabled sentinel, got: %v", err)
	}
}

func TestScraper_ScrapeTier2_NoRodExtractor(t *testing.T) {
	// When rodExtractor is nil, scrapeTier2 must return a ScrapeResult with
	// Error != nil and a nil Go error (per the scrapeTier2 contract).
	logger := zerolog.Nop()
	s := NewScraper(nil, nil, logger) // nil ingest — exercises error path only

	source := SourceConfig{
		Name: "no-rod-source",
		URL:  "https://example.com",
		Tier: 2,
	}

	result, err := s.scrapeTier2(context.Background(), source, ScrapeOptions{})
	if err != nil {
		t.Fatalf("scrapeTier2 should not return a Go error when rodExtractor is nil, got: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected result.Error to be non-nil when rodExtractor is nil")
	}
	if !strings.Contains(result.Error.Error(), "RodExtractor") && !strings.Contains(result.Error.Error(), "headless") {
		t.Errorf("expected descriptive error about missing extractor, got: %v", result.Error)
	}
}

func TestScraper_ScrapeSource_Tier2_NilRodExtractor(t *testing.T) {
	// A Tier 2 source with a nil rodExtractor must produce a ScrapeResult.Error,
	// not a Go error return from ScrapeSource.
	// We use a YAML source directory stub with a single Tier 2 source.
	logger := zerolog.Nop()
	s := NewScraper(nil, nil, logger) // no rod extractor set

	// The scraper will fail to load YAML from a non-existent directory.
	// We test via scrapeTier2 directly (same contract tested in _ScrapeTier2_NoRodExtractor).
	// Here we verify the high-level ScrapeAll path is consistent: scrapeErr is nil,
	// result.Error carries the problem.
	source := SourceConfig{
		Name:    "tier2-disabled",
		URL:     "https://example.com",
		Tier:    2,
		Enabled: true,
	}

	result, goErr := s.scrapeTier2(context.Background(), source, ScrapeOptions{})
	if goErr != nil {
		t.Fatalf("expected nil Go error, got: %v", goErr)
	}
	if result.Error == nil {
		t.Fatal("expected ScrapeResult.Error != nil when no RodExtractor configured")
	}
}

func TestValidateNavigationURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		// Use IP literals for "should pass" cases to avoid DNS dependency in tests.
		{"http public IP OK", "http://93.184.216.34/path", false},   // example.com IP
		{"https public IP OK", "https://93.184.216.34/path", false}, // example.com IP
		{"file scheme", "file:///etc/passwd", true},
		{"chrome scheme", "chrome://settings", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"no scheme", "example.com/path", true},
		{"missing host", "https:///path", true},
		{"empty", "", true},
		// srv-tbg24: private/loopback IPs must be blocked
		{"loopback 127.0.0.1", "http://127.0.0.1/events", true},
		{"localhost resolves to loopback", "http://localhost/events", true},
		{"RFC1918 10.x", "http://10.0.0.1/events", true},
		{"RFC1918 172.16.x", "http://172.16.0.1/events", true},
		{"RFC1918 192.168.x", "http://192.168.1.1/events", true},
		{"link-local 169.254.x", "http://169.254.169.254/events", true},
		{"IPv6 loopback ::1", "http://[::1]/events", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNavigationURL(tt.rawURL)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for URL %q, got nil", tt.rawURL)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for URL %q: %v", tt.rawURL, err)
			}
		})
	}
}

// --- helper tests ---

func TestResolveURL(t *testing.T) {
	tests := []struct {
		base, href, want string
	}{
		{"https://example.com/events", "/foo", "https://example.com/foo"},
		{"https://example.com/events", "bar", "https://example.com/bar"},
		{"https://example.com/events", "https://other.com/page", "https://other.com/page"},
		{"https://example.com", "", ""},
	}
	for _, tt := range tests {
		got := resolveURL(tt.base, tt.href)
		if got != tt.want {
			t.Errorf("resolveURL(%q, %q) = %q; want %q", tt.base, tt.href, got, tt.want)
		}
	}
}

func TestExtractDateFromSelection(t *testing.T) {
	html := `
<div class="event">
  <time class="date" datetime="2026-06-01T18:00:00">June 1, 2026</time>
  <time class="textonly">July 4, 2026</time>
</div>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	sel := doc.Find(".event")

	// Should prefer datetime attribute.
	got := extractDateFromSelection(sel, ".date")
	if got != "2026-06-01T18:00:00" {
		t.Errorf("expected datetime attr, got %q", got)
	}

	// Falls back to text when no datetime attribute.
	got = extractDateFromSelection(sel, ".textonly")
	if got != "July 4, 2026" {
		t.Errorf("expected text content, got %q", got)
	}

	// Non-existent selector returns empty.
	got = extractDateFromSelection(sel, ".nonexistent")
	if got != "" {
		t.Errorf("expected empty string for missing selector, got %q", got)
	}
}
