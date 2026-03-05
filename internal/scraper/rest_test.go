package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// showpassPage builds a JSON response resembling the Showpass API shape.
// nextURL may be "" to signal the last page.
func showpassPage(t *testing.T, events []map[string]any, nextURL string) []byte {
	t.Helper()
	m := map[string]any{
		"count":   len(events),
		"results": events,
	}
	if nextURL != "" {
		m["next"] = nextURL
	} else {
		m["next"] = nil
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// sampleEvent returns a realistic Showpass-like event map.
func sampleEvent(slug, name, startsOn, endsOn string) map[string]any {
	return map[string]any{
		"slug":      slug,
		"name":      name,
		"starts_on": startsOn,
		"ends_on":   endsOn,
		"image":     "https://cdn.example.com/" + slug + ".jpg",
	}
}

func restSource(endpoint string, fieldMap map[string]string, urlTemplate string, maxPages int) SourceConfig {
	return SourceConfig{
		Name:     "test-rest-source",
		URL:      "https://example.com",
		Tier:     3,
		MaxPages: maxPages,
		REST: &RestConfig{
			Endpoint:     endpoint,
			ResultsField: "results",
			NextField:    "next",
			URLTemplate:  urlTemplate,
			FieldMap:     fieldMap,
		},
	}
}

// --------------------------------------------------------------------------
// FetchAndExtractREST tests
// --------------------------------------------------------------------------

func TestFetchAndExtractREST_SinglePage(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		sampleEvent("event-one", "Event One", "2026-04-01T19:00:00Z", "2026-04-01T22:00:00Z"),
		sampleEvent("event-two", "Event Two", "2026-04-02T19:00:00Z", "2026-04-02T22:00:00Z"),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	fieldMap := map[string]string{
		"name":       "name",
		"start_date": "starts_on",
		"end_date":   "ends_on",
		"image":      "image",
	}
	source := restSource(srv.URL, fieldMap, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "Event One", got[0].Name)
	assert.Equal(t, "2026-04-01T19:00:00Z", got[0].StartDate)
	assert.Equal(t, "2026-04-01T22:00:00Z", got[0].EndDate)
	assert.Equal(t, "https://cdn.example.com/event-one.jpg", got[0].Image)

	assert.Equal(t, "Event Two", got[1].Name)
}

func TestFetchAndExtractREST_Pagination(t *testing.T) {
	t.Parallel()

	page1Events := []map[string]any{sampleEvent("slug-1", "Event 1", "2026-04-01T19:00:00Z", "")}
	page2Events := []map[string]any{sampleEvent("slug-2", "Event 2", "2026-04-02T19:00:00Z", "")}

	requestCount := int32(0)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := atomic.AddInt32(&requestCount, 1)
		switch n {
		case 1:
			_, _ = w.Write(showpassPage(t, page1Events, srv.URL+"?page=2"))
		default:
			_, _ = w.Write(showpassPage(t, page2Events, ""))
		}
	}))
	defer srv.Close()

	source := restSource(srv.URL, map[string]string{"name": "name", "start_date": "starts_on"}, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 2, "both pages must be fetched")

	assert.Equal(t, "Event 1", got[0].Name)
	assert.Equal(t, "Event 2", got[1].Name)
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "must have fetched exactly 2 pages")
}

func TestFetchAndExtractREST_MaxPagesRespected(t *testing.T) {
	t.Parallel()

	requestCount := int32(0)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		atomic.AddInt32(&requestCount, 1)
		events := []map[string]any{sampleEvent("slug", "Event", "2026-04-01T19:00:00Z", "")}
		// Always return a next page — max_pages should stop us.
		_, _ = w.Write(showpassPage(t, events, srv.URL+"?page=next"))
	}))
	defer srv.Close()

	// max_pages = 2 — should fetch exactly 2 pages then stop.
	source := restSource(srv.URL, map[string]string{"name": "name"}, "", 2)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Len(t, got, 2, "must return 2 events (1 per page, 2 pages)")
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "must stop at max_pages=2")
}

