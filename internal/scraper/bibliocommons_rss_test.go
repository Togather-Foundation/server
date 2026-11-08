package scraper

import (
	"context"
	"fmt"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readBCFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return data
}

// bcFeedServer serves page1 for ?page=1 (or no page) and the empty feed otherwise.
// It records every request URL.
func bcFeedServer(t *testing.T) (*httptest.Server, func() []*http.Request) {
	t.Helper()
	page1 := readBCFixture(t, "bibliocommons_rss_page1.xml")
	empty := readBCFixture(t, "bibliocommons_rss_empty.xml")
	var mu sync.Mutex
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Clone(context.Background()))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if p := r.URL.Query().Get("page"); p == "" || p == "1" {
			_, _ = w.Write(page1)
			return
		}
		_, _ = w.Write(empty)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return append([]*http.Request(nil), seen...)
	}
}

func newBCTestExtractor() *BiblioCommonsRSSExtractor {
	e := NewBiblioCommonsRSSExtractor(http.DefaultClient, 0, zerolog.Nop())
	e.pageDelay = 0
	return e
}

func bcTestSource(url string) SourceConfig {
	return SourceConfig{
		Name:       "tpl-career-job-search",
		URL:        url,
		TrustLevel: 8,
		License:    "CC0-1.0",
		Domain:     "education",
		Timezone:   "America/Toronto",
		MaxPages:   5,
	}
}

func TestBiblioCommonsRSS_MapsInPersonEvent(t *testing.T) {
	srv, _ := bcFeedServer(t)
	evs, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events?types=abc"))
	require.NoError(t, err)
	require.Len(t, evs, 3, "cancelled event must be skipped")

	e := evs[0]
	assert.Equal(t, "Finding a Job with ACCES Employment", e.Name)
	// Local time is used (UTC feed value is 22:00Z); October is EDT.
	assert.Equal(t, "2026-10-16T18:00:00-04:00", e.StartDate)
	assert.Equal(t, "2026-10-16T19:30:00-04:00", e.EndDate)
	assert.Equal(t, "https://tpl.bibliocommons.com/events/69ebdb675c04bc0fc0636d6d", e.URL)
	assert.Equal(t, "education", e.EventDomain)
	assert.Equal(t, "CC0-1.0", e.License)
	assert.False(t, e.AllDay)
	assert.True(t, strings.HasPrefix(e.Image, "https://tpl.bibliocommons.com/events/uploads/"), "image upgraded to https: %q", e.Image)

	require.NotNil(t, e.Location)
	assert.Equal(t, "Weston", e.Location.Name)
	assert.Equal(t, "2 King Street", e.Location.StreetAddress)
	assert.Equal(t, "Toronto", e.Location.AddressLocality)
	assert.Equal(t, "ON", e.Location.AddressRegion)
	assert.Equal(t, "M9N 1K9", e.Location.PostalCode)
	assert.Equal(t, "CA", e.Location.AddressCountry)
	assert.InDelta(t, 43.7017009, e.Location.Latitude, 1e-9)
	assert.InDelta(t, -79.5206752, e.Location.Longitude, 1e-9)
	assert.Nil(t, e.VirtualLocation)

	assert.Contains(t, e.Keywords, "Career & Job Search")
	assert.Contains(t, e.Keywords, "Adults (18+)")
	assert.NotContains(t, e.Keywords, "English", "language is not a keyword")
	assert.Equal(t, []string{"en"}, e.InLanguage)

	require.NotNil(t, e.Organizer)
	assert.Equal(t, "Toronto Public Library", e.Organizer.Name)

	require.NotNil(t, e.Source)
	assert.Equal(t, "69ebdb675c04bc0fc0636d6d", e.Source.EventID)
	assert.Equal(t, "tpl-career-job-search", e.Source.Name)

	// Description: plain text, no markup or entities, with room + registration facts.
	assert.Contains(t, e.Description, "Interview Preparation & Personal Branding")
	assert.Contains(t, e.Description, "answering behavioral questions")
	assert.Contains(t, e.Description, "Room: Program Room, Basement Level")
	assert.Contains(t, e.Description, "No registration required")
	assert.NotContains(t, e.Description, "<")
	assert.NotContains(t, e.Description, "&nbsp;")
	assert.NotContains(t, e.Description, " ")
}

