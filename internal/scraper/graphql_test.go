package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"text/template"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tranzacResponse builds a mock DatoCMS GraphQL response.
func tranzacResponse(events []map[string]any) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"allEvents": events,
		},
	}
}

func newGraphQLServer(t *testing.T, resp any) *httptest.Server {
	t.Helper()
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGraphQLSource(t *testing.T, endpoint, token, urlTemplate string) SourceConfig {
	t.Helper()
	return SourceConfig{
		Name:       "test-graphql",
		URL:        "https://example.com",
		Tier:       3,
		TrustLevel: 7,
		License:    "CC0-1.0",
		GraphQL: &GraphQLConfig{
			Endpoint:    endpoint,
			Token:       token,
			Query:       `{ allEvents { title startDate endDate slug description photo { url } rooms { name } } }`,
			EventField:  "allEvents",
			TimeoutMs:   5000,
			URLTemplate: urlTemplate,
		},
	}
}

func TestGraphQLExtract_HappyPath(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{
			"title":       "Book Launch",
			"startDate":   "2026-03-15T19:00:00+00:00",
			"endDate":     "2026-03-15T22:00:00+00:00",
			"slug":        "book-launch-2026-03-15",
			"description": "A great book launch event.",
			"photo":       map[string]any{"url": "https://example.com/photo.jpg"},
			"rooms":       []any{map[string]any{"name": "Main Hall"}},
		},
	}

	srv := newGraphQLServer(t, tranzacResponse(events))
	source := newGraphQLSource(t, srv.URL, "", "https://tranzac.org/events/{{.slug}}")

	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, "Book Launch", got[0].Name)
	assert.Equal(t, "2026-03-15T19:00:00+00:00", got[0].StartDate)
	assert.Equal(t, "2026-03-15T22:00:00+00:00", got[0].EndDate)
	assert.Equal(t, "A great book launch event.", got[0].Description)
	assert.Equal(t, "https://example.com/photo.jpg", got[0].Image)
	assert.Equal(t, "Main Hall", got[0].Location)
	assert.Equal(t, "https://tranzac.org/events/book-launch-2026-03-15", got[0].URL)
}

func TestGraphQLExtract_BearerTokenSent(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tranzacResponse(nil))
	}))
	t.Cleanup(srv.Close)

	source := newGraphQLSource(t, srv.URL, "my-secret-token", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}

func TestGraphQLExtract_RequestHeaders(t *testing.T) {
	t.Parallel()

	var gotContentType, gotAccept, gotUserAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tranzacResponse(nil))
	}))
	t.Cleanup(srv.Close)

	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Equal(t, "application/json", gotContentType, "Content-Type must be application/json")
	assert.Equal(t, "application/json", gotAccept, "Accept must be application/json")
	assert.Equal(t, scraperUserAgent, gotUserAgent, "User-Agent must be the Togather scraper identity string")
}

func TestGraphQLExtract_NoToken(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tranzacResponse(nil))
	}))
	t.Cleanup(srv.Close)

	source := newGraphQLSource(t, srv.URL, "", "") // no token
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, gotAuth, "no Authorization header should be sent when token is empty")
}