func TestFetchAndExtractREST_FieldMapMapping(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{
			"title":      "Mapped Event",
			"date_start": "2026-05-01T20:00:00Z",
			"date_end":   "2026-05-01T23:00:00Z",
			"link":       "https://example.com/mapped",
			"thumbnail":  "https://cdn.example.com/img.jpg",
			"venue":      "The Venue",
			"about":      "A description",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	fieldMap := map[string]string{
		"name":        "title",
		"start_date":  "date_start",
		"end_date":    "date_end",
		"url":         "link",
		"image":       "thumbnail",
		"location":    "venue",
		"description": "about",
	}
	source := restSource(srv.URL, fieldMap, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)

	e := got[0]
	assert.Equal(t, "Mapped Event", e.Name)
	assert.Equal(t, "2026-05-01T20:00:00Z", e.StartDate)
	assert.Equal(t, "2026-05-01T23:00:00Z", e.EndDate)
	assert.Equal(t, "https://example.com/mapped", e.URL)
	assert.Equal(t, "https://cdn.example.com/img.jpg", e.Image)
	assert.Equal(t, "The Venue", e.Location)
	assert.Equal(t, "A description", e.Description)
}

func TestFetchAndExtractREST_URLTemplate(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{"slug": "jazz-night", "name": "Jazz Night", "starts_on": "2026-04-10T20:00:00Z"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	source := restSource(srv.URL,
		map[string]string{"name": "name", "start_date": "starts_on"},
		"https://www.showpass.com/{{.slug}}",
		10,
	)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "https://www.showpass.com/jazz-night", got[0].URL)
}

func TestFetchAndExtractREST_URLTemplate_MissingField(t *testing.T) {
	t.Parallel()

	// Event with no "slug" field — template renders <no value>, URL must be cleared.
	events := []map[string]any{
		{"name": "No Slug Event", "starts_on": "2026-04-10T20:00:00Z"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	source := restSource(srv.URL,
		map[string]string{"name": "name", "start_date": "starts_on"},
		"https://www.showpass.com/{{.slug}}",
		10,
	)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "", got[0].URL, "URL must be empty when template renders <no value>")
}

func TestFetchAndExtractREST_NullNextFieldStopsPagination(t *testing.T) {
	t.Parallel()

	events := []map[string]any{sampleEvent("only-event", "Only Event", "2026-04-01T19:00:00Z", "")}

	requestCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		atomic.AddInt32(&requestCount, 1)
		// Explicit null next field.
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	source := restSource(srv.URL, map[string]string{"name": "name", "start_date": "starts_on"}, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "must stop after one page when next is null")
}

func TestFetchAndExtractREST_EmptyResultsField(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, []map[string]any{}, ""))
	}))
	defer srv.Close()

	source := restSource(srv.URL, nil, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, got, "empty results must return empty slice, not error")
}

func TestFetchAndExtractREST_CustomHeaders(t *testing.T) {
	t.Parallel()

	var receivedAuthHeader atomic.Pointer[string]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.Header.Get("Authorization")
		receivedAuthHeader.Store(&v)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, []map[string]any{}, ""))
	}))
	defer srv.Close()

	source := SourceConfig{
		Name:     "headers-source",
		URL:      "https://example.com",
		Tier:     3,
		MaxPages: 10,
		REST: &RestConfig{
			Endpoint:     srv.URL,
			ResultsField: "results",
			NextField:    "next",
			Headers: map[string]string{
				"Authorization": "Bearer test-token",
			},
		},
	}

	extractor := NewRestExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	gotHeader := ""
	if p := receivedAuthHeader.Load(); p != nil {
		gotHeader = *p
	}
	assert.Equal(t, "Bearer test-token", gotHeader, "custom Authorization header must be sent")
}

func TestFetchAndExtractREST_IdentityMapping(t *testing.T) {
	t.Parallel()

	// No field_map: identity mapping uses RawEvent field names as JSON keys.
	events := []map[string]any{
		{
			"Name":        "Identity Event",
			"StartDate":   "2026-05-01T20:00:00Z",
			"EndDate":     "2026-05-01T23:00:00Z",
			"URL":         "https://example.com/event",
			"Image":       "https://cdn.example.com/img.jpg",
			"Location":    "Somewhere",
			"Description": "Details",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	source := restSource(srv.URL, nil, "", 10) // nil field_map = identity

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)

	e := got[0]
	assert.Equal(t, "Identity Event", e.Name)
	assert.Equal(t, "2026-05-01T20:00:00Z", e.StartDate)
	assert.Equal(t, "2026-05-01T23:00:00Z", e.EndDate)
	assert.Equal(t, "https://example.com/event", e.URL)
	assert.Equal(t, "https://cdn.example.com/img.jpg", e.Image)
	assert.Equal(t, "Somewhere", e.Location)
	assert.Equal(t, "Details", e.Description)
}

func TestFetchAndExtractREST_MaxPagesZeroNoLimit(t *testing.T) {
	t.Parallel()

	requestCount := int32(0)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := atomic.AddInt32(&requestCount, 1)
		events := []map[string]any{sampleEvent("slug", "Event", "2026-04-01T19:00:00Z", "")}
		// Return a next page for exactly 3 pages, then stop.
		next := ""
		if n < 3 {
			next = srv.URL + "?page=" + string(rune('0'+n+1))
		}
		_, _ = w.Write(showpassPage(t, events, next))
	}))
	defer srv.Close()

	// max_pages = 0 means no limit — should fetch all 3 pages.
	source := restSource(srv.URL, map[string]string{"name": "name"}, "", 0)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Len(t, got, 3, "must return all events when max_pages=0")
	assert.Equal(t, int32(3), atomic.LoadInt32(&requestCount))
}