func TestBiblioCommonsRSS_RegistrationAndBoilerplate(t *testing.T) {
	srv, _ := bcFeedServer(t)
	evs, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events"))
	require.NoError(t, err)

	e := evs[1]
	assert.Equal(t, "Using AI in Job Searching and Career Exploration", e.Name)
	assert.Equal(t, "2026-10-22T18:30:00-04:00", e.StartDate)
	assert.Contains(t, e.Description, "Registration required (27 spots remaining)")
	assert.Contains(t, e.Description, "practical, interactive workshop")
	assert.NotContains(t, e.Description, "Information Collection Notice", "privacy boilerplate is dropped")
	assert.NotContains(t, e.Description, "answerline@tpl.ca")
}

func TestBiblioCommonsRSS_VirtualEvent(t *testing.T) {
	srv, _ := bcFeedServer(t)
	evs, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events"))
	require.NoError(t, err)

	e := evs[2]
	assert.Equal(t, "Google Prompting Essentials: Online Learning Circle", e.Name)
	assert.Nil(t, e.Location, "virtual events have no physical place")
	require.NotNil(t, e.VirtualLocation)
	assert.Equal(t, "https://tpl.bibliocommons.com/events/6aa464b886d8450031156f87", e.VirtualLocation.URL)
	assert.Contains(t, e.Description, "Registration required — this event is full")
}

func TestBiblioCommonsRSS_CancelledEventSkippedWithWarning(t *testing.T) {
	srv, _ := bcFeedServer(t)
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events"))
	require.NoError(t, err)
	for _, e := range evs {
		assert.NotContains(t, e.Name, "Cancelled")
	}
	require.NotEmpty(t, warnings)
	assert.Contains(t, strings.Join(warnings, "\n"), "cancelled")
}

func TestBiblioCommonsRSS_RequestShapeAndPaginationStopsOnEmptyPage(t *testing.T) {
	srv, seen := bcFeedServer(t)
	_, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events?types=68b050d1023ff32800756704"))
	require.NoError(t, err)

	reqs := seen()
	require.Len(t, reqs, 2, "page 1 has events, page 2 is empty → stop")
	for i, r := range reqs {
		assert.Equal(t, "68b050d1023ff32800756704", r.URL.Query().Get("types"), "filter query is preserved")
		assert.Equal(t, []string{"1", "2"}[i], r.URL.Query().Get("page"))
		assert.Equal(t, ScraperUserAgent, r.Header.Get("User-Agent"), "identify honestly")
		assert.Contains(t, r.Header.Get("Accept"), "xml")
	}
}

func TestBiblioCommonsRSS_StopsWhenPageRepeats(t *testing.T) {
	// A server that ignores ?page and always returns the same items must not loop to MaxPages.
	page1 := readBCFixture(t, "bibliocommons_rss_page1.xml")
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		_, _ = w.Write(page1)
	}))
	defer srv.Close()

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 10
	evs, _, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, 3, "duplicates across pages are dropped")
	assert.Equal(t, 2, n, "second page added nothing new → stop")
}