func TestGraphQLExtract_EmptyEvents(t *testing.T) {
	t.Parallel()

	srv := newGraphQLServer(t, tranzacResponse(nil))
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGraphQLExtract_APIErrors(t *testing.T) {
	t.Parallel()

	errResp := map[string]any{
		"errors": []any{
			map[string]any{"message": "unauthorized"},
		},
	}
	srv := newGraphQLServer(t, errResp)
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestGraphQLExtract_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestGraphQLExtract_MissingEventField(t *testing.T) {
	t.Parallel()

	resp := map[string]any{"data": map[string]any{"otherField": []any{}}}
	srv := newGraphQLServer(t, resp)
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allEvents")
}

func TestGraphQLExtract_PartialFields(t *testing.T) {
	t.Parallel()

	// Only title and startDate provided — other fields empty/missing
	events := []map[string]any{
		{
			"title":     "Minimal Event",
			"startDate": "2026-04-01T18:00:00+00:00",
		},
	}
	srv := newGraphQLServer(t, tranzacResponse(events))
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Minimal Event", got[0].Name)
	assert.Equal(t, "2026-04-01T18:00:00+00:00", got[0].StartDate)
	assert.Empty(t, got[0].Description)
	assert.Empty(t, got[0].Image)
	assert.Empty(t, got[0].Location)
	assert.Empty(t, got[0].URL)
}

func TestGraphQLExtract_URLTemplateNoSlug(t *testing.T) {
	t.Parallel()

	// Event has no slug key. template.Option("missingkey=error") causes Execute()
	// to return an error rather than rendering "<no value>". The URL is cleared
	// to prevent all slug-less events from colliding on the same dedup key in
	// eventIDFromRaw.
	events := []map[string]any{
		{
			"title":     "No Slug Event",
			"startDate": "2026-04-01T18:00:00+00:00",
		},
	}
	srv := newGraphQLServer(t, tranzacResponse(events))
	source := newGraphQLSource(t, srv.URL, "", "https://tranzac.org/events/{{.slug}}")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	// URL must be cleared (Execute returns an error for missing key) so each event
	// gets a unique content-based EventID rather than colliding as duplicates.
	assert.Equal(t, "", got[0].URL)
}

// TestGraphQLExtract_NullDataField verifies that a response with a
// null "data" field ({"data": null}) does not panic and returns an error
// about the missing event field. This is the srv-rstvo scenario: Go's nil
// map lookup returns (zero, false), so the "event field not found" path is
// reached without a nil-pointer dereference.
func TestGraphQLExtract_NullDataField(t *testing.T) {
	t.Parallel()

	// JSON: {"data": null} — data decodes to a nil map[string]json.RawMessage
	srv := newGraphQLServer(t, map[string]any{"data": nil})
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allEvents", "should report missing event field, not panic")
}

// TestGraphQLExtract_URLTemplateNoSlug_MultipleEvents verifies that
// multiple events all missing the slug field are returned individually (not
// collapsed to one) after Execute() errors on the missing key and clears URLs.
func TestGraphQLExtract_URLTemplateNoSlug_MultipleEvents(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{"title": "No Slug Event A", "startDate": "2026-04-01T18:00:00+00:00"},
		{"title": "No Slug Event B", "startDate": "2026-04-02T18:00:00+00:00"},
	}
	srv := newGraphQLServer(t, tranzacResponse(events))
	source := newGraphQLSource(t, srv.URL, "", "https://tranzac.org/events/{{.slug}}")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	// Both events must be returned — without the fix they would be collapsed
	// to one because both would share the same "<no value>" dedup URL.
	require.Len(t, got, 2)
	assert.Equal(t, "", got[0].URL)
	assert.Equal(t, "", got[1].URL)
	assert.Equal(t, "No Slug Event A", got[0].Name)
	assert.Equal(t, "No Slug Event B", got[1].Name)
}

// TestGraphQLExtract_PartialSuccess verifies the conservative error
// handling: when the GraphQL response contains both errors and partial data,
// the extractor returns an error and does NOT return the partial events.
// This is intentional — partial data from a misbehaving endpoint is less
// trustworthy than a clean failure (GraphQL spec allows both errors and data).
func TestGraphQLExtract_PartialSuccess(t *testing.T) {
	t.Parallel()

	resp := map[string]any{
		"errors": []any{
			map[string]any{"message": "partial"},
		},
		"data": map[string]any{
			"allEvents": []any{
				map[string]any{
					"title":     "Partial Event",
					"startDate": "2026-04-01T18:00:00+00:00",
				},
			},
		},
	}
	srv := newGraphQLServer(t, resp)
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.Extract(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "partial")
	assert.Nil(t, got, "no events should be returned when errors are present")
}

// TestGraphQLExtract_TimeoutOverride verifies that cfg.TimeoutMs
// overrides a too-short client.Timeout when the cfg value is larger.
func TestGraphQLExtract_TimeoutOverride(t *testing.T) {
	t.Parallel()

	t.Run("cfg timeout longer than client timeout — request succeeds", func(t *testing.T) {
		t.Parallel()

		// Server sleeps 30ms before responding. A 10ms client.Timeout would fail
		// without the override; cfg.TimeoutMs=5000 extends it so the request succeeds.
		slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(30 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tranzacResponse(nil))
		}))
		t.Cleanup(slowSrv.Close)

		source := newGraphQLSource(t, slowSrv.URL, "", "")
		source.GraphQL.TimeoutMs = 5000 // 5 s — much longer than the 30 ms server delay

		clientWithShortTimeout := &http.Client{Timeout: 10 * time.Millisecond}
		extractor := NewGraphQLExtractor(zerolog.Nop())
		got, err := extractor.Extract(context.Background(), source, clientWithShortTimeout)
		require.NoError(t, err, "cfg timeout should override the 10ms client timeout")
		assert.Empty(t, got)
	})

	t.Run("cfg timeout shorter than client timeout — client timeout preserved", func(t *testing.T) {
		t.Parallel()

		// Quick server (no delay). cfg.TimeoutMs is shorter than client.Timeout,
		// so the client.Timeout is preserved and the request completes fine.
		fastSrv := newGraphQLServer(t, tranzacResponse(nil))

		source := newGraphQLSource(t, fastSrv.URL, "", "")
		source.GraphQL.TimeoutMs = 10 // 10ms — shorter than client's 5s

		clientWithLongTimeout := &http.Client{Timeout: 5000 * time.Millisecond}
		extractor := NewGraphQLExtractor(zerolog.Nop())
		got, err := extractor.Extract(context.Background(), source, clientWithLongTimeout)
		require.NoError(t, err, "client timeout should be preserved when cfg is shorter")
		assert.Empty(t, got)
	})
}

// --------------------------------------------------------------------------
// mapToRawEvent unit tests (srv-8ddm0)
// --------------------------------------------------------------------------

func TestMapToRawEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		item        map[string]any
		urlTemplate string
		want        RawEvent
	}{
		{
			name: "all fields populated",
			item: map[string]any{
				"title":       "Full Event",
				"startDate":   "2026-06-01T19:00:00+00:00",
				"endDate":     "2026-06-01T22:00:00+00:00",
				"description": "A fully populated event.",
				"photo":       map[string]any{"url": "https://example.com/photo.jpg"},
				"rooms":       []any{map[string]any{"name": "Main Stage"}},
				"slug":        "full-event-2026-06-01",
			},
			urlTemplate: "https://example.com/events/{{.slug}}",
			want: RawEvent{
				Name:        "Full Event",
				StartDate:   "2026-06-01T19:00:00+00:00",
				EndDate:     "2026-06-01T22:00:00+00:00",
				Description: "A fully populated event.",
				Image:       "https://example.com/photo.jpg",
				Location:    "Main Stage",
				URL:         "https://example.com/events/full-event-2026-06-01",
			},
		},
		{
			name: "photo field present",
			item: map[string]any{
				"title": "Photo Event",
				"photo": map[string]any{"url": "https://cdn.example.com/img.png"},
			},
			want: RawEvent{
				Name:  "Photo Event",
				Image: "https://cdn.example.com/img.png",
			},
		},
		{
			name: "photo field nil — no panic",
			item: map[string]any{
				"title": "No Photo",
				"photo": nil,
			},
			want: RawEvent{Name: "No Photo"},
		},
		{
			name: "rooms field present — uses first room",
			item: map[string]any{
				"title": "Rooms Event",
				"rooms": []any{
					map[string]any{"name": "Bar Room"},
					map[string]any{"name": "Back Room"},
				},
			},
			want: RawEvent{Name: "Rooms Event", Location: "Bar Room"},
		},
		{
			name: "rooms field empty slice — no location",
			item: map[string]any{
				"title": "No Room Event",
				"rooms": []any{},
			},
			want: RawEvent{Name: "No Room Event"},
		},
		{
			name: "empty item — zero RawEvent",
			item: map[string]any{},
			want: RawEvent{},
		},
		{
			name: "url_template with non-slug field",
			item: map[string]any{
				"title": "ID Event",
				"id":    "42",
			},
			urlTemplate: "https://example.com/events/{{.id}}",
			want: RawEvent{
				Name: "ID Event",
				URL:  "https://example.com/events/42",
			},
		},
		{
			// srv-oyv17: photo field as a plain string (not an object) is silently
			// ignored — Image remains empty. Documents current behaviour.
			name: "photo field as plain string — not object, silently ignored",
			item: map[string]any{
				"title": "String Photo Event",
				"photo": "https://example.com/photo.jpg",
			},
			want: RawEvent{Name: "String Photo Event"},
		},
		{
			// srv-oyv17: rooms first element is a plain string (not a map) — silently
			// ignored because the type assertion to map[string]any fails. Location
			// remains empty. Documents current behaviour.
			name: "rooms first element is not a map — silently ignored",
			item: map[string]any{
				"title": "String Room Event",
				"rooms": []any{"Main Stage"},
			},
			want: RawEvent{Name: "String Room Event"},
		},
		{
			// missingkey=error: when the template references a key absent from the
			// item map, Execute() returns an error and URL is cleared rather than
			// containing a literal "<no value>" string.
			name: "url_template missing key — URL cleared, no error propagated",
			item: map[string]any{
				"title": "Missing Key Event",
			},
			urlTemplate: "https://example.com/events/{{.slug}}",
			want:        RawEvent{Name: "Missing Key Event"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tmpl *template.Template
			if tt.urlTemplate != "" {
				var err error
				tmpl, err = template.New("url").Option("missingkey=error").Parse(tt.urlTemplate)
				require.NoError(t, err)
			}

			got := mapToRawEvent(tt.item, tmpl, zerolog.Nop())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScrapeTier3_Integration(t *testing.T) {
	t.Parallel()

	events := []map[string]any{
		{
			"title":     "Tier3 Event",
			"startDate": "2026-05-10T20:00:00+00:00",
			"slug":      "tier3-event-2026-05-10",
			"rooms":     []any{map[string]any{"name": "Side Bar"}},
		},
	}

	gqlSrv := newGraphQLServer(t, tranzacResponse(events))

	dir := t.TempDir()
	cfgYAML := `name: tier3-test
url: "https://example.com"
tier: 3
enabled: true
trust_level: 7
schedule: manual
graphql:
  endpoint: "` + gqlSrv.URL + `"
  query: "{ allEvents { title startDate slug rooms { name } } }"
  event_field: "allEvents"
  url_template: "https://tranzac.org/events/{{.slug}}"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tier3-test.yaml"), []byte(cfgYAML), 0o644))

	scraper := newTestScraper(t, "http://unused")
	results, err := scraper.ScrapeAll(t.Context(), ScrapeOptions{
		DryRun:     true,
		SourcesDir: dir,
		TierFilter: -1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Error)
	assert.Equal(t, 1, results[0].EventsFound)
	assert.Equal(t, 1, results[0].EventsSubmitted)
	assert.Equal(t, 3, results[0].Tier)
}
