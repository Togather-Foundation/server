package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestContentNegotiationJSON verifies that requests with Accept: application/json
// receive JSON-LD responses (application/json is aliased to JSON-LD per spec)
func TestContentNegotiationJSON(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, eventName, eventNameFromPayload(payload))
	require.Contains(t, payload, "@context", "JSON response should include JSON-LD @context")
	require.Contains(t, payload, "@type", "JSON response should include JSON-LD @type")
}

// TestContentNegotiationJSONLD verifies that requests with Accept: application/ld+json
// receive proper JSON-LD responses with @context and @type
func TestContentNegotiationJSONLD(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/ld+json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, eventName, eventNameFromPayload(payload))
	require.Contains(t, payload, "@context", "JSON-LD must include @context")
	require.Contains(t, payload, "@type", "JSON-LD must include @type")
	require.Contains(t, payload, "@id", "JSON-LD must include @id (canonical URI)")
}

// TestContentNegotiationHTML verifies that requests with Accept: text/html
// receive HTML responses with embedded JSON-LD
func TestContentNegotiationHTML(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)

	// Verify HTML structure
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "<html")
	require.Contains(t, html, eventName)

	// Verify embedded JSON-LD
	require.Contains(t, html, `<script type="application/ld+json">`)
	require.Contains(t, html, `"@context"`)
	require.Contains(t, html, `"@type"`)
	require.Contains(t, html, `"name"`)
}

// TestContentNegotiationTurtle verifies that requests with Accept: text/turtle
// receive valid RDF Turtle serialization
func TestContentNegotiationTurtle(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/turtle", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify Turtle syntax basics
	require.Contains(t, turtle, "@prefix")
	require.Contains(t, turtle, "schema:")
	require.Contains(t, turtle, "a schema:Event")
	require.Contains(t, turtle, `schema:name "`+eventName)
}

// TestContentNegotiationMissingAcceptHeader verifies that requests without
// an Accept header default to application/json
func TestContentNegotiationMissingAcceptHeader(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	// Explicitly do NOT set Accept header

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, eventName, eventNameFromPayload(payload))
}

// TestContentNegotiationInvalidAcceptHeader verifies that requests with
// an unsupported Accept header default to application/json
func TestContentNegotiationInvalidAcceptHeader(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/xml")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, eventName, eventNameFromPayload(payload))
}

// TestContentNegotiationListEndpoint verifies content negotiation works
// for list endpoints (GET /api/v1/events)
func TestContentNegotiationListEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	testCases := []struct {
		name        string
		accept      string
		expectType  string
		expectItems bool
	}{
		{
			name:        "JSON",
			accept:      "application/json",
			expectType:  "application/json",
			expectItems: true,
		},
		{
			name:        "JSON-LD",
			accept:      "application/ld+json",
			expectType:  "application/ld+json",
			expectItems: true,
		},
		{
			name:        "Missing Accept",
			accept:      "",
			expectType:  "application/json",
			expectItems: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
			require.NoError(t, err)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, tc.expectType, resp.Header.Get("Content-Type"))

			if tc.expectItems {
				var payload map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
				require.Contains(t, payload, "items")
				items, ok := payload["items"].([]any)
				require.True(t, ok)
				require.Greater(t, len(items), 0)
			}
		})
	}
}

// TestContentNegotiationPlaces verifies content negotiation works for places
func TestContentNegotiationPlaces(t *testing.T) {
	env := setupTestEnv(t)

	place := insertPlace(t, env, "Centennial Park", "Toronto")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places/"+place.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/ld+json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Contains(t, payload, "@context")
	require.Contains(t, payload, "@type")
}

// TestContentNegotiationOrganizations verifies content negotiation works for organizations
func TestContentNegotiationOrganizations(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/organizations/"+org.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/ld+json", resp.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Contains(t, payload, "@context")
	require.Contains(t, payload, "@type")
}

// TestContentNegotiationBrowserAcceptHeader verifies that typical browser
// Accept headers (with */*) are handled correctly
func TestContentNegotiationBrowserAcceptHeader(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	// Typical browser Accept header (from Chrome/Firefox)
	browserAccept := "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", browserAccept)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)

	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, strings.ToLower("Jazz in the Park"))
}