func TestBiblioCommonsRSS_RespectsMaxPages(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		// Every page has one new, distinct event.
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:bc="http://bibliocommons.com/rss/1.0/modules/event/" version="2.0"><channel><title>Events | X Library</title>
<item><title>E` + r.URL.Query().Get("page") + `</title><link>https://x.example/events/` + r.URL.Query().Get("page") + `</link><guid>https://x.example/events/` + r.URL.Query().Get("page") + `</guid>
<bc:start_date_local>2026-11-01T10:00</bc:start_date_local><bc:end_date_local>2026-11-01T11:00</bc:end_date_local><bc:is_cancelled>false</bc:is_cancelled><bc:is_virtual>false</bc:is_virtual><bc:location><bc:name>Branch</bc:name><bc:street>Main St</bc:street></bc:location></item></channel></rss>`))
	}))
	defer srv.Close()

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 3
	evs, _, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, 3)
	assert.Equal(t, 3, n)
	assert.Equal(t, "X Library", evs[0].Organizer.Name, "organizer comes from the channel title")
}

func TestBiblioCommonsRSS_Errors(t *testing.T) {
	t.Run("HTTP 403", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
		defer srv.Close()
		_, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "403")
	})
	t.Run("not a feed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html><body>Just a moment...</body></html>"))
		}))
		defer srv.Close()
		_, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL))
		require.Error(t, err)
	})
	t.Run("oversized body", func(t *testing.T) {
		srv, _ := bcFeedServer(t)
		e := NewBiblioCommonsRSSExtractor(http.DefaultClient, 100, zerolog.Nop())
		e.pageDelay = 0
		_, _, err := e.Extract(context.Background(), bcTestSource(srv.URL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})
}

func TestHTMLToPlainText(t *testing.T) {
	got := htmlToPlainText(`<p>One &amp; two</p><p>Three<br>four&nbsp;five</p><ul><li>a</li><li>b</li></ul><script>x()</script>`)
	assert.Equal(t, "One & two\n\nThree\nfour five\n\na\nb", got)
}

func TestValidateConfig_BiblioCommonsRSSExtractionMethod(t *testing.T) {
	t.Parallel()
	base := func() SourceConfig {
		return SourceConfig{
			Name:             "tpl-career-job-search",
			URL:              "https://gateway.bibliocommons.com/v2/libraries/tpl/rss/events?types=68b050d1023ff32800756704",
			TrustLevel:       8,
			MaxPages:         5,
			Schedule:         "daily",
			ExtractionMethod: "bibliocommons_rss",
		}
	}

	t.Run("valid without selectors", func(t *testing.T) {
		t.Parallel()
		warnings, err := ValidateConfigWithWarnings(base())
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})
	t.Run("tier 1 and 3 do not demand selectors/rest", func(t *testing.T) {
		t.Parallel()
		for _, tier := range []int{1, 2, 3} {
			cfg := base()
			cfg.Tier = tier
			warnings, err := ValidateConfigWithWarnings(cfg)
			require.NoError(t, err, "tier %d", tier)
			require.NotEmpty(t, warnings, "tier %d should warn it is ignored", tier)
			assert.Contains(t, warnings[0], "ignored")
		}
	})
	t.Run("selectors warn", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.Selectors.EventList = "div.event"
		warnings, err := ValidateConfigWithWarnings(cfg)
		require.NoError(t, err)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "CSS selectors will be ignored")
	})
	t.Run("error message lists the new method", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.ExtractionMethod = "magic"
		_, err := ValidateConfigWithWarnings(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bibliocommons_rss")
	})
}

// TestScrapeBiblioCommonsRSS_ViaDispatch verifies ScrapeSource routes
// extraction_method: bibliocommons_rss to the RSS extractor (dry-run, YAML source file).
func TestScrapeBiblioCommonsRSS_ViaDispatch(t *testing.T) {
	srv, _ := bcFeedServer(t)

	cfgYAML := fmt.Sprintf(`
name: test-bc-rss
url: %s/rss/events?types=abc
trust_level: 8
enabled: true
extraction_method: bibliocommons_rss
domain: education
schedule: manual
`, srv.URL)
	cfgPath := filepath.Join(t.TempDir(), "test-bc-rss.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgYAML), 0o644))

	s := NewScraper(NewIngestClient("http://unused", "test-key"), nil, zerolog.Nop())
	s.SetPastEventGrace(testPastEventGrace)
	noDelay := time.Duration(0)
	s.bcPageDelay = &noDelay
	result, err := s.ScrapeSource(t.Context(), "test-bc-rss", ScrapeOptions{DryRun: true, Verbose: true, SourceFile: cfgPath})
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, 3, result.EventsFound)
	assert.Len(t, result.DryRunEvents, 3)
}

// TestBiblioCommonsRSS_MappedEventsPassIngestValidation runs every mapped event through
// the same validation the ingest API applies, so a feed change that yields unusable
// events fails here rather than at 3am in production.
func TestBiblioCommonsRSS_MappedEventsPassIngestValidation(t *testing.T) {
	srv, _ := bcFeedServer(t)
	evs, _, err := newBCTestExtractor().Extract(context.Background(), bcTestSource(srv.URL+"/rss/events"))
	require.NoError(t, err)
	require.NotEmpty(t, evs)
	for _, e := range evs {
		norm := events.NormalizeEventInput(e)
		_, err := events.ValidateEventInput(norm, "toronto.test")
		assert.NoError(t, err, "event %q", e.Name)
	}
}

func TestMapBCItem_NoLocationAndNotVirtualIsSkipped(t *testing.T) {
	item := bcItem{
		Title:      "Mystery Event",
		StartLocal: "2026-10-22T18:30",
		Link:       "https://tpl.bibliocommons.com/events/abc",
		IsVirtual:  "false",
	}
	_, reason, ok := mapBCItem(item, "abc", bcTestSource("http://x"), time.UTC, nil)
	assert.False(t, ok, "an in-person event with no location must not be published as online")
	assert.Contains(t, reason, "location")
}

func TestMapBCItem_ImageUpgradeOnlyRewritesScheme(t *testing.T) {
	item := bcItem{
		Title:      "Event",
		StartLocal: "2026-10-22T18:30",
		Link:       "https://tpl.bibliocommons.com/events/abc",
		IsVirtual:  "true",
	}
	item.Enclosure.URL = "/img.jpg?src=http://origin.example/a.jpg"
	in, _, ok := mapBCItem(item, "abc", bcTestSource("http://x"), time.UTC, nil)
	require.True(t, ok)
	assert.Equal(t, "/img.jpg?src=http://origin.example/a.jpg", in.Image)
}

// bcPageXML renders one feed page of n distinct, valid, virtual events. Virtual
// events need no location, which keeps the fixture small.
func bcPageXML(page, n int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss xmlns:bc="http://bibliocommons.com/rss/1.0/modules/event/" version="2.0"><channel><title>Events | Paged Library</title>`)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("p%d-e%d", page, i)
		fmt.Fprintf(&b, `<item><title>Event %s</title><link>https://x.example/events/%s</link><guid>https://x.example/events/%s</guid>`+
			`<bc:start_date_local>2030-11-01T10:00</bc:start_date_local><bc:end_date_local>2030-11-01T11:00</bc:end_date_local>`+
			`<bc:is_cancelled>false</bc:is_cancelled><bc:is_virtual>true</bc:is_virtual></item>`, id, id, id)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

