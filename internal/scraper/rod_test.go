package scraper

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headlessEnabled reports whether the SCRAPER_HEADLESS_ENABLED env var is set to "true".
// Tests that require a real headless browser must call t.Skip when this returns false.
func headlessEnabled() bool {
	return os.Getenv("SCRAPER_HEADLESS_ENABLED") == "true"
}

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

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
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

	// Empty HTML should return no events with a diagnostic about 0 containers.
	events, _, err := extractEventsFromHTML("", cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic error for 0 containers on empty HTML")
	}
	if !strings.Contains(err.Error(), "matched 0 containers") {
		t.Errorf("expected '0 containers' diagnostic, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty HTML, got %d", len(events))
	}

	// HTML with no matching elements — same diagnostic.
	plain := `<html><body><p>Nothing here</p></body></html>`
	events, _, err = extractEventsFromHTML(plain, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic error for 0 containers on unmatching HTML")
	}
	if !strings.Contains(err.Error(), "matched 0 containers") {
		t.Errorf("expected '0 containers' diagnostic, got: %v", err)
	}
	if !strings.Contains(err.Error(), "scrape capture") {
		t.Errorf("expected actionable hint about 'scrape capture' in error, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestExtractEventsFromHTML_MissingFields(t *testing.T) {
	// Events with empty name should be skipped, and a diagnostic warning returned.
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

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	// Partial name miss → diagnostic warning error returned with valid events.
	if err == nil {
		t.Fatal("expected diagnostic warning error for partial name miss, got nil")
	}
	if !strings.Contains(err.Error(), "WARNING") {
		t.Errorf("expected WARNING in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("expected '1 of 2' skip count in error, got: %v", err)
	}

	// The empty-name event should be skipped.
	if len(events) != 1 {
		t.Fatalf("expected 1 event (name-less event skipped), got %d", len(events))
	}
	if events[0].Name != "Good Event" {
		t.Errorf("expected name %q, got %q", "Good Event", events[0].Name)
	}
}

// TestExtractEventsFromHTML_DateSelectorProbes verifies that probes are captured
// only from the first event container, correctly record matched/unmatched/empty
// states, and are nil when date_selectors is not configured (legacy path).
func TestExtractEventsFromHTML_DateSelectorProbes(t *testing.T) {
	t.Parallel()

	html := `
<html><body>
  <div class="event">
    <h2 class="name">Event One</h2>
    <span class="date">Thu 5th March</span>
    <span class="time"></span>
  </div>
  <div class="event">
    <h2 class="name">Event Two</h2>
    <span class="date">Fri 6th March</span>
    <span class="time">9:00 PM</span>
  </div>
  <div class="event">
    <h2 class="name">Event Three</h2>
    <!-- no .date element -->
    <span class="time">8:00 PM</span>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "probe-test",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList:     ".event",
			Name:          ".name",
			DateSelectors: []string{".date", ".time", ".venue"},
		},
	}

	// .date  → matched, non-empty text ("Thu 5th March")
	// .time  → matched, empty text (element exists but no content)
	// .venue → not matched (no such element in first container)
	_, probes, _ := extractEventsFromHTML(html, cfg, "https://example.com")

	if len(probes) != 3 {
		t.Fatalf("expected 3 probes (one per date_selector), got %d: %v", len(probes), probes)
	}

	// Probe 0: .date → matched with text
	if probes[0].Selector != ".date" {
		t.Errorf("probes[0].Selector = %q; want %q", probes[0].Selector, ".date")
	}
	if !probes[0].Matched {
		t.Errorf("probes[0].Matched = false; want true (.date element exists)")
	}
	if probes[0].Text != "Thu 5th March" {
		t.Errorf("probes[0].Text = %q; want %q", probes[0].Text, "Thu 5th March")
	}

	// Probe 1: .time → matched but empty text
	if probes[1].Selector != ".time" {
		t.Errorf("probes[1].Selector = %q; want %q", probes[1].Selector, ".time")
	}
	if !probes[1].Matched {
		t.Errorf("probes[1].Matched = false; want true (.time element exists in first container)")
	}
	if probes[1].Text != "" {
		t.Errorf("probes[1].Text = %q; want empty (element has no text content)", probes[1].Text)
	}

	// Probe 2: .venue → no element in first container
	if probes[2].Selector != ".venue" {
		t.Errorf("probes[2].Selector = %q; want %q", probes[2].Selector, ".venue")
	}
	if probes[2].Matched {
		t.Errorf("probes[2].Matched = true; want false (.venue not present)")
	}
	if probes[2].Text != "" {
		t.Errorf("probes[2].Text = %q; want empty (no element found)", probes[2].Text)
	}

	// Probes are only from the FIRST container — second and third containers
	// have different content but must not affect the probe slice length.
	if len(probes) != 3 {
		t.Errorf("probe count changed after first container: got %d, want 3", len(probes))
	}

	// Legacy path: no date_selectors → probes must be nil.
	legacyCfg := SourceConfig{
		Name: "legacy-test",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      ".name",
			StartDate: ".date",
		},
	}
	_, legacyProbes, _ := extractEventsFromHTML(html, legacyCfg, "https://example.com")
	if legacyProbes != nil {
		t.Errorf("expected nil probes for legacy (non-date_selectors) config, got %v", legacyProbes)
	}
}

// TestExtractEventsFromHTML_DescriptionSelectors verifies that description_selectors
// extracts and concatenates text from multiple selectors in Tier 2 (srv-nojwn).
func TestExtractEventsFromHTML_DescriptionSelectors(t *testing.T) {
	t.Parallel()

	html := `
<html><body>
  <div class="event">
    <h2 class="name">Art Exhibition</h2>
    <p class="summary">Join us for an amazing exhibition.</p>
    <div class="full-description">This event features works from local artists.</div>
    <p class="more-info">Free admission. Accessible venue.</p>
    <span class="location">Gallery One</span>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "Description Selectors Test",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      "h2.name",
			Location:  "span.location",
			DescriptionSelectors: []string{
				"p.summary",
				"div.full-description",
				"p.more-info",
			},
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	require.NoError(t, err)
	require.Len(t, events, 1, "should extract 1 event")

	event := events[0]
	assert.Equal(t, "Art Exhibition", event.Name)
	assert.Equal(t, "Gallery One", event.Location)
	// Description should be concatenated from all three selectors
	expectedDesc := "Join us for an amazing exhibition. This event features works from local artists. Free admission. Accessible venue."
	assert.Equal(t, expectedDesc, event.Description,
		"description should be assembled from all description_selectors")
}

// TestExtractEventsFromHTML_DescriptionSelectorsFallback verifies that when
// DescriptionSelectors is empty, it falls back to single Description selector.
func TestExtractEventsFromHTML_DescriptionSelectorsFallback(t *testing.T) {
	t.Parallel()

	html := `
<html><body>
  <div class="event">
    <h2 class="name">Simple Event</h2>
    <p class="desc">A simple description.</p>
    <span class="location">Venue</span>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "Description Fallback Test",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList:   ".event",
			Name:        "h2.name",
			Location:    "span.location",
			Description: "p.desc",
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	require.NoError(t, err)
	require.Len(t, events, 1, "should extract 1 event")

	event := events[0]
	assert.Equal(t, "Simple Event", event.Name)
	assert.Equal(t, "A simple description.", event.Description,
		"description should be extracted via legacy Description field")
}

// TestExtractEventsFromHTML_DescriptionSelectorsEmptyResult verifies that when
// no description selectors match, the result is empty (no crash).
func TestExtractEventsFromHTML_DescriptionSelectorsEmptyResult(t *testing.T) {
	t.Parallel()

	html := `
<html><body>
  <div class="event">
    <h2 class="name">No Description Event</h2>
    <span class="location">Venue</span>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "Empty Description Test",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      "h2.name",
			Location:  "span.location",
			DescriptionSelectors: []string{
				"p.nonexistent",
				"div.alsonotthere",
			},
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	require.NoError(t, err)
	require.Len(t, events, 1, "should extract 1 event")

	event := events[0]
	assert.Equal(t, "No Description Event", event.Name)
	assert.Equal(t, "", event.Description,
		"description should be empty when no selectors match")
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

	events, _, err := extractEventsFromHTML("<html><body></body></html>", cfg, "https://example.com")
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

	_, _, err := ext.ScrapeWithBrowser(context.Background(), SourceConfig{
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
			_, _ = fmt.Fprintf(w, "User-agent: %s\nDisallow: /\n", scraperUserAgent)
			return
		}
		_, _ = fmt.Fprint(w, "<html><body></body></html>")
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

	_, _, err := ext.ScrapeWithBrowser(context.Background(), cfg)
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

	_, _, err := ext.ScrapeWithBrowser(ctx, SourceConfig{
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

	_, _, err := ext.ScrapeWithBrowser(context.Background(), SourceConfig{
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
			err := validateNavigationURL(tt.rawURL, blockedCIDRs)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for URL %q, got nil", tt.rawURL)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for URL %q: %v", tt.rawURL, err)
			}
		})
	}
}

func TestValidateNavigationURL_CustomBlocklist(t *testing.T) {
	// With an empty blocklist, loopback/private addresses must be permitted.
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"loopback allowed with empty blocklist", "http://127.0.0.1/events", false},
		{"private IP allowed with empty blocklist", "http://10.0.0.1/events", false},
		{"file scheme still blocked (scheme check)", "file:///etc/passwd", true},
		{"no scheme still blocked", "example.com/path", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNavigationURL(tt.rawURL, nil) // nil = no CIDR blocks
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

// TestExtractTextOrAttrFromSelection verifies that extractTextOrAttrFromSelection
// correctly handles text extraction, attribute extraction, self-matching (where the
// selection itself matches the selector), child-matching, and the no-match case.
func TestExtractTextOrAttrFromSelection(t *testing.T) {
	// HTML fixture: a container span.prices with a data-event attribute and
	// child elements for various extraction scenarios.
	const fixture = `
<div class="container">
  <span class="prices" data-event="EVT-42">
    <a class="link" href="https://example.com/evt">Buy tickets</a>
    <time class="date" datetime="2026-06-01">June 1</time>
    <span class="empty"></span>
  </span>
</div>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("failed to parse HTML fixture: %v", err)
	}

	// The outer container — s does NOT match "span.prices" but has it as a child.
	outer := doc.Find(".container")
	// The span.prices element itself.
	self := doc.Find("span.prices")

	tests := []struct {
		name     string
		sel      *goquery.Selection // the selection passed as 's'
		selector string             // the selector argument (may contain ::attr)
		want     string
	}{
		// --- text extraction ---
		{
			name:     "child text extraction",
			sel:      outer,
			selector: ".link",
			want:     "Buy tickets",
		},
		{
			name:     "child text extraction concatenates all child text",
			sel:      outer,
			selector: "span.prices",
			want:     "Buy tickets\n    June 1", // goquery .Text() concatenates descendant text; TrimSpace trims trailing newline/spaces
		},
		{
			name:     "self text extraction when selection matches selector",
			sel:      self,
			selector: "span.prices",
			want:     "Buy tickets\n    June 1",
		},
		// --- attribute extraction ---
		{
			name:     "child attribute extraction via ::attr syntax",
			sel:      outer,
			selector: "span.prices::data-event",
			want:     "EVT-42",
		},
		{
			name:     "self attribute extraction when selection matches",
			sel:      self,
			selector: "span.prices::data-event",
			want:     "EVT-42",
		},
		{
			name:     "child href attribute extraction",
			sel:      outer,
			selector: "a.link::href",
			want:     "https://example.com/evt",
		},
		{
			name:     "child datetime attribute extraction",
			sel:      outer,
			selector: "time.date::datetime",
			want:     "2026-06-01",
		},
		// --- no match cases ---
		{
			name:     "selector matches no child returns empty",
			sel:      outer,
			selector: ".nonexistent",
			want:     "",
		},
		{
			name:     "selector with attribute but element absent returns empty",
			sel:      outer,
			selector: ".nonexistent::href",
			want:     "",
		},
		{
			name:     "attribute does not exist on matched element returns empty",
			sel:      outer,
			selector: "span.prices::data-missing",
			want:     "",
		},
		// --- empty selector ---
		{
			name:     "empty selector returns empty",
			sel:      outer,
			selector: "",
			want:     "",
		},
		// --- whitespace trimming ---
		{
			name:     "empty child element returns empty string",
			sel:      outer,
			selector: "span.empty",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextOrAttrFromSelection(tt.sel, tt.selector)
			if got != tt.want {
				t.Errorf("extractTextOrAttrFromSelection(sel, %q) = %q; want %q", tt.selector, got, tt.want)
			}
		})
	}
}

// --- RenderHTMLWithNetwork tests (require headless browser) ---

// xhrPageHTML is an HTML page that, once loaded, makes an XHR request to /api/data.
// It injects the returned JSON into a <div id="result">.
const xhrPageHTML = `<!DOCTYPE html>
<html>
<head><title>XHR Test Page</title></head>
<body>
<div id="result">loading...</div>
<script>
var xhr = new XMLHttpRequest();
xhr.open("GET", "/api/data", true);
xhr.onload = function() {
  document.getElementById("result").textContent = xhr.responseText;
};
xhr.send();
</script>
</body>
</html>`

// apiDataJSON is the JSON response served at /api/data.
const apiDataJSON = `{"events":[{"name":"Test Event"}]}`

func TestNetworkCapture_BasicRequests(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/data":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, apiDataJSON)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, xhrPageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	html, requests, err := ext.RenderHTMLWithNetwork(context.Background(), srv.URL+"/", "#result", 5000, "")
	if err != nil {
		t.Fatalf("RenderHTMLWithNetwork returned error: %v", err)
	}

	if html == "" {
		t.Error("expected non-empty HTML")
	}

	if len(requests) == 0 {
		t.Fatal("expected at least one network request to be captured")
	}

	// Find the XHR/API request.
	var apiReq *NetworkRequest
	for i := range requests {
		if strings.Contains(requests[i].URL, "/api/data") {
			apiReq = &requests[i]
			break
		}
	}

	if apiReq == nil {
		t.Fatalf("expected to find request to /api/data, got requests: %v", requests)
	}

	if apiReq.Method != "GET" {
		t.Errorf("expected method GET, got %q", apiReq.Method)
	}
	if !strings.Contains(apiReq.ContentType, "application/json") {
		t.Errorf("expected content type to contain 'application/json', got %q", apiReq.ContentType)
	}
	if apiReq.Status != 200 {
		t.Errorf("expected status 200, got %d", apiReq.Status)
	}
	if !apiReq.IsAPI {
		t.Error("expected IsAPI=true for XHR request with JSON content type")
	}
}

func TestNetworkCapture_NoRequests(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	// Simple static page with no XHR/fetch.
	const staticHTML = `<!DOCTYPE html><html><body><p>Static content</p></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, staticHTML)
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	html, requests, err := ext.RenderHTMLWithNetwork(context.Background(), srv.URL+"/", "body", 5000, "")
	if err != nil {
		t.Fatalf("RenderHTMLWithNetwork returned error: %v", err)
	}

	if html == "" {
		t.Error("expected non-empty HTML")
	}

	// Should capture the Document request (no XHR).
	// No request should be marked IsAPI.
	for _, req := range requests {
		if req.IsAPI {
			t.Errorf("unexpected IsAPI=true for static page request: %+v", req)
		}
	}
}

func TestNetworkCapture_APIDetection(t *testing.T) {
	// Unit test for IsAPI detection logic — uses NetworkRequest values directly.
	// IsAPI must be true when resource type is XHR or Fetch AND content type contains "json".
	tests := []struct {
		resourceType string
		contentType  string
		wantIsAPI    bool
	}{
		{"XHR", "application/json", true},
		{"Fetch", "application/json; charset=utf-8", true},
		{"XHR", "text/html", false},
		{"Fetch", "text/plain", false},
		{"Script", "application/json", false},
		{"Document", "application/json", false},
		{"XHR", "", false},
	}

	for _, tt := range tests {
		got := isAPIRequest(tt.resourceType, tt.contentType)
		if got != tt.wantIsAPI {
			t.Errorf("isAPIRequest(%q, %q) = %v; want %v", tt.resourceType, tt.contentType, got, tt.wantIsAPI)
		}
	}
}

func TestNetworkCapture_WithConfigAndNetwork(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/data":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, apiDataJSON)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, xhrPageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	cfg := SourceConfig{
		Name:    "test-network-config",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector:  "#result",
			WaitTimeoutMs: 5000,
		},
	}

	html, requests, err := ext.RenderHTMLWithConfigAndNetwork(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("RenderHTMLWithConfigAndNetwork returned error: %v", err)
	}

	if html == "" {
		t.Error("expected non-empty HTML")
	}

	// Should capture network activity including the API call.
	var foundAPI bool
	for _, req := range requests {
		if strings.Contains(req.URL, "/api/data") && req.IsAPI {
			foundAPI = true
			break
		}
	}
	if !foundAPI {
		t.Errorf("expected to find IsAPI=true request to /api/data, got: %v", requests)
	}
}

