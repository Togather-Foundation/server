package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// newTestCollector returns a CollyExtractor with zero rate limit for fast tests.
func newTestExtractor() *CollyExtractor {
	return &CollyExtractor{
		userAgent: "TestBot/1.0",
		rateLimit: 0, // no delay in tests
		logger:    zerolog.Nop(),
	}
}

// TestScrapeWithSelectors_Basic verifies that a single-page listing with three
// event cards is scraped correctly.
func TestScrapeWithSelectors_Basic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Jazz Night</h2>
    <time class="date" datetime="2026-03-15">March 15, 2026</time>
    <span class="venue">The Rex Hotel</span>
    <p class="desc">A great jazz evening.</p>
    <a class="link" href="/events/jazz-night">More Info</a>
    <img class="img" src="/images/jazz.jpg" />
  </div>
  <div class="event-card">
    <h2 class="title">Art Opening</h2>
    <time class="date" datetime="2026-04-01">April 1, 2026</time>
    <span class="venue">Gallery 44</span>
    <p class="desc">Contemporary photography.</p>
    <a class="link" href="/events/art-opening">More Info</a>
  </div>
  <div class="event-card">
    <h2 class="title">Dance Workshop</h2>
    <time class="date" datetime="2026-04-10">April 10, 2026</time>
    <span class="venue">Harbourfront Centre</span>
    <p class="desc">Beginner dance workshop.</p>
    <a class="link" href="/events/dance">More Info</a>
  </div>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-source",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList:   "div.event-card",
			Name:        "h2.title",
			StartDate:   "time.date",
			Location:    "span.venue",
			Description: "p.desc",
			URL:         "a.link",
			Image:       "img.img",
		},
	}

	extractor := newTestExtractor()
	events, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// First event
	assert.Equal(t, "Jazz Night", events[0].Name)
	assert.Equal(t, "2026-03-15", events[0].StartDate)
	assert.Equal(t, "The Rex Hotel", events[0].Location)
	assert.Equal(t, "A great jazz evening.", events[0].Description)
	assert.True(t, strings.HasSuffix(events[0].URL, "/events/jazz-night"), "URL should end with /events/jazz-night, got: %s", events[0].URL)
	assert.True(t, strings.HasSuffix(events[0].Image, "/images/jazz.jpg"), "Image should end with /images/jazz.jpg, got: %s", events[0].Image)

	// Second event
	assert.Equal(t, "Art Opening", events[1].Name)
	assert.Equal(t, "2026-04-01", events[1].StartDate)
	assert.Equal(t, "Gallery 44", events[1].Location)

	// Third event
	assert.Equal(t, "Dance Workshop", events[2].Name)
	assert.Equal(t, "2026-04-10", events[2].StartDate)
}

// TestScrapeWithSelectors_EmptyName verifies that event cards with no name are
// skipped and not included in the results.
func TestScrapeWithSelectors_EmptyName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Valid Event</h2>
    <time class="date" datetime="2026-03-15">March 15, 2026</time>
  </div>
  <div class="event-card">
    <!-- No title element — should be skipped -->
    <time class="date" datetime="2026-03-20">March 20, 2026</time>
  </div>
  <div class="event-card">
    <h2 class="title">  </h2>
    <time class="date" datetime="2026-03-22">March 22, 2026</time>
  </div>
  <div class="event-card">
    <h2 class="title">Another Valid Event</h2>
    <time class="date" datetime="2026-04-01">April 1, 2026</time>
  </div>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-source",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			StartDate: "time.date",
		},
	}

	extractor := newTestExtractor()
	events, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 2, "expected only 2 events with non-empty names")

	assert.Equal(t, "Valid Event", events[0].Name)
	assert.Equal(t, "Another Valid Event", events[1].Name)
}

// TestScrapeWithSelectors_Pagination verifies that the scraper follows
// pagination links and collects events from multiple pages.
func TestScrapeWithSelectors_Pagination(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Page 1 — has a "next" link to page 2.
		// The pagination link href is set dynamically after the test server starts,
		// so we embed a path that will be resolved as absolute by Colly.
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Page 1 Event A</h2>
    <time class="date" datetime="2026-03-10">March 10, 2026</time>
  </div>
  <div class="event-card">
    <h2 class="title">Page 1 Event B</h2>
    <time class="date" datetime="2026-03-11">March 11, 2026</time>
  </div>
</div>
<a class="next-page" href="/events?page=2">Next</a>
</body></html>`)
	})

	mux.HandleFunc("/events/", func(w http.ResponseWriter, r *http.Request) {
		// Handles /events?page=2 (Go mux routes by path)
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Page 2 Event A</h2>
    <time class="date" datetime="2026-04-01">April 1, 2026</time>
  </div>
</div>
</body></html>`)
	})

	// Use a single handler that inspects the query param.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "2" {
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Page 2 Event A</h2>
    <time class="date" datetime="2026-04-01">April 1, 2026</time>
  </div>
  <div class="event-card">
    <h2 class="title">Page 2 Event B</h2>
    <time class="date" datetime="2026-04-02">April 2, 2026</time>
  </div>
