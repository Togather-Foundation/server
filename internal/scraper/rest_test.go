package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

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

	// Event with no "slug" field — template.Option("missingkey=error") causes
	// Execute() to return an error; URL must be cleared rather than set to a
	// malformed value that would cause all such events to share the same dedup key.
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
	assert.Equal(t, "", got[0].URL, "URL must be empty when template key is missing")
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

// TestMapRESTItemToRawEvent_URLTemplate tests the url_template rendering path
// inside mapRESTItemToRawEvent, including the missingkey=error behaviour.
func TestMapRESTItemToRawEvent_URLTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		item        map[string]any
		fieldMap    map[string]string
		urlTemplate string
		wantURL     string
	}{
		{
			name:        "template renders correctly when key present",
			item:        map[string]any{"slug": "jazz-night"},
			fieldMap:    map[string]string{},
			urlTemplate: "https://example.com/{{.slug}}",
			wantURL:     "https://example.com/jazz-night",
		},
		{
			// missingkey=error: Execute() returns an error for a missing key;
			// URL must be cleared rather than containing "<no value>" or any
			// other sentinel string.
			name:        "template missing key — URL cleared, no error propagated",
			item:        map[string]any{"name": "No Slug Event"},
			fieldMap:    map[string]string{},
			urlTemplate: "https://example.com/{{.slug}}",
			wantURL:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpl, err := template.New("url").Option("missingkey=error").Parse(tt.urlTemplate)
			require.NoError(t, err)

			got := mapRESTItemToRawEvent(tt.item, tt.fieldMap, tmpl, zerolog.Nop())
			assert.Equal(t, tt.wantURL, got.URL)
		})
	}
}

// --------------------------------------------------------------------------
// Error-path tests
// --------------------------------------------------------------------------

func TestFetchAndExtractREST_NonOKStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"internal server error", http.StatusInternalServerError},
		{"not found", http.StatusNotFound},
		{"service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "server error", tt.statusCode)
			}))
			t.Cleanup(srv.Close)

			source := restSource(srv.URL, nil, "", 10)
			extractor := NewRestExtractor(zerolog.Nop())
			_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), strconv.Itoa(tt.statusCode))
		})
	}
}

func TestFetchAndExtractREST_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	t.Cleanup(srv.Close)

	source := restSource(srv.URL, nil, "", 10)
	extractor := NewRestExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.Error(t, err)
}

func TestFetchAndExtractREST_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Server blocks until the request context is cancelled.
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())

	source := restSource(srv.URL, nil, "", 10)
	extractor := NewRestExtractor(zerolog.Nop())

	done := make(chan error, 1)
	go func() {
		_, err := extractor.FetchAndExtractREST(ctx, source, &http.Client{})
		done <- err
	}()

	// Wait until the handler is reached before cancelling.
	<-started
	cancel()

	err := <-done
	require.Error(t, err)
}

func TestFetchAndExtractREST_MissingResultsField(t *testing.T) {
	t.Parallel()

	// Server returns valid JSON but the key configured as results_field ("results")
	// is absent — fetchPage treats this as an empty page (not an error).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Respond with a different key, not "results".
		_, _ = w.Write([]byte(`{"items":[],"next":null}`))
	}))
	t.Cleanup(srv.Close)

	source := restSource(srv.URL, nil, "", 10)
	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, got, "missing results_field should return empty slice, not error")
}

// --------------------------------------------------------------------------
// resolveNestedString tests
// --------------------------------------------------------------------------

func TestResolveNestedString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item map[string]any
		path string
		want string
	}{
		{
			name: "flat key returns string value",
			item: map[string]any{"name": "Event Name"},
			path: "name",
			want: "Event Name",
		},
		{
			name: "single dot traverses one level",
			item: map[string]any{
				"logo": map[string]any{"url": "https://img.example.com/pic.jpg"},
			},
			path: "logo.url",
			want: "https://img.example.com/pic.jpg",
		},
		{
			name: "double dot traverses two levels",
			item: map[string]any{
				"venue": map[string]any{
					"address": map[string]any{"city": "Calgary"},
				},
			},
			path: "venue.address.city",
			want: "Calgary",
		},
		{
			name: "missing intermediate key returns empty string",
			item: map[string]any{
				"logo": map[string]any{"url": "https://img.example.com/pic.jpg"},
			},
			path: "missing.url",
			want: "",
		},
		{
			name: "non-map intermediate returns empty string",
			item: map[string]any{"name": "plain string"},
			path: "name.url",
			want: "",
		},
		{
			name: "final value is non-string returns empty string",
			item: map[string]any{
				"count": map[string]any{"value": 42},
			},
			path: "count.value",
			want: "",
		},
		{
			name: "empty path returns empty string",
			item: map[string]any{"name": "Event Name"},
			path: "",
			want: "",
		},
		{
			name: "missing flat key returns empty string",
			item: map[string]any{"name": "Event Name"},
			path: "other",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveNestedString(tt.item, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFetchAndExtractREST_NestedFieldMap tests dot-notation field_map values
// through the full FetchAndExtractREST path (Eventbrite-style nested JSON).
func TestFetchAndExtractREST_NestedFieldMap(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{
			"title":      map[string]any{"text": "My Event"},
			"logo":       map[string]any{"url": "https://img.com/pic.jpg"},
			"date_start": "2026-05-01T20:00:00Z",
			"venue_name": "The Hall",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	defer srv.Close()

	fieldMap := map[string]string{
		"name":       "title.text",
		"image":      "logo.url",
		"start_date": "date_start",
		"location":   "venue_name",
	}
	source := restSource(srv.URL, fieldMap, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)

	e := got[0]
	assert.Equal(t, "My Event", e.Name, "nested title.text must be resolved")
	assert.Equal(t, "https://img.com/pic.jpg", e.Image, "nested logo.url must be resolved")
	assert.Equal(t, "2026-05-01T20:00:00Z", e.StartDate, "flat date_start must still work")
	assert.Equal(t, "The Hall", e.Location, "flat venue_name must still work")
}

// --------------------------------------------------------------------------
// Redirect-limiting tests
// --------------------------------------------------------------------------

func TestFetchAndExtractREST_RedirectLimited(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		// Always redirect back to ourselves (with an incrementing counter) to
		// create an infinite redirect chain.
		next := fmt.Sprintf("%s?n=%d", srv.URL, n)
		http.Redirect(w, r, next, http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	source := restSource(srv.URL, nil, "", 10)
	extractor := NewRestExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	// The request must fail (we never get a 200), but the critical assertion is
	// that the server received at most maxRESTRedirects+1 requests (not infinite).
	require.Error(t, err)
	got := int(requestCount.Load())
	assert.LessOrEqual(t, got, maxRESTRedirects+1,
		"redirect chain must be stopped at maxRESTRedirects; got %d requests", got)
}

func TestFetchAndExtractREST_RedirectFollowed(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		sampleEvent("redir-event", "Redirect Event", "2026-06-01T19:00:00Z", "2026-06-01T22:00:00Z"),
	}

	var requestCount atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			// First request: permanent redirect to canonical URL.
			http.Redirect(w, r, srv.URL+"/canonical", http.StatusMovedPermanently)
			return
		}
		// Second request: serve valid JSON.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, events, ""))
	}))
	t.Cleanup(srv.Close)

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
	require.Len(t, got, 1, "redirect must be followed and events returned")
	assert.Equal(t, "Redirect Event", got[0].Name)
}

