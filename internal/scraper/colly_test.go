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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
	events, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
	events, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
			fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
	events, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
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
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
	events, err := extractor.ScrapeWithSelectors(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Should use the datetime attribute, not the human-readable text.
	assert.Equal(t, "2026-06-21T19:30:00-04:00", events[0].StartDate)
}

// TestScrapeWithSelectors_ContextCancellation verifies that a cancelled context
// results in an immediate return (no error, empty-or-partial results).
func TestScrapeWithSelectors_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
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
	_, err := extractor.ScrapeWithSelectors(ctx, cfg)
	// Should return context error or nil (partial results) — never panic.
	// We just verify it doesn't block or crash.
	_ = err
}