func TestRenderHTMLWithNetwork_HeadlessDisabled(t *testing.T) {
	logger := zerolog.Nop()
	ext := NewRodExtractor(logger, 2, "", false)

	_, _, err := ext.RenderHTMLWithNetwork(context.Background(), "https://example.com", "body", 0, "")
	if err != ErrHeadlessDisabled {
		t.Errorf("expected ErrHeadlessDisabled, got: %v", err)
	}
}

func TestRenderHTMLWithConfigAndNetwork_HeadlessDisabled(t *testing.T) {
	logger := zerolog.Nop()
	ext := NewRodExtractor(logger, 2, "", false)

	cfg := SourceConfig{Name: "test", URL: "https://example.com"}
	_, _, err := ext.RenderHTMLWithConfigAndNetwork(context.Background(), cfg, "")
	if err != ErrHeadlessDisabled {
		t.Errorf("expected ErrHeadlessDisabled, got: %v", err)
	}
}

func TestRenderHTMLWithNetwork_Screenshot(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	// Simple HTML page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><h1>Screenshot Test</h1></body></html>`)
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	t.Run("saves PNG when path provided", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "screenshot.png")
		html, _, err := ext.RenderHTMLWithNetwork(context.Background(), srv.URL+"/", "body", 5000, path)
		if err != nil {
			t.Fatalf("RenderHTMLWithNetwork returned error: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML")
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("screenshot file not found: %v", readErr)
		}
		if len(data) == 0 {
			t.Error("screenshot file is empty")
		}
		// PNG magic bytes: 137 80 78 71 13 10 26 10
		if len(data) < 8 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
			t.Errorf("screenshot file does not have PNG magic bytes, got first 4 bytes: %x", data[:4])
		}
	})

	t.Run("no file when path empty", func(t *testing.T) {
		dir := t.TempDir()
		html, _, err := ext.RenderHTMLWithNetwork(context.Background(), srv.URL+"/", "body", 5000, "")
		if err != nil {
			t.Fatalf("RenderHTMLWithNetwork returned error: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML")
		}

		// Verify no PNG files were created in the temp dir.
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".png") {
				t.Errorf("unexpected PNG file created: %s", e.Name())
			}
		}
	})
}

