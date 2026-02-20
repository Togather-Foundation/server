package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureServer starts a test HTTP server that serves files from testdata/.
// It returns the server and a helper that returns the full URL for a given fixture filename.
func fixtureServer(t *testing.T) (*httptest.Server, func(string) string) {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	t.Cleanup(srv.Close)
	return srv, func(name string) string {
		return srv.URL + "/" + name
	}
}

// ---- FetchAndExtractJSONLD tests (via httptest) -----------------------------------------

func TestFetchAndExtractJSONLD_Fixtures(t *testing.T) {
	_, urlFor := fixtureServer(t)

	tests := []struct {
		name          string
		fixture       string
		wantCount     int
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:      "single event",
			fixture:   "single_event.html",
			wantCount: 1,
		},
		{
			name:      "graph with mixed types",
			fixture:   "graph_events.html",
			wantCount: 2,
		},
		{
			name:      "top-level array of events",
			fixture:   "array_events.html",
			wantCount: 2,
		},
		{
			name:      "itemlist wrapping events",
			fixture:   "itemlist_events.html",
			wantCount: 2,
		},
		{
			name:      "no json-ld at all",
			fixture:   "no_jsonld.html",
			wantCount: 0,
		},
		{
			name:          "malformed json-ld",
			fixture:       "malformed_jsonld.html",
			wantErr:       true,
			wantErrSubstr: "parsing JSON-LD",
		},
		{
			name:      "event with string location",
			fixture:   "string_location.html",
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, err := FetchAndExtractJSONLD(context.Background(), urlFor(tc.fixture))
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrSubstr != "" {
					assert.Contains(t, err.Error(), tc.wantErrSubstr)
				}
				return
			}
			require.NoError(t, err)
			assert.Len(t, events, tc.wantCount)
		})
	}
}

// ---- extractEvents unit tests -----------------------------------------------------------

func TestExtractEvents_SingleEvent(t *testing.T) {
	input := `{"@context":"https://schema.org","@type":"Event","name":"Solo Show","startDate":"2026-01-01"}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assertEventName(t, events[0], "Solo Show")
}

func TestExtractEvents_EventSeries(t *testing.T) {
	input := `{"@context":"https://schema.org","@type":"EventSeries","name":"Weekly Jazz","startDate":"2026-01-01"}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assertEventName(t, events[0], "Weekly Jazz")
}

func TestExtractEvents_NonEventSkipped(t *testing.T) {
	input := `{"@context":"https://schema.org","@type":"Organization","name":"ACME Corp"}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestExtractEvents_TopLevelArray(t *testing.T) {
	input := `[
		{"@context":"https://schema.org","@type":"Event","name":"Alpha"},
		{"@context":"https://schema.org","@type":"Event","name":"Beta"},
		{"@context":"https://schema.org","@type":"Organization","name":"Skip Me"}
	]`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestExtractEvents_GraphContainer(t *testing.T) {
	input := `{
		"@context":"https://schema.org",
		"@graph":[
			{"@type":"Organization","name":"Org"},
			{"@type":"Event","name":"Graph Event A"},
			{"@type":"Event","name":"Graph Event B"}
		]
	}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestExtractEvents_ItemList(t *testing.T) {
	input := `{
		"@context":"https://schema.org",
		"@type":"ItemList",
		"itemListElement":[
			{"@type":"ListItem","position":1,"item":{"@type":"Event","name":"Workshop"}},
			{"@type":"ListItem","position":2,"item":{"@type":"Event","name":"Seminar"}},
			{"@type":"ListItem","position":3,"item":{"@type":"Organization","name":"Skip"}}
		]
	}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestExtractEvents_StringLocation(t *testing.T) {
	input := `{"@context":"https://schema.org","@type":"Event","name":"Reading","location":"Glad Day Bookshop"}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestExtractEvents_InvalidJSON(t *testing.T) {
	input := `{this is not valid json`
	_, err := extractEvents([]byte(input))
	require.Error(t, err)
}

func TestExtractEvents_EmptyInput(t *testing.T) {
	events, err := extractEvents([]byte(""))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestExtractEvents_SchemaOrgPrefixedType(t *testing.T) {
	input := `{"@context":"https://schema.org","@type":"https://schema.org/Event","name":"Prefixed"}`
	events, err := extractEvents([]byte(input))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

// ---- RobotsAllowed tests ---------------------------------------------------------------

func TestRobotsAllowed_NoRobotsFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	allowed, err := RobotsAllowed(context.Background(), srv.URL+"/events", scraperUserAgent)
	require.NoError(t, err)
	assert.True(t, allowed, "missing robots.txt should allow all")
}

func TestRobotsAllowed_AllowAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	allowed, err := RobotsAllowed(context.Background(), srv.URL+"/events", scraperUserAgent)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRobotsAllowed_DisallowAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	allowed, err := RobotsAllowed(context.Background(), srv.URL+"/events", scraperUserAgent)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRobotsAllowed_DisallowSpecificAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: Togather-SEL-Scraper\nDisallow: /\n\nUser-agent: *\nAllow: /\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	allowed, err := RobotsAllowed(context.Background(), srv.URL+"/events", scraperUserAgent)
	require.NoError(t, err)
	assert.False(t, allowed)
}

// ---- Fixture-file sanity tests ---------------------------------------------------------

func TestFixtureFilesExist(t *testing.T) {
	fixtures := []string{
		"single_event.html",
		"graph_events.html",
		"array_events.html",
		"itemlist_events.html",
		"no_jsonld.html",
		"malformed_jsonld.html",
		"string_location.html",
	}
	for _, f := range fixtures {
		path := filepath.Join("testdata", f)
		_, err := os.Stat(path)
		assert.NoError(t, err, "fixture file should exist: %s", path)
	}
}

// ---- helpers ---------------------------------------------------------------------------

// assertEventName unmarshals a raw JSON event and checks its "name" field.
func assertEventName(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()
	var obj struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, want, obj.Name)
}