// --------------------------------------------------------------------------
// Bare JSON array tests (results_field: ".")
// --------------------------------------------------------------------------

func bareArrayRestSource(endpoint string, fieldMap map[string]string, urlTemplate string, maxPages int) SourceConfig {
	return SourceConfig{
		Name:     "test-bare-array-source",
		URL:      "https://example.com",
		Tier:     3,
		MaxPages: maxPages,
		REST: &RestConfig{
			Endpoint:     endpoint,
			ResultsField: ".",
			URLTemplate:  urlTemplate,
			FieldMap:     fieldMap,
		},
	}
}

func TestFetchAndExtractREST_BareArray(t *testing.T) {
	t.Parallel()

	requestCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Event 1","starts_on":"2026-04-01T19:00:00Z"},{"name":"Event 2","starts_on":"2026-04-02T19:00:00Z"}]`))
	}))
	defer srv.Close()

	fieldMap := map[string]string{
		"name":       "name",
		"start_date": "starts_on",
	}
	source := bareArrayRestSource(srv.URL, fieldMap, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "Event 1", got[0].Name)
	assert.Equal(t, "2026-04-01T19:00:00Z", got[0].StartDate)
	assert.Equal(t, "Event 2", got[1].Name)
	assert.Equal(t, "2026-04-02T19:00:00Z", got[1].StartDate)

	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "bare array must make exactly 1 request (no pagination)")
}

func TestFetchAndExtractREST_BareArrayEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	source := bareArrayRestSource(srv.URL, nil, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, got, "empty bare array must return empty slice, not error")
}

func TestFetchAndExtractREST_BareArrayWithURLTemplate(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"slug":"jazz-night","name":"Jazz Night","starts_on":"2026-04-10T20:00:00Z"}]`))
	}))
	defer srv.Close()

	source := bareArrayRestSource(
		srv.URL,
		map[string]string{"name": "name", "start_date": "starts_on"},
		"https://www.showpass.com/{{.slug}}",
		10,
	)

	extractor := NewRestExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Jazz Night", got[0].Name)
	assert.Equal(t, "https://www.showpass.com/jazz-night", got[0].URL, "url_template must be applied to bare array items")
}

func TestFetchAndExtractREST_BareArrayWithFieldMap(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"Mapped Event","date_start":"2026-05-01T20:00:00Z","date_end":"2026-05-01T23:00:00Z","link":"https://example.com/mapped","thumbnail":"https://cdn.example.com/img.jpg","venue":"The Venue","about":"A description"}]`))
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
	source := bareArrayRestSource(srv.URL, fieldMap, "", 10)

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

func TestFetchAndExtractREST_BareArrayInvalidJSON(t *testing.T) {
	t.Parallel()

	// Server returns a JSON object, not a bare array.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"name": "Event"}]}`))
	}))
	defer srv.Close()

	source := bareArrayRestSource(srv.URL, nil, "", 10)

	extractor := NewRestExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.Error(t, err, "a JSON object body in bare array mode must return an error")
	assert.Contains(t, err.Error(), "rest: decoding bare array from", "error must identify the bare array decode failure")
}

func TestFetchAndExtractREST_TimeoutMs(t *testing.T) {
	t.Parallel()

	// Server sleeps longer than the configured timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(showpassPage(t, nil, ""))
	}))
	t.Cleanup(srv.Close)

	source := SourceConfig{
		Name:     "timeout-source",
		URL:      "https://example.com",
		Tier:     3,
		MaxPages: 10,
		REST: &RestConfig{
			Endpoint:     srv.URL,
			ResultsField: "results",
			NextField:    "next",
			TimeoutMs:    50, // 50 ms — much shorter than 500 ms server delay
		},
	}

	extractor := NewRestExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractREST(t.Context(), source, &http.Client{})
	require.Error(t, err, "request should time out before the slow server responds")
}
