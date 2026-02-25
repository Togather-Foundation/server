package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

func TestFetchAndExtractGraphQL_HappyPath(t *testing.T) {
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
	got, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
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

func TestFetchAndExtractGraphQL_BearerTokenSent(t *testing.T) {
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
	_, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}

func TestFetchAndExtractGraphQL_NoToken(t *testing.T) {
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
	_, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, gotAuth, "no Authorization header should be sent when token is empty")
}

func TestFetchAndExtractGraphQL_EmptyEvents(t *testing.T) {
	t.Parallel()

	srv := newGraphQLServer(t, tranzacResponse(nil))
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFetchAndExtractGraphQL_APIErrors(t *testing.T) {
	t.Parallel()

	errResp := map[string]any{
		"errors": []any{
			map[string]any{"message": "unauthorized"},
		},
	}
	srv := newGraphQLServer(t, errResp)
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestFetchAndExtractGraphQL_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestFetchAndExtractGraphQL_MissingEventField(t *testing.T) {
	t.Parallel()

	resp := map[string]any{"data": map[string]any{"otherField": []any{}}}
	srv := newGraphQLServer(t, resp)
	source := newGraphQLSource(t, srv.URL, "", "")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	_, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allEvents")
}

func TestFetchAndExtractGraphQL_PartialFields(t *testing.T) {
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
	got, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Minimal Event", got[0].Name)
	assert.Equal(t, "2026-04-01T18:00:00+00:00", got[0].StartDate)
	assert.Empty(t, got[0].Description)
	assert.Empty(t, got[0].Image)
	assert.Empty(t, got[0].Location)
	assert.Empty(t, got[0].URL)
}

func TestFetchAndExtractGraphQL_URLTemplateNoSlug(t *testing.T) {
	t.Parallel()

	// Event has no slug key. text/template renders missing map keys as "<no value>",
	// so the resulting URL is non-empty but malformed. This documents the current
	// behaviour: sources should ensure the template field is always present, or
	// use a conditional in the template.
	events := []map[string]any{
		{
			"title":     "No Slug Event",
			"startDate": "2026-04-01T18:00:00+00:00",
		},
	}
	srv := newGraphQLServer(t, tranzacResponse(events))
	source := newGraphQLSource(t, srv.URL, "", "https://tranzac.org/events/{{.slug}}")
	extractor := NewGraphQLExtractor(zerolog.Nop())
	got, err := extractor.FetchAndExtractGraphQL(context.Background(), source, &http.Client{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	// text/template renders missing keys as "<no value>" — URL will be set but
	// contain that placeholder. NormalizeRawEvent will reject it as invalid.
	assert.Contains(t, got[0].URL, "<no value>")
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