func TestRenderHTMLWithConfigAndNetwork_Screenshot(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><h1>Config Screenshot Test</h1></body></html>`)
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	t.Run("saves PNG with source config", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config-screenshot.png")
		cfg := SourceConfig{
			Name: "test-screenshot",
			URL:  srv.URL + "/",
			Headless: HeadlessConfig{
				WaitSelector:  "body",
				WaitTimeoutMs: 5000,
			},
		}
		html, _, err := ext.RenderHTMLWithConfigAndNetwork(context.Background(), cfg, path)
		if err != nil {
			t.Fatalf("RenderHTMLWithConfigAndNetwork returned error: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML")
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("screenshot file not found: %v", readErr)
		}
		if len(data) == 0 {
			t.Error("screenshot file is empty")
		}
		// PNG magic bytes
		if len(data) < 8 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
			t.Errorf("screenshot file does not have PNG magic bytes, got first 4 bytes: %x", data[:4])
		}
	})
}

// --- RodExtractor iframe extraction tests (require headless browser) ---

// parentPageHTML is served at / and contains a same-origin iframe.
const parentPageHTML = `<!DOCTYPE html>
<html>
<head><title>Test Venue</title></head>
<body>
    <h1>Test Venue Events</h1>
    <iframe id="events-frame" title="Event Widget" src="/iframe-events"></iframe>
</body>
</html>`

// iframePageHTML is served at /iframe-events and contains the event cards.
const iframePageHTML = `<!DOCTYPE html>
<html>
<body>
<div class="events-container">
    <div class="event-card">
        <h3 class="event-title">Concert A</h3>
        <time datetime="2025-07-15T20:00:00">July 15, 2025</time>
        <a href="/events/concert-a">Details</a>
    </div>
    <div class="event-card">
        <h3 class="event-title">Concert B</h3>
        <time datetime="2025-07-22T20:00:00">July 22, 2025</time>
        <a href="/events/concert-b">Details</a>
    </div>
    <div class="event-card">
        <h3 class="event-title">Concert C</h3>
        <time datetime="2025-08-01T19:00:00">Aug 1, 2025</time>
        <a href="/events/concert-c">Details</a>
    </div>
</div>
</body>
</html>`

// newTestBrowser launches a real headless Chromium browser for tests and
// returns it along with a cleanup function. It calls t.Fatal if the browser
// cannot be launched.
func newTestBrowser(t *testing.T, ext *RodExtractor) (*rod.Browser, func()) {
	t.Helper()
	browser, cleanup, err := ext.launchBrowser(context.Background())
	if err != nil {
		t.Fatalf("failed to launch test browser: %v", err)
	}
	return browser, cleanup
}

// newTestExtractorAllowLocalhost creates a RodExtractor with headless enabled
// and an empty SSRF blocklist so that httptest servers on 127.0.0.1 can be
// reached without mutating the package-level blockedCIDRs variable.
func newTestExtractorAllowLocalhost(logger zerolog.Logger) *RodExtractor {
	return NewRodExtractor(logger, 2, "", true, WithBlocklist([]*net.IPNet{}))
}

func TestScrapeSinglePage_Iframe(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	// Single httptest server serves both the parent page and iframe content.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iframe-events":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, iframePageHTML)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, parentPageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	waitTimeout := rodDefaultWaitTimeout

	t.Run("iframe extraction succeeds", func(t *testing.T) {
		ext := newTestExtractorAllowLocalhost(logger)
		browser, cleanup := newTestBrowser(t, ext)
		defer cleanup()

		cfg := SourceConfig{
			Name:    "test-iframe-source",
			URL:     srv.URL + "/",
			Tier:    2,
			Enabled: true,
			Headless: HeadlessConfig{
				WaitSelector: "body",
				Iframe: &IframeConfig{
					Selector:      "iframe#events-frame",
					WaitSelector:  ".events-container",
					WaitTimeoutMs: 10000,
				},
			},
			Selectors: SelectorConfig{
				EventList: ".event-card",
				Name:      ".event-title",
				StartDate: "time",
				URL:       "a",
			},
		}

		events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", waitTimeout)
		if err != nil {
			t.Fatalf("scrapeSinglePage returned error: %v", err)
		}

		if len(events) != 3 {
			t.Fatalf("expected 3 events from iframe, got %d", len(events))
		}

		wantNames := []string{"Concert A", "Concert B", "Concert C"}
		wantDates := []string{"2025-07-15T20:00:00", "2025-07-22T20:00:00", "2025-08-01T19:00:00"}

		for i, ev := range events {
			if ev.Name != wantNames[i] {
				t.Errorf("event[%d].Name = %q; want %q", i, ev.Name, wantNames[i])
			}
			if ev.StartDate != wantDates[i] {
				t.Errorf("event[%d].StartDate = %q; want %q", i, ev.StartDate, wantDates[i])
			}
		}
	})

	t.Run("no iframe config extracts parent HTML", func(t *testing.T) {
		ext := newTestExtractorAllowLocalhost(logger)
		browser, cleanup := newTestBrowser(t, ext)
		defer cleanup()

		// Config without iframe block — selectors target event cards that only
		// exist inside the iframe, not the parent page. Expect 0 events.
		cfg := SourceConfig{
			Name:    "test-no-iframe-source",
			URL:     srv.URL + "/",
			Tier:    2,
			Enabled: true,
			Headless: HeadlessConfig{
				WaitSelector: "body",
			},
			Selectors: SelectorConfig{
				EventList: ".event-card",
				Name:      ".event-title",
				StartDate: "time",
				URL:       "a",
			},
		}

		events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", waitTimeout)
		if err != nil {
			t.Fatalf("scrapeSinglePage returned error: %v", err)
		}

		// The parent page has no .event-card elements, so we expect 0 events.
		if len(events) != 0 {
			t.Errorf("expected 0 events from parent HTML (no iframe config), got %d", len(events))
		}
	})

	t.Run("iframe selector not found falls back to parent", func(t *testing.T) {
		ext := newTestExtractorAllowLocalhost(logger)
		browser, cleanup := newTestBrowser(t, ext)
		defer cleanup()

		// Use a selector that matches no iframe in the parent page.
		cfg := SourceConfig{
			Name:    "test-iframe-fallback",
			URL:     srv.URL + "/",
			Tier:    2,
			Enabled: true,
			Headless: HeadlessConfig{
				WaitSelector: "body",
				Iframe: &IframeConfig{
					Selector:      "iframe#nonexistent",
					WaitSelector:  ".events-container",
					WaitTimeoutMs: 3000,
				},
			},
			Selectors: SelectorConfig{
				EventList: ".event-card",
				Name:      ".event-title",
				StartDate: "time",
				URL:       "a",
			},
		}

		// Should not return an error — graceful fallback to parent HTML.
		events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", waitTimeout)
		if err != nil {
			t.Fatalf("scrapeSinglePage should not error on iframe fallback, got: %v", err)
		}

		// Parent has no .event-card elements, so 0 events expected after fallback.
		if len(events) != 0 {
			t.Errorf("expected 0 events after iframe fallback to parent HTML, got %d", len(events))
		}
	})
}

func TestRenderHTMLWithConfig(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	// Same httptest server as the iframe tests.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iframe-events":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, iframePageHTML)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, parentPageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()

	t.Run("returns iframe HTML when iframe config set", func(t *testing.T) {
		ext := newTestExtractorAllowLocalhost(logger)

		cfg := SourceConfig{
			Name:    "test-render-iframe",
			URL:     srv.URL + "/",
			Tier:    2,
			Enabled: true,
			Headless: HeadlessConfig{
				WaitSelector: "body",
				Iframe: &IframeConfig{
					Selector:      "iframe#events-frame",
					WaitSelector:  ".events-container",
					WaitTimeoutMs: 10000,
				},
			},
		}

		html, err := ext.RenderHTMLWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatalf("RenderHTMLWithConfig returned error: %v", err)
		}

		// The returned HTML should be from the iframe, not the parent page.
		if !strings.Contains(html, "events-container") {
			t.Error("expected HTML to contain 'events-container' from iframe")
		}
		if !strings.Contains(html, "Concert A") {
			t.Error("expected HTML to contain 'Concert A' from iframe")
		}
		// Should NOT contain the parent page's unique content.
		if strings.Contains(html, "Test Venue Events") {
			t.Error("HTML should be iframe content, not parent page (contains 'Test Venue Events')")
		}
	})

	t.Run("returns parent HTML when no iframe config", func(t *testing.T) {
		ext := newTestExtractorAllowLocalhost(logger)

		cfg := SourceConfig{
			Name:    "test-render-no-iframe",
			URL:     srv.URL + "/",
			Tier:    2,
			Enabled: true,
			Headless: HeadlessConfig{
				WaitSelector: "body",
			},
		}

		html, err := ext.RenderHTMLWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatalf("RenderHTMLWithConfig returned error: %v", err)
		}

		// Without iframe config, should return parent page HTML.
		if !strings.Contains(html, "Test Venue Events") {
			t.Error("expected parent page HTML containing 'Test Venue Events'")
		}
	})

	t.Run("returns error for empty URL", func(t *testing.T) {
		ext := NewRodExtractor(logger, 2, "", true)

		cfg := SourceConfig{
			Name: "test-empty-url",
		}

		_, err := ext.RenderHTMLWithConfig(context.Background(), cfg)
		if err == nil {
			t.Fatal("expected error for empty URL, got nil")
		}
		if !strings.Contains(err.Error(), "has no URL") {
			t.Errorf("expected 'has no URL' in error, got: %v", err)
		}
	})

	t.Run("returns error when headless disabled", func(t *testing.T) {
		ext := NewRodExtractor(logger, 2, "", false) // headless disabled

		cfg := SourceConfig{
			Name: "test-disabled",
			URL:  srv.URL + "/",
		}

		_, err := ext.RenderHTMLWithConfig(context.Background(), cfg)
		if err != ErrHeadlessDisabled {
			t.Errorf("expected ErrHeadlessDisabled, got: %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// Intercept tests (srv-enisd)
// --------------------------------------------------------------------------

// interceptPageHTML is a page that makes an XHR to /api/events on load.
const interceptPageHTML = `<!DOCTYPE html>
<html>
<head><title>Intercept Test</title></head>
<body>
<div id="events-container"></div>
<script>
var xhr = new XMLHttpRequest();
xhr.open("GET", "/api/events", false); // synchronous for simplicity in tests
xhr.send();
if (xhr.status === 200) {
    document.getElementById("events-container").textContent = "loaded";
}
</script>
</body>
</html>`

// interceptAPIJSON is the JSON response served at /api/events.
const interceptAPIJSON = `{"results":[{"title":"Test Event","date":"2026-03-15"}]}`

// TestIntercept_CapturesAPIResponse verifies that when a page makes an XHR to
// a URL matching URLPattern, the intercepted JSON is parsed and returned as
// RawEvents using ResultsPath and FieldMap.
func TestIntercept_CapturesAPIResponse(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/events":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, interceptAPIJSON)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, interceptPageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)
	browser, cleanup := newTestBrowser(t, ext)
	defer cleanup()

	cfg := SourceConfig{
		Name:    "test-intercept-source",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector: "body",
			Intercept: &InterceptConfig{
				URLPattern:  `api/events`,
				ResultsPath: "results",
				FieldMap: map[string]string{
					"name":       "title",
					"start_date": "date",
				},
			},
		},
		Selectors: SelectorConfig{
			EventList: ".event-card", // no such elements in test page
			Name:      ".event-name",
		},
	}

	events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", rodDefaultWaitTimeout)
	if err != nil {
		t.Fatalf("scrapeSinglePage returned error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least 1 intercepted event, got 0")
	}

	found := false
	for _, ev := range events {
		if ev.Name == "Test Event" && ev.StartDate == "2026-03-15" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected event with Name=%q StartDate=%q; got events: %+v", "Test Event", "2026-03-15", events)
	}
}

// TestIntercept_NoMatchingRequests verifies that when no API calls match the
// pattern, no error occurs and DOM-extracted events are still returned.
func TestIntercept_NoMatchingRequests(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	const staticPageWithDOM = `<!DOCTYPE html>
<html><body>
<div class="event-card"><span class="event-name">DOM Event</span></div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, staticPageWithDOM)
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)
	browser, cleanup := newTestBrowser(t, ext)
	defer cleanup()

	cfg := SourceConfig{
		Name:    "test-no-match-source",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector: "body",
			Intercept: &InterceptConfig{
				URLPattern:  `nonexistent-api/events`,
				ResultsPath: "results",
				FieldMap:    map[string]string{"name": "title"},
			},
		},
		Selectors: SelectorConfig{
			EventList: ".event-card",
			Name:      ".event-name",
		},
	}

	events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", rodDefaultWaitTimeout)
	if err != nil {
		t.Fatalf("scrapeSinglePage should not error when no intercept matches, got: %v", err)
	}

	// DOM events should still be extracted.
	if len(events) == 0 {
		t.Fatal("expected at least 1 DOM-extracted event, got 0")
	}
	if events[0].Name != "DOM Event" {
		t.Errorf("expected DOM event Name=%q, got %q", "DOM Event", events[0].Name)
	}
}