// bcPagedServer serves fullPages pages of bcRSSPageSize events followed by a final
// page of finalItems events (omit for a full final page) and then empty pages.
// It counts requests via the returned func.
func bcPagedServer(t *testing.T, fullPages, finalItems int) (*httptest.Server, func() int) {
	t.Helper()
	empty := readBCFixture(t, "bibliocommons_rss_empty.xml")
	var mu sync.Mutex
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil || page < 1 {
			page = 1
		}
		lastPage := fullPages + 1
		switch {
		case page <= fullPages:
			_, _ = w.Write([]byte(bcPageXML(page, bcRSSPageSize)))
		case finalItems > 0 && page == lastPage:
			_, _ = w.Write([]byte(bcPageXML(page, finalItems)))
		default:
			_, _ = w.Write(empty)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func bcWarningsText(warnings []string) string { return strings.Join(warnings, "\n") }

func TestBiblioCommonsRSS_MaxPagesReachedWarnsOnFullFinalPage(t *testing.T) {
	srv, requests := bcPagedServer(t, 5, 0)

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 3
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, 3*bcRSSPageSize)
	assert.Equal(t, 3, requests(), "walk stops at the page cap")

	assert.Contains(t, bcWarningsText(warnings), "max_pages_reached")
	assert.Contains(t, bcWarningsText(warnings), "max_pages=3")
	assert.Contains(t, bcWarningsText(warnings), "full final page")
	assert.Contains(t, bcWarningsText(warnings), "results may be truncated")
}

func TestBiblioCommonsRSS_MaxPagesZeroUsesDefaultAndWarns(t *testing.T) {
	// MaxPages <= 0 falls back to 10; a feed longer than that must still warn.
	srv, requests := bcPagedServer(t, 12, 0)

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 0
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, 10*bcRSSPageSize)
	assert.Equal(t, 10, requests(), "default page cap is 10")
	assert.Contains(t, bcWarningsText(warnings), "max_pages_reached")
	assert.Contains(t, bcWarningsText(warnings), "max_pages=10")
}