</div>
</body></html>`)
			return
		}
		// Default: page 1.
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="events">
  <div class="event-card">
    <h2 class="title">Page 1 Event A</h2>
    <time class="date" datetime="2026-03-10">March 10, 2026</time>
  </div>
  <div class="event-card">
    <h2 class="title">Page 1 Event B</h2>
    <time class="date" datetime="2026-03-11">March 11, 2026</time>
  </div>
</div>
<a class="next-page" href="?page=2">Next</a>
</body></html>`)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-paginated",
		URL:      ts.URL + "/events",
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList:  "div.event-card",
			Name:       "h2.title",
			StartDate:  "time.date",
			Pagination: "a.next-page",
		},
	}

	extractor := newTestExtractor()
	events, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)

	// Should have events from both pages.
	require.GreaterOrEqual(t, len(events), 3, "expected events from at least 2 pages")

	names := make(map[string]bool)
	for _, e := range events {
		names[e.Name] = true
	}
	assert.True(t, names["Page 1 Event A"], "missing Page 1 Event A")
	assert.True(t, names["Page 1 Event B"], "missing Page 1 Event B")
	assert.True(t, names["Page 2 Event A"], "missing Page 2 Event A")
}

// TestScrapeWithSelectors_DatetimeAttr verifies that datetime attributes are
// preferred over text content for date extraction.
func TestScrapeWithSelectors_DatetimeAttr(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card">
  <h2 class="title">Concert</h2>
  <time class="date" datetime="2026-06-21T19:30:00-04:00">June 21, 2026 at 7:30 PM</time>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-source",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			StartDate: "time.date",
		},
	}

	extractor := newTestExtractor()
	events, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Should use the datetime attribute, not the human-readable text.
	assert.Equal(t, "2026-06-21T19:30:00-04:00", events[0].StartDate)
}

// TestScrapeWithSelectors_ContextCancellation verifies that a cancelled context
// results in an immediate return (no error, empty-or-partial results).
func TestScrapeWithSelectors_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card"><h2 class="title">Event</h2></div>
</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	cfg := SourceConfig{
		Name:     "test-source",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
		},
	}

	extractor := newTestExtractor()
	_, _, err := extractor.ScrapeWithSelectors(ctx, cfg)
	// Should return context error or nil (partial results) — never panic.
	// We just verify it doesn't block or crash.
	_ = err
}