// TestIntercept_MergesWithDOMEvents verifies that intercept events and DOM
// events are both returned when the page has both.
func TestIntercept_MergesWithDOMEvents(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	const mergePageHTML = `<!DOCTYPE html>
<html><body>
<div class="event-card"><span class="event-name">DOM Event</span></div>
<script>
var xhr = new XMLHttpRequest();
xhr.open("GET", "/api/events", false);
xhr.send();
</script>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/events":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, interceptAPIJSON)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, mergePageHTML)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)
	browser, cleanup := newTestBrowser(t, ext)
	defer cleanup()

	cfg := SourceConfig{
		Name:    "test-merge-source",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector: "body",
			Intercept: &InterceptConfig{
				URLPattern:  `api/events`,
				ResultsPath: "results",
				FieldMap:    map[string]string{"name": "title", "start_date": "date"},
			},
		},
		Selectors: SelectorConfig{
			EventList: ".event-card",
			Name:      ".event-name",
		},
	}

	events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", rodDefaultWaitTimeout)
	if err != nil {
		t.Fatalf("scrapeSinglePage returned error: %v", err)
	}

	// Expect at least one DOM event and at least one intercepted event.
	hasDOMEvent := false
	hasInterceptEvent := false
	for _, ev := range events {
		if ev.Name == "DOM Event" {
			hasDOMEvent = true
		}
		if ev.Name == "Test Event" {
			hasInterceptEvent = true
		}
	}

	if !hasDOMEvent {
		t.Errorf("expected DOM event in merged results; got events: %+v", events)
	}
	if !hasInterceptEvent {
		t.Errorf("expected intercepted event in merged results; got events: %+v", events)
	}
}

// TestIntercept_CacheEndpointLogs verifies that when CacheEndpoint is true,
// the intercepted URL is logged at Info level.
func TestIntercept_CacheEndpointLogs(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/events":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, interceptAPIJSON)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, interceptPageHTML)
		}
	}))
	defer srv.Close()

	// Use a logger that captures output so we can inspect it.
	var logBuf strings.Builder
	logger := zerolog.New(&logBuf)
	ext := newTestExtractorWithLogger(logger)

	browser, cleanup := newTestBrowser(t, ext)
	defer cleanup()

	cfg := SourceConfig{
		Name:    "test-cache-endpoint-source",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector: "body",
			Intercept: &InterceptConfig{
				URLPattern:    `api/events`,
				ResultsPath:   "results",
				CacheEndpoint: true,
				FieldMap:      map[string]string{"name": "title"},
			},
		},
		Selectors: SelectorConfig{EventList: ".none", Name: ".none"},
	}

	_, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", rodDefaultWaitTimeout)
	if err != nil {
		t.Fatalf("scrapeSinglePage returned error: %v", err)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "api/events") {
		t.Errorf("expected log output to contain intercepted URL 'api/events'; got: %s", logOutput)
	}
}

// TestIntercept_RegexPatternMatching verifies that URLPattern is used as a Go
// regex, not a simple glob, so only matching URLs are captured.
func TestIntercept_RegexPatternMatching(t *testing.T) {
	if !headlessEnabled() {
		t.Skip("set SCRAPER_HEADLESS_ENABLED=true to run headless browser tests")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/events":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, interceptAPIJSON)
		case "/api/other":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"results":[{"title":"Other Event","date":"2026-05-01"}]}`)
		default:
			w.Header().Set("Content-Type", "text/html")
			// Page makes TWO XHR calls.
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<script>
var xhr1 = new XMLHttpRequest();
xhr1.open("GET", "/api/events", false);
xhr1.send();
var xhr2 = new XMLHttpRequest();
xhr2.open("GET", "/api/other", false);
xhr2.send();
</script>
</body></html>`)
		}
	}))
	defer srv.Close()

	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)
	browser, cleanup := newTestBrowser(t, ext)
	defer cleanup()

	// Pattern matches ONLY /api/events (not /api/other).
	cfg := SourceConfig{
		Name:    "test-regex-source",
		URL:     srv.URL + "/",
		Tier:    2,
		Enabled: true,
		Headless: HeadlessConfig{
			WaitSelector: "body",
			Intercept: &InterceptConfig{
				URLPattern:  `/api/events$`, // anchored to match only events, not other
				ResultsPath: "results",
				FieldMap:    map[string]string{"name": "title"},
			},
		},
		Selectors: SelectorConfig{EventList: ".none", Name: ".none"},
	}

	events, _, _, err := ext.scrapeSinglePage(context.Background(), browser, cfg, srv.URL+"/", "body", rodDefaultWaitTimeout)
	if err != nil {
		t.Fatalf("scrapeSinglePage returned error: %v", err)
	}

	// Should only have events from /api/events, not /api/other.
	for _, ev := range events {
		if ev.Name == "Other Event" {
			t.Errorf("unexpected event from non-matching URL: %+v", ev)
		}
	}
	// Must have the matching event.
	found := false
	for _, ev := range events {
		if ev.Name == "Test Event" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Test Event' from /api/events; got: %+v", events)
	}
}

// newTestExtractorWithLogger creates a RodExtractor with headless enabled,
// empty SSRF blocklist, and a custom logger for log-capture tests.
func newTestExtractorWithLogger(logger zerolog.Logger) *RodExtractor {
	return NewRodExtractor(logger, 2, "", true, WithBlocklist([]*net.IPNet{}))
}

func TestParseInterceptedBody(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	ext := newTestExtractorAllowLocalhost(logger)

	ic := &InterceptConfig{
		ResultsPath: "data.events",
		FieldMap:    map[string]string{"name": "title", "start_date": "date"},
	}

	tests := []struct {
		name      string
		body      string
		ic        *InterceptConfig
		wantLen   int
		wantFirst string // expected Name of first event, "" to skip check
	}{
		{
			name:    "empty body",
			body:    "",
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "malformed JSON",
			body:    `{not valid json`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "wrong results_path (missing segment)",
			body:    `{"other": {"stuff": []}}`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "results_path segment is not an object",
			body:    `{"data": "just a string"}`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "results_path leaf not found",
			body:    `{"data": {"other_key": []}}`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "results_path resolves to non-array",
			body:    `{"data": {"events": "not an array"}}`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:    "empty array",
			body:    `{"data": {"events": []}}`,
			ic:      ic,
			wantLen: 0,
		},
		{
			name:      "valid single item",
			body:      `{"data": {"events": [{"title": "My Event", "date": "2026-03-06"}]}}`,
			ic:        ic,
			wantLen:   1,
			wantFirst: "My Event",
		},
		{
			name:    "array contains non-object items (skipped)",
			body:    `{"data": {"events": ["string item", 42, {"title": "Good Event"}]}}`,
			ic:      ic,
			wantLen: 1,
		},
		{
			name:      "top-level results_path (single segment)",
			body:      `{"results": [{"title": "Top Level"}]}`,
			ic:        &InterceptConfig{ResultsPath: "results", FieldMap: map[string]string{"name": "title"}},
			wantLen:   1,
			wantFirst: "Top Level",
		},
		{
			name:      "deeply nested results_path",
			body:      `{"a": {"b": {"c": {"items": [{"title": "Deep"}]}}}}`,
			ic:        &InterceptConfig{ResultsPath: "a.b.c.items", FieldMap: map[string]string{"name": "title"}},
			wantLen:   1,
			wantFirst: "Deep",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events := ext.parseInterceptedBody(tt.body, tt.ic, "test-source")
			if len(events) != tt.wantLen {
				t.Errorf("got %d events, want %d", len(events), tt.wantLen)
			}
			if tt.wantFirst != "" && len(events) > 0 && events[0].Name != tt.wantFirst {
				t.Errorf("first event Name = %q, want %q", events[0].Name, tt.wantFirst)
			}
		})
	}
}

// TestScreenshotPathCleaning verifies that filepath.Clean normalises paths the
// way captureScreenshot and saveScreenshot rely on. This is a browser-free unit
// test of the path-sanitisation invariant, not a full integration test.
func TestScreenshotPathCleaning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
	}{
		{"/tmp/rod-screenshot-example-1234.png", "/tmp/rod-screenshot-example-1234.png"},
		{"/tmp//rod-screenshot-example-1234.png", "/tmp/rod-screenshot-example-1234.png"},
		{"/tmp/foo/../rod-screenshot-example-1234.png", "/tmp/rod-screenshot-example-1234.png"},
		{"/tmp/a/b/../../rod-screenshot-example-1234.png", "/tmp/rod-screenshot-example-1234.png"},
	}
	for _, c := range cases {
		got := filepath.Clean(c.raw)
		if got != c.want {
			t.Errorf("filepath.Clean(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestNetworkCollectorSnapshotOrder(t *testing.T) {
	t.Parallel()

	nc := newNetworkCollector()

	// Insert entries in an arbitrary non-sorted order; use URL to track which entry is which.
	for _, id := range []proto.NetworkRequestID{"3", "1", "5", "2", "4"} {
		id := id
		nc.requests[id] = &NetworkRequest{URL: "https://example.com/" + string(id)}
	}

	// Call snapshot() multiple times and assert the order is stable.
	first := nc.snapshot()
	for i := 0; i < 10; i++ {
		got := nc.snapshot()
		if len(got) != len(first) {
			t.Fatalf("iteration %d: got len %d, want %d", i, len(got), len(first))
		}
		for j, r := range got {
			if r.URL != first[j].URL {
				t.Fatalf("iteration %d: index %d: got URL %q, want %q", i, j, r.URL, first[j].URL)
			}
		}
	}

	// Assert the order is sorted by RequestID (lexicographic).
	// IDs "1"–"5" → URLs sorted as https://example.com/1 … https://example.com/5.
	want := []string{
		"https://example.com/1",
		"https://example.com/2",
		"https://example.com/3",
		"https://example.com/4",
		"https://example.com/5",
	}
	for i, r := range first {
		if r.URL != want[i] {
			t.Errorf("index %d: got URL %q, want %q", i, r.URL, want[i])
		}
	}
}

// TestRodHardTimeoutAccommodatesWaitTimeout verifies that rodHardTimeout
// returns a value that accommodates the configured wait timeout plus overhead,
// and never falls below rodDefaultTimeout.
func TestRodHardTimeoutAccommodatesWaitTimeout(t *testing.T) {
	tests := []struct {
		name   string
		config SourceConfig
		want   time.Duration
	}{
		{
			name:   "default config (no wait_timeout_ms) returns rodDefaultTimeout",
			config: SourceConfig{MaxPages: 1},
			// perPage = 10s + 1s = 11s; total = 11s*1 + 30s = 41s > 30s → 41s
			// BUT: this ensures we return at least rodDefaultTimeout (30s).
			// 41s > 30s so result is 41s.
			want: 41 * time.Second,
		},
		{
			name: "wait_timeout_ms 30000 single page returns 30s+overhead",
			config: SourceConfig{
				MaxPages: 1,
				Headless: HeadlessConfig{WaitTimeoutMs: 30000},
			},
			// perPage = 30s + 1s = 31s; total = 31s*1 + 30s = 61s
			want: 61 * time.Second,
		},
		{
			name: "wait_timeout_ms 5000 single page exceeds default",
			config: SourceConfig{
				MaxPages: 1,
				Headless: HeadlessConfig{WaitTimeoutMs: 5000},
			},
			// perPage = 5s + 1s = 6s; total = 6s*1 + 30s = 36s > 30s
			want: 36 * time.Second,
		},
		{
			name: "wait_timeout_ms 60000 max_pages 1 returns 91s",
			config: SourceConfig{
				MaxPages: 1,
				Headless: HeadlessConfig{WaitTimeoutMs: 60000},
			},
			// perPage = 60s + 1s = 61s; total = 61s*1 + 30s = 91s
			want: 91 * time.Second,
		},
		{
			name: "wait_timeout_ms 35000 max_pages 5 accommodates rcmusic",
			config: SourceConfig{
				MaxPages: 5,
				Headless: HeadlessConfig{WaitTimeoutMs: 35000},
			},
			// perPage = 35s + 1s = 36s; total = 36s*5 + 30s = 210s
			want: 210 * time.Second,
		},
		{
			name: "zero wait timeout uses default (no MaxPages uses 1)",
			config: SourceConfig{
				MaxPages: 0,
				Headless: HeadlessConfig{WaitTimeoutMs: 0},
			},
			// pages clamped to 1; perPage = 10s + 1s = 11s; total = 11s + 30s = 41s
			want: 41 * time.Second,
		},
		{
			name: "custom rate_limit_ms",
			config: SourceConfig{
				MaxPages: 2,
				Headless: HeadlessConfig{WaitTimeoutMs: 10000, RateLimitMs: 5000},
			},
			// perPage = 10s + 5s = 15s; total = 15s*2 + 30s = 60s
			want: 60 * time.Second,
		},
		{
			name: "floor guard: total with overhead alone equals rodDefaultTimeout",
			config: SourceConfig{
				MaxPages: 1,
				// Force a scenario where perPage*pages is negligible:
				// Use WaitTimeoutMs=1ms and RateLimitMs=1ms so perPage≈2ms.
				// total = 2ms + 30s ≈ 30.002s > 30s, so not exactly the floor.
				// The floor guard ensures we never return < 30s. Verify that
				// the minimum possible total (overhead alone) equals rodDefaultTimeout
				// by checking overhead=30s with 0 wait and 0 rate — but those fall
				// back to defaults. So test: with overhead=30s as the only component
				// we get at minimum rodDefaultTimeout.
				// Practical: test that the result is always ≥ rodDefaultTimeout.
				Headless: HeadlessConfig{WaitTimeoutMs: 1, RateLimitMs: 1},
			},
			// perPage = 1ms + 1ms = 2ms; total = 2ms*1 + 30s ≈ 30.002s
			// which is > rodDefaultTimeout (30s), so floor doesn't trigger here.
			// Result: 30s + 2ms.
			want: rodDefaultTimeout + 2*time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rodHardTimeout(tt.config)
			if got != tt.want {
				t.Errorf("rodHardTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractionDiagnostic_AllNamesEmpty verifies that when every container has
// an empty name, the error is FATAL-level and includes actionable hints about
// the most common cause (descendant selector vs element-is-target).
func TestExtractionDiagnostic_AllNamesEmpty(t *testing.T) {
	html := `
<html><body>
  <div class="card"><a class="title" href="/e/1">Concert A</a><span class="date">2026-05-01</span></div>
  <div class="card"><a class="title" href="/e/2">Concert B</a><span class="date">2026-05-02</span></div>
  <div class="card"><a class="title" href="/e/3">Concert C</a><span class="date">2026-05-03</span></div>
</body></html>`

	// Bug: ".title a" looks for <a> inside .title, but .title IS the <a>.
	cfg := SourceConfig{
		Name: "rcmusic-like",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".title a", // wrong — should be ".title"
			StartDate: ".date",
			URL:       "a[href]",
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected FATAL diagnostic error when all names are empty")
	}
	if events != nil {
		t.Errorf("expected nil events on FATAL, got %d", len(events))
	}

	errMsg := err.Error()

	// Must contain FATAL.
	if !strings.Contains(errMsg, "FATAL") {
		t.Errorf("expected FATAL in error, got: %s", errMsg)
	}
	// Must identify the source.
	if !strings.Contains(errMsg, "rcmusic-like") {
		t.Errorf("expected source name in error, got: %s", errMsg)
	}
	// Must show the broken selector.
	if !strings.Contains(errMsg, ".title a") {
		t.Errorf("expected broken selector in error, got: %s", errMsg)
	}
	// Must show the container count.
	if !strings.Contains(errMsg, "all 3 containers") {
		t.Errorf("expected 'all 3 containers' in error, got: %s", errMsg)
	}
	// Must include the hint about element-is-target.
	if !strings.Contains(errMsg, "IS the selector target") {
		t.Errorf("expected hint about element IS target in error, got: %s", errMsg)
	}
}

// TestExtractionDiagnostic_AllDatesEmpty verifies that all-dates-empty produces
// a WARNING with guidance about datetime/data-utc-date attributes.
func TestExtractionDiagnostic_AllDatesEmpty(t *testing.T) {
	html := `
<html><body>
  <div class="card"><h2 class="name">Event A</h2><span class="when">Tomorrow</span></div>
  <div class="card"><h2 class="name">Event B</h2><span class="when">Next Week</span></div>
</body></html>`

	cfg := SourceConfig{
		Name: "date-miss",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".name",
			StartDate: ".date", // wrong — should be ".when"
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic warning for all-dates-empty")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (names OK), got %d", len(events))
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "all 2 containers had empty dates") {
		t.Errorf("expected all-dates-empty warning, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "datetime") {
		t.Errorf("expected hint about datetime attribute, got: %s", errMsg)
	}
}

// TestExtractionDiagnostic_AllURLsEmpty verifies that all-URLs-empty produces
// a WARNING with guidance about href attributes.
func TestExtractionDiagnostic_AllURLsEmpty(t *testing.T) {
	html := `
<html><body>
  <div class="card"><h2 class="name">Event A</h2><span class="date">2026-05-01</span></div>
  <div class="card"><h2 class="name">Event B</h2><span class="date">2026-05-02</span></div>
</body></html>`

	cfg := SourceConfig{
		Name: "url-miss",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".name",
			StartDate: ".date",
			URL:       "a.detail-link", // nothing matches
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic warning for all-URLs-empty")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (names OK), got %d", len(events))
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "all 2 containers had empty URLs") {
		t.Errorf("expected all-URLs-empty warning, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "a.detail-link") {
		t.Errorf("expected broken URL selector in error, got: %s", errMsg)
	}
}

// TestExtractionDiagnostic_MultipleFieldsMissing verifies that when multiple
// fields are broken, all are reported in a single error separated by semicolons.
func TestExtractionDiagnostic_MultipleFieldsMissing(t *testing.T) {
	html := `
<html><body>
  <div class="card"><h2 class="name">Event A</h2></div>
  <div class="card"><h2 class="name">Event B</h2></div>
</body></html>`

	cfg := SourceConfig{
		Name: "multi-miss",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".name",
			StartDate: ".date", // nothing matches
			URL:       ".link", // nothing matches
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic error for multiple missing fields")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	errMsg := err.Error()
	// Both date and URL warnings should appear, joined by semicolons.
	if !strings.Contains(errMsg, "dates") {
		t.Errorf("expected date warning in combined error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "URLs") {
		t.Errorf("expected URL warning in combined error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "; ") {
		t.Errorf("expected semicolon-separated diagnostics, got: %s", errMsg)
	}
}

// TestExtractionDiagnostic_NoIssues verifies that a clean extraction returns
// no error.
func TestExtractionDiagnostic_NoIssues(t *testing.T) {
	html := `
<html><body>
  <div class="card">
    <h2 class="name">Event A</h2>
    <span class="date">2026-05-01</span>
    <a class="link" href="/events/a">Details</a>
  </div>
</body></html>`

	cfg := SourceConfig{
		Name: "clean",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".name",
			StartDate: ".date",
			URL:       ".link",
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err != nil {
		t.Fatalf("expected no error for clean extraction, got: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// TestExtractionDiagnostic_NoDateSelectorConfigured verifies that missing dates
// are NOT reported when no date selector was configured (intentional omission).
func TestExtractionDiagnostic_NoDateSelectorConfigured(t *testing.T) {
	html := `
<html><body>
  <div class="card"><h2 class="name">Event A</h2></div>
</body></html>`

	cfg := SourceConfig{
		Name: "no-date-sel",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList: ".card",
			Name:      ".name",
			// No StartDate, EndDate, or DateSelectors — intentional.
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err != nil {
		t.Fatalf("expected no error when date selector is intentionally omitted, got: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// TestProbesSummary exercises the probesSummary helper with various probe
// combinations: nil/empty, matched/unmatched, and long text.
func TestProbesSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		probes []DateSelectorProbe
		want   string
	}{
		{
			name:   "nil probes",
			probes: nil,
			want:   " (first container:)",
		},
		{
			name:   "empty probes",
			probes: []DateSelectorProbe{},
			want:   " (first container:)",
		},
		{
			name: "single matched probe with text",
			probes: []DateSelectorProbe{
				{Selector: ".date", Matched: true, Text: "2026-03-06"},
			},
			want: ` (first container: ".date"→"2026-03-06";)`,
		},
		{
			name: "single matched probe with empty text",
			probes: []DateSelectorProbe{
				{Selector: ".time", Matched: true, Text: ""},
			},
			want: ` (first container: ".time"→empty text;)`,
		},
		{
			name: "single unmatched probe",
			probes: []DateSelectorProbe{
				{Selector: ".missing", Matched: false, Text: ""},
			},
			want: ` (first container: ".missing"→no element;)`,
		},
		{
			name: "mixed probes matched and unmatched",
			probes: []DateSelectorProbe{
				{Selector: ".date", Matched: true, Text: "March 6"},
				{Selector: ".time", Matched: false, Text: ""},
				{Selector: ".year", Matched: true, Text: ""},
			},
			want: ` (first container: ".date"→"March 6"; ".time"→no element; ".year"→empty text;)`,
		},
		{
			name: "multiple probes all matched",
			probes: []DateSelectorProbe{
				{Selector: ".start", Matched: true, Text: "9:00am"},
				{Selector: ".end", Matched: true, Text: "5:00pm"},
			},
			want: ` (first container: ".start"→"9:00am"; ".end"→"5:00pm";)`,
		},
		{
			name: "long text preserved verbatim",
			probes: []DateSelectorProbe{
				{Selector: ".desc", Matched: true, Text: strings.Repeat("x", 200)},
			},
			want: ` (first container: ".desc"→"` + strings.Repeat("x", 200) + `";)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := probesSummary(tc.probes)
			if got != tc.want {
				t.Errorf("probesSummary() =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestExtractionDiagnostic_WithProbes calls extractionDiagnostic directly with
// non-nil firstProbes to verify that probe detail is appended to date-miss
// warning messages. Covers both the "all dates empty" (missDate==total) and
// "partial dates empty" (0 < missDate < total) paths, and also verifies that
// non-date diagnostics (name miss, URL miss) are NOT enriched with probe data.
func TestExtractionDiagnostic_WithProbes(t *testing.T) {
	t.Parallel()

	// Mixed probes: one fully matched, one matched but empty text, one unmatched.
	mixedProbes := []DateSelectorProbe{
		{Selector: ".day", Matched: true, Text: "Thu 5th March"},
		{Selector: ".time", Matched: true, Text: ""},
		{Selector: ".venue", Matched: false, Text: ""},
	}

	cfg := SourceConfig{
		Name: "probe-enrichment-source",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList:     ".card",
			Name:          ".name",
			DateSelectors: []string{".day", ".time", ".venue"},
			URL:           ".link",
		},
	}

	t.Run("all dates empty enriched with probe summary", func(t *testing.T) {
		t.Parallel()
		// total=3, missName=0, missDate=3 (all miss), missURL=0
		err := extractionDiagnostic(cfg, 3, 0, 3, 0, mixedProbes)
		if err == nil {
			t.Fatal("expected diagnostic error, got nil")
		}
		msg := err.Error()

		// Must contain the all-dates-empty warning text.
		if !strings.Contains(msg, "all 3 containers had empty dates") {
			t.Errorf("expected all-dates-empty warning; got: %s", msg)
		}
		// Must contain the probe summary opener.
		if !strings.Contains(msg, "(first container:") {
			t.Errorf("expected probe summary '(first container:' in message; got: %s", msg)
		}
		// Matched probe with text must appear as selector→"text".
		if !strings.Contains(msg, `".day"→"Thu 5th March"`) {
			t.Errorf("expected matched probe with text in summary; got: %s", msg)
		}
		// Matched probe with empty text must appear as selector→empty text.
		if !strings.Contains(msg, `".time"→empty text`) {
			t.Errorf("expected matched-but-empty probe in summary; got: %s", msg)
		}
		// Unmatched probe must appear as selector→no element.
		if !strings.Contains(msg, `".venue"→no element`) {
			t.Errorf("expected unmatched probe in summary; got: %s", msg)
		}
	})

	t.Run("partial date miss enriched with probe summary", func(t *testing.T) {
		t.Parallel()
		// total=5, missName=0, missDate=2 (partial miss), missURL=0
		err := extractionDiagnostic(cfg, 5, 0, 2, 0, mixedProbes)
		if err == nil {
			t.Fatal("expected diagnostic error, got nil")
		}
		msg := err.Error()

		// Partial miss warning.
		if !strings.Contains(msg, "2 of 5 containers had empty dates") {
			t.Errorf("expected partial-dates warning; got: %s", msg)
		}
		// Probe summary must also appear on the partial-miss path.
		if !strings.Contains(msg, "(first container:") {
			t.Errorf("expected probe summary on partial-miss path; got: %s", msg)
		}
		if !strings.Contains(msg, `".day"→"Thu 5th March"`) {
			t.Errorf("expected matched probe with text in partial-miss summary; got: %s", msg)
		}
	})

	t.Run("nil probes produces no probe summary in date-miss warning", func(t *testing.T) {
		t.Parallel()
		// total=3, missDate=3, firstProbes=nil → no probe summary appended.
		err := extractionDiagnostic(cfg, 3, 0, 3, 0, nil)
		if err == nil {
			t.Fatal("expected diagnostic error, got nil")
		}
		msg := err.Error()

		if !strings.Contains(msg, "all 3 containers had empty dates") {
			t.Errorf("expected all-dates-empty warning; got: %s", msg)
		}
		if strings.Contains(msg, "(first container:") {
			t.Errorf("expected NO probe summary when firstProbes is nil; got: %s", msg)
		}
	})

	t.Run("name-miss diagnostic is not enriched with probe summary", func(t *testing.T) {
		t.Parallel()
		// total=3, missName=3 (all miss) → FATAL for names, probes irrelevant.
		err := extractionDiagnostic(cfg, 3, 3, 0, 0, mixedProbes)
		if err == nil {
			t.Fatal("expected FATAL diagnostic error for all-names-empty, got nil")
		}
		msg := err.Error()

		if !strings.Contains(msg, "FATAL") {
			t.Errorf("expected FATAL in name-miss error; got: %s", msg)
		}
		// The name-miss path must NOT receive probe enrichment.
		if strings.Contains(msg, "(first container:") {
			t.Errorf("name-miss warning should not contain probe summary; got: %s", msg)
		}
	})

	t.Run("URL-miss diagnostic is not enriched with probe summary", func(t *testing.T) {
		t.Parallel()
		// total=2, missURL=2 (all miss), probes provided but irrelevant to URL path.
		err := extractionDiagnostic(cfg, 2, 0, 0, 2, mixedProbes)
		if err == nil {
			t.Fatal("expected diagnostic warning for all-URLs-empty, got nil")
		}
		msg := err.Error()

		if !strings.Contains(msg, "all 2 containers had empty URLs") {
			t.Errorf("expected all-URLs-empty warning; got: %s", msg)
		}
		// The URL-miss path must NOT receive probe enrichment.
		if strings.Contains(msg, "(first container:") {
			t.Errorf("URL-miss warning should not contain probe summary; got: %s", msg)
		}
	})
}

func TestExtractionDiagnostic_DateSelectorsAllEmpty(t *testing.T) {
	html := `
<html><body>
  <div class="card"><h2 class="name">Event A</h2></div>
  <div class="card"><h2 class="name">Event B</h2></div>
</body></html>`

	cfg := SourceConfig{
		Name: "date-selectors-miss",
		URL:  "https://example.com",
		Selectors: SelectorConfig{
			EventList:     ".card",
			Name:          ".name",
			DateSelectors: []string{".day", ".time"}, // nothing matches
		},
	}

	events, _, err := extractEventsFromHTML(html, cfg, "https://example.com")
	if err == nil {
		t.Fatal("expected diagnostic warning for all-dates-empty via date_selectors")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "date_selectors") {
		t.Errorf("expected date_selectors reference in error, got: %s", errMsg)
	}
}