func TestBiblioCommonsRSS_ExactFeedDoesNotWarn(t *testing.T) {
	// Two pages, the last one partial: the feed is complete and the page cap
	// happens to match, so no truncation may be reported.
	srv, _ := bcPagedServer(t, 1, 7)

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 2
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, bcRSSPageSize+7)
	assert.NotContains(t, bcWarningsText(warnings), "max_pages_reached")
}

func TestBiblioCommonsRSS_EmptyPageBeforeCapDoesNotWarn(t *testing.T) {
	srv, _ := bcPagedServer(t, 1, 0)

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 9
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, bcRSSPageSize)
	assert.NotContains(t, bcWarningsText(warnings), "max_pages_reached")
}

func TestBiblioCommonsRSS_EmptyPageAtCapDoesNotWarn(t *testing.T) {
	// The feed ends naturally on the same iteration the page cap is reached:
	// an empty final page means nothing was dropped.
	srv, requests := bcPagedServer(t, 2, 0)

	src := bcTestSource(srv.URL + "/rss/events")
	src.MaxPages = 3
	evs, warnings, err := newBCTestExtractor().Extract(context.Background(), src)
	require.NoError(t, err)
	assert.Len(t, evs, 2*bcRSSPageSize)
	assert.Equal(t, 3, requests(), "the empty page at the cap is still fetched")
	assert.NotContains(t, bcWarningsText(warnings), "max_pages_reached")
}

func TestScrapeBiblioCommonsRSS_LimitTruncationWarnsAndKeepsFirstN(t *testing.T) {
	srv, _ := bcPagedServer(t, 3, 0)

	s := NewScraper(NewIngestClient("http://unused", "test-key"), nil, zerolog.Nop())
	s.SetPastEventGrace(testPastEventGrace)
	noDelay := time.Duration(0)
	s.bcPageDelay = &noDelay

	src := bcTestSource(srv.URL + "/rss/events")
	result, err := s.scrapeBiblioCommonsRSS(context.Background(), src, ScrapeOptions{
		DryRun:  true,
		Verbose: true,
		Limit:   10,
	})
	require.NoError(t, err)
	require.NoError(t, result.Error)

	total := 3 * bcRSSPageSize
	require.Len(t, result.DryRunEvents, 10, "only the first N events are kept")
	assert.Equal(t, "Event p1-e0", result.DryRunEvents[0].Name)
	assert.Equal(t, "Event p1-e9", result.DryRunEvents[9].Name)

	text := bcWarningsText(result.QualityWarnings)
	want := fmt.Sprintf("result_truncated_by_limit: extracted %d events but limit is 10; keeping the first 10 (scheduled runs set this from scraper_config.max_batch_size)", total)
	assert.Contains(t, text, want)
}

func TestScrapeBiblioCommonsRSS_LimitEqualToResultDoesNotWarn(t *testing.T) {
	srv, _ := bcPagedServer(t, 3, 0)

	s := NewScraper(NewIngestClient("http://unused", "test-key"), nil, zerolog.Nop())
	s.SetPastEventGrace(testPastEventGrace)
	noDelay := time.Duration(0)
	s.bcPageDelay = &noDelay

	src := bcTestSource(srv.URL + "/rss/events")
	total := 3 * bcRSSPageSize
	result, err := s.scrapeBiblioCommonsRSS(context.Background(), src, ScrapeOptions{
		DryRun:  true,
		Verbose: true,
		Limit:   total,
	})
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Len(t, result.DryRunEvents, total, "an exact fit is not truncated")
	assert.NotContains(t, bcWarningsText(result.QualityWarnings), "result_truncated_by_limit")
}