// TestWwwVariants verifies that wwwVariants produces both www and non-www forms.
func TestWwwVariants(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"example.com", []string{"example.com", "www.example.com"}},
		{"www.example.com", []string{"www.example.com", "example.com"}},
		{"sub.example.com", []string{"sub.example.com", "www.sub.example.com"}},
		{"www.sub.example.com", []string{"www.sub.example.com", "sub.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := wwwVariants(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScrapeWithSelectors_WwwRedirect verifies that a www → non-www redirect
// (or vice versa) is followed successfully instead of being blocked by
// Colly's AllowedDomains check.
func TestScrapeWithSelectors_WwwRedirect(t *testing.T) {
	// Target server: the "non-www" version that serves the actual content.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event"><h2 class="name">Redirected Event</h2></div>
</body></html>`)
	}))
	defer target.Close()

	// Redirect server: simulates www → non-www redirect.
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.RequestURI, http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	extractor := newTestExtractor()

	cfg := SourceConfig{
		Name: "redirect-test",
		URL:  redirect.URL, // start at the "www" version
		Tier: 1,
		Selectors: SelectorConfig{
			EventList: ".event",
			Name:      ".name",
		},
	}

	events, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 1, "redirect should be followed and events extracted")
	assert.Equal(t, "Redirected Event", events[0].Name)
}

// TestScrapeWithSelectors_DateSelectors verifies that the date_selectors
// grab-bag model works in the Colly (Tier 1) extractor: always-indexed
// DateParts and probe capture on the first event container.
func TestScrapeWithSelectors_DateSelectors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card">
  <h2 class="title">Jazz Night</h2>
  <span class="date-part">Thu 5th March</span>
  <span class="time-part">9:30 PM</span>
  <span class="venue">The Rex</span>
</div>
<div class="event-card">
  <h2 class="title">Art Opening</h2>
  <span class="date-part">Fri 6th March</span>
  <span class="time-part">7:00 PM</span>
  <span class="venue">Gallery 44</span>
</div>
<div class="event-card">
  <h2 class="title">Incomplete Event</h2>
  <span class="date-part">Sat 7th March</span>
  <!-- no time-part element -->
  <span class="venue">Harbourfront</span>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-date-selectors",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			Location:  "span.venue",
			DateSelectors: []string{
				"span.date-part",
				"span.time-part",
			},
		},
	}

	extractor := newTestExtractor()
	events, probes, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// First event: both selectors match.
	assert.Equal(t, "Jazz Night", events[0].Name)
	require.Len(t, events[0].DateParts, 2, "DateParts should always have 2 entries (one per selector)")
	assert.Equal(t, "Thu 5th March", events[0].DateParts[0])
	assert.Equal(t, "9:30 PM", events[0].DateParts[1])
	assert.Empty(t, events[0].StartDate, "StartDate should be empty when date_selectors is used")

	// Second event: both selectors match.
	assert.Equal(t, "Art Opening", events[1].Name)
	require.Len(t, events[1].DateParts, 2)
	assert.Equal(t, "Fri 6th March", events[1].DateParts[0])
	assert.Equal(t, "7:00 PM", events[1].DateParts[1])

	// Third event: only first selector matches.
	assert.Equal(t, "Incomplete Event", events[2].Name)
	require.Len(t, events[2].DateParts, 2, "DateParts should still have 2 entries for always-indexed")
	assert.Equal(t, "Sat 7th March", events[2].DateParts[0])
	assert.Empty(t, events[2].DateParts[1], "missing selector should produce empty string")

	// Probes: captured from first container only.
	require.Len(t, probes, 2, "probes should have 2 entries (one per date_selector)")
	assert.Equal(t, "span.date-part", probes[0].Selector)
	assert.True(t, probes[0].Matched)
	assert.Equal(t, "Thu 5th March", probes[0].Text)

	assert.Equal(t, "span.time-part", probes[1].Selector)
	assert.True(t, probes[1].Matched)
	assert.Equal(t, "9:30 PM", probes[1].Text)
}

// TestScrapeWithSelectors_DateSelectorsProbesMissing verifies that probes
// correctly report unmatched selectors in the first container.
func TestScrapeWithSelectors_DateSelectorsProbesMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card">
  <h2 class="title">Event One</h2>
  <span class="date-part">March 15</span>
  <!-- no time-part or end-date -->
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-probes-missing",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			DateSelectors: []string{
				"span.date-part",
				"span.time-part",
				"span.end-date",
			},
		},
	}

	extractor := newTestExtractor()
	events, probes, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Len(t, events[0].DateParts, 3, "always-indexed: 3 selectors = 3 DateParts")
	assert.Equal(t, "March 15", events[0].DateParts[0])
	assert.Empty(t, events[0].DateParts[1])
	assert.Empty(t, events[0].DateParts[2])

	// Probes should reflect the match status.
	require.Len(t, probes, 3)
	assert.True(t, probes[0].Matched)
	assert.Equal(t, "March 15", probes[0].Text)

	assert.False(t, probes[1].Matched)
	assert.Empty(t, probes[1].Text)

	assert.False(t, probes[2].Matched)
	assert.Empty(t, probes[2].Text)
}

// TestParseSelector verifies that parseSelector correctly splits a selector
// string into its CSS selector and optional attribute name components.
func TestParseSelector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSel  string
		wantAttr string
	}{
		{
			name:     "plain selector returns empty attribute",
			input:    "span.title",
			wantSel:  "span.title",
			wantAttr: "",
		},
		{
			name:     "selector with attribute",
			input:    "span.prices::data-event",
			wantSel:  "span.prices",
			wantAttr: "data-event",
		},
		{
			name:     "class-only selector with attribute",
			input:    ".card::data-url",
			wantSel:  ".card",
			wantAttr: "data-url",
		},
		{
			name:     "double-colon but no attribute name",
			input:    "div.thing::",
			wantSel:  "div.thing",
			wantAttr: "",
		},
		{
			name:     "empty string",
			input:    "",
			wantSel:  "",
			wantAttr: "",
		},
		{
			name:     "no dot, just tag with attribute",
			input:    "a::href",
			wantSel:  "a",
			wantAttr: "href",
		},
		{
			name:     "selector with multiple classes, no attribute",
			input:    "div.foo.bar",
			wantSel:  "div.foo.bar",
			wantAttr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSel, gotAttr := parseSelector(tt.input)
			assert.Equal(t, tt.wantSel, gotSel, "selector part")
			assert.Equal(t, tt.wantAttr, gotAttr, "attribute part")
		})
	}
}

// TestScrapeWithSelectors_DateSelectorsFallback verifies that when
// date_selectors is set, StartDate/EndDate are NOT populated (the
// date_selectors path takes priority).
func TestScrapeWithSelectors_DateSelectorsFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card">
  <h2 class="title">Dual Config Event</h2>
  <time class="date" datetime="2026-06-21">June 21, 2026</time>
  <span class="date-fragment">June 21</span>
  <span class="time-fragment">8:00 PM</span>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-fallback",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			StartDate: "time.date",
			DateSelectors: []string{
				"span.date-fragment",
				"span.time-fragment",
			},
		},
	}

	extractor := newTestExtractor()
	events, probes, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// date_selectors takes priority — StartDate should be empty.
	assert.Empty(t, events[0].StartDate, "date_selectors should take priority over StartDate")
	require.Len(t, events[0].DateParts, 2)
	assert.Equal(t, "June 21", events[0].DateParts[0])
	assert.Equal(t, "8:00 PM", events[0].DateParts[1])

	// Probes captured.
	require.Len(t, probes, 2)
	assert.True(t, probes[0].Matched)
	assert.True(t, probes[1].Matched)
}

// TestScrapeWithSelectors_MultiRowDateTable verifies that when date_selectors
// match multiple rows (e.g., table rows with dates in one column and times in another),
// the extractor emits one RawEvent per row with per-row DateParts.
//
// This is the core feature for multi-occurrence events like Luminato Festival:
// if a show detail page has a date table with 3 performances, we emit 3 RawEvents
// (each with the same name/location/description/URL/image, but different DateParts).
func TestScrapeWithSelectors_MultiRowDateTable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div id="show-detail">
  <h1 class="show-title">Hamlet</h1>
  <p class="venue-name">Princess of Wales Theatre</p>
  <table class="dmTable">
    <tr>
      <td class="cell">June 1</td>
      <td class="cell">7:30 PM</td>
    </tr>
    <tr>
      <td class="cell">June 2</td>
      <td class="cell">8:00 PM</td>
    </tr>
    <tr>
      <td class="cell">June 3</td>
      <td class="cell">7:30 PM</td>
    </tr>
  </table>
  <link rel="canonical" href="https://example.com/hamlet">
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-multi-row",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div#show-detail",
			Name:      "h1.show-title",
			Location:  "p.venue-name",
			URL:       "link[rel='canonical']",
			DateSelectors: []string{
				"table.dmTable tr td.cell:first-child",
				"table.dmTable tr td.cell:nth-child(2)",
			},
		},
	}

	extractor := newTestExtractor()
	events, probes, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)

	// Should emit 3 RawEvents (one per row).
	require.Len(t, events, 3, "expected 3 events (one per table row)")

	// All events share the same name, location, and URL.
	for i, ev := range events {
		assert.Equal(t, "Hamlet", ev.Name, "event %d: name mismatch", i)
		assert.Equal(t, "Princess of Wales Theatre", ev.Location, "event %d: location mismatch", i)
		assert.Equal(t, "https://example.com/hamlet", ev.URL, "event %d: URL mismatch", i) // h.Request.AbsoluteURL resolves relative URLs
	}

	// Each event has its own DateParts (from its row).
	expectedParts := [][]string{
		{"June 1", "7:30 PM"},
		{"June 2", "8:00 PM"},
		{"June 3", "7:30 PM"},
	}
	for i, expected := range expectedParts {
		require.Len(t, events[i].DateParts, 2, "event %d: DateParts length mismatch", i)
		assert.Equal(t, expected[0], events[i].DateParts[0], "event %d: date mismatch", i)
		assert.Equal(t, expected[1], events[i].DateParts[1], "event %d: time mismatch", i)
	}

	// Probes should be captured from the first row.
	require.Len(t, probes, 2, "expected 2 probes (one per date_selector)")
	assert.Equal(t, "table.dmTable tr td.cell:first-child", probes[0].Selector)
	assert.True(t, probes[0].Matched)
	assert.Equal(t, "June 1", probes[0].Text, "probe should show first row's date")

	assert.Equal(t, "table.dmTable tr td.cell:nth-child(2)", probes[1].Selector)
	assert.True(t, probes[1].Matched)
	assert.Equal(t, "7:30 PM", probes[1].Text, "probe should show first row's time")
}

// TestScrapeWithSelectors_SingleRowDateTable verifies backward compatibility:
// when date_selectors match only one row (or the old concatenated-text behavior),
// we emit a single RawEvent.
func TestScrapeWithSelectors_SingleRowDateTable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div class="event-card">
  <h2 class="title">One-Time Concert</h2>
  <span class="date">December 25</span>
  <span class="time">6:00 PM</span>
  <span class="venue">Concert Hall</span>
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-single-row",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 5,
		Selectors: SelectorConfig{
			EventList: "div.event-card",
			Name:      "h2.title",
			Location:  "span.venue",
			DateSelectors: []string{
				"span.date",
				"span.time",
			},
		},
	}

	extractor := newTestExtractor()
	events, probes, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)

	// Should emit 1 RawEvent.
	require.Len(t, events, 1)

	// Event has name, location, and DateParts from the single row.
	assert.Equal(t, "One-Time Concert", events[0].Name)
	assert.Equal(t, "Concert Hall", events[0].Location)
	require.Len(t, events[0].DateParts, 2)
	assert.Equal(t, "December 25", events[0].DateParts[0])
	assert.Equal(t, "6:00 PM", events[0].DateParts[1])

	// Probes captured.
	require.Len(t, probes, 2)
	assert.True(t, probes[0].Matched)
	assert.Equal(t, "December 25", probes[0].Text)
	assert.True(t, probes[1].Matched)
	assert.Equal(t, "6:00 PM", probes[1].Text)
}

// TestScrapeWithSelectors_MultiRowConsolidation tests the full tier 1 pipeline
// with multi-row extraction and consolidation into EventInput with Occurrences.
//
// This is an end-to-end test that verifies multi-occurrence events are correctly
// built by the normalizer and consolidation logic.
func TestScrapeWithSelectors_MultiRowConsolidation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
<div id="detail">
  <h1>Show Name</h1>
  <p class="loc">Theatre X</p>
  <p class="desc">A wonderful show</p>
  <table>
    <tr>
      <td class="date">2026-06-01</td>
      <td class="time">19:30</td>
    </tr>
    <tr>
      <td class="date">2026-06-02</td>
      <td class="time">19:30</td>
    </tr>
  </table>
  <link rel="canonical" href="https://example.com/show">
</div>
</body></html>`)
	}))
	defer ts.Close()

	cfg := SourceConfig{
		Name:     "test-consolidate",
		URL:      ts.URL,
		Tier:     1,
		MaxPages: 1,
		Timezone: "America/Toronto",
		Selectors: SelectorConfig{
			EventList:   "div#detail",
			Name:        "h1",
			Location:    "p.loc",
			Description: "p.desc",
			URL:         "link[rel='canonical']",
			DateSelectors: []string{
				"table tr td.date",
				"table tr td.time",
			},
		},
	}

	extractor := newTestExtractor()
	rawEvents, _, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, rawEvents, 2, "should extract 2 RawEvents (one per row)")

	// Now normalize and consolidate like the scraper does.
	eventGroups := make(map[string][]RawEvent)
	for _, raw := range rawEvents {
		key := fmt.Sprintf("%s|||%s", raw.URL, raw.Name)
		eventGroups[key] = append(eventGroups[key], raw)
	}

	require.Len(t, eventGroups, 1, "both RawEvents should group under the same key (same URL+Name)")

	var consolidated events.EventInput
	for _, group := range eventGroups {
		if len(group) > 1 {
			var consolidateErr error
			consolidated, consolidateErr = consolidateOccurrences(group, cfg)
			require.NoError(t, consolidateErr, "consolidation should succeed")
		}
	}

	// Verify consolidated event.
	assert.Equal(t, "Show Name", consolidated.Name)
	require.NotNil(t, consolidated.Location)
	assert.Equal(t, "Theatre X", consolidated.Location.Name)
	assert.Equal(t, "A wonderful show", consolidated.Description)
	assert.Equal(t, "https://example.com/show", consolidated.URL)

	// Verify occurrences.
	require.Len(t, consolidated.Occurrences, 2, "consolidated event should have 2 occurrences")

	// Each occurrence should have a start date.
	for i, occ := range consolidated.Occurrences {
		require.NotEmpty(t, occ.StartDate, "occurrence %d should have a start date", i)
		// Dates should be in RFC 3339 format (contains 'T' for datetime or is just YYYY-MM-DD)
		assert.True(t, strings.Contains(occ.StartDate, "2026-06-0"+(string(rune('1'+i)))) ||
			strings.Contains(occ.StartDate, "2026-06-0"+string(rune('1'+i))),
			"occurrence %d start date should contain 2026-06-0%d", i, 1+i)
	}
}
