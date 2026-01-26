package contracts_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHTMLWithEmbeddedJSONLD verifies that HTML responses for dereferenceable
// URIs contain properly embedded JSON-LD in <script type="application/ld+json"> tags
func TestHTMLWithEmbeddedJSONLD(t *testing.T) {
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
	require.Contains(t, html, "<!DOCTYPE html>", "HTML must have DOCTYPE")
	require.Contains(t, html, "<html", "HTML must have html tag")
	require.Contains(t, html, "</html>", "HTML must close html tag")
	require.Contains(t, html, "<head>", "HTML must have head section")
	require.Contains(t, html, "</head>", "HTML must close head section")
	require.Contains(t, html, "<body>", "HTML must have body section")
	require.Contains(t, html, "</body>", "HTML must close body section")

	// Verify embedded JSON-LD script tag
	require.Contains(t, html, `<script type="application/ld+json">`, "HTML must contain JSON-LD script tag")

	// Extract and parse embedded JSON-LD
	jsonldStart := strings.Index(html, `<script type="application/ld+json">`)
	require.NotEqual(t, -1, jsonldStart, "JSON-LD script tag must exist")

	jsonldStart += len(`<script type="application/ld+json">`)
	jsonldEnd := strings.Index(html[jsonldStart:], "</script>")
	require.NotEqual(t, -1, jsonldEnd, "JSON-LD script tag must be closed")

	jsonldContent := html[jsonldStart : jsonldStart+jsonldEnd]
	jsonldContent = strings.TrimSpace(jsonldContent)

	var jsonld map[string]any
	err = json.Unmarshal([]byte(jsonldContent), &jsonld)
	require.NoError(t, err, "Embedded JSON-LD must be valid JSON")

	// Verify JSON-LD structure
	require.Contains(t, jsonld, "@context", "Embedded JSON-LD must have @context")
	require.Contains(t, jsonld, "@type", "Embedded JSON-LD must have @type")
	require.Equal(t, "Event", jsonld["@type"], "Embedded JSON-LD must have @type Event")
	require.Contains(t, jsonld, "@id", "Embedded JSON-LD must have @id (canonical URI)")
	require.Contains(t, jsonld, "name", "Embedded JSON-LD must have name")

	// Verify event name in both HTML and JSON-LD
	require.Contains(t, html, eventName, "HTML must display event name")
	require.Equal(t, eventName, jsonld["name"], "JSON-LD name must match event name")
}

// TestHTMLEmbeddingWithOrganization verifies HTML with embedded JSON-LD for organizations
func TestHTMLEmbeddingWithOrganization(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Collective")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/organizations/"+org.ULID, nil)
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

	require.Contains(t, html, `<script type="application/ld+json">`)
	require.Contains(t, html, "Toronto Arts Collective")

	// Extract JSON-LD
	jsonldStart := strings.Index(html, `<script type="application/ld+json">`)
	jsonldStart += len(`<script type="application/ld+json">`)
	jsonldEnd := strings.Index(html[jsonldStart:], "</script>")
	jsonldContent := strings.TrimSpace(html[jsonldStart : jsonldStart+jsonldEnd])

	var jsonld map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonldContent), &jsonld))
	require.Equal(t, "Organization", jsonld["@type"])
	require.Contains(t, jsonld, "@id")
}

// TestHTMLEmbeddingWithPlace verifies HTML with embedded JSON-LD for places
func TestHTMLEmbeddingWithPlace(t *testing.T) {
	env := setupTestEnv(t)

	place := insertPlace(t, env, "Centennial Park", "Toronto")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/places/"+place.ULID, nil)
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

	require.Contains(t, html, `<script type="application/ld+json">`)
	require.Contains(t, html, "Centennial Park")

	// Extract JSON-LD
	jsonldStart := strings.Index(html, `<script type="application/ld+json">`)
	jsonldStart += len(`<script type="application/ld+json">`)
	jsonldEnd := strings.Index(html[jsonldStart:], "</script>")
	jsonldContent := strings.TrimSpace(html[jsonldStart : jsonldStart+jsonldEnd])

	var jsonld map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonldContent), &jsonld))
	require.Equal(t, "Place", jsonld["@type"])
	require.Contains(t, jsonld, "@id")
}

// TestHTMLMetaTags verifies that HTML responses include appropriate meta tags
func TestHTMLMetaTags(t *testing.T) {
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

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)

	// Verify meta tags for SEO and social sharing
	require.Contains(t, html, `<meta charset="UTF-8">`, "HTML must specify charset")
	require.Contains(t, html, `<meta name="viewport"`, "HTML must have viewport meta tag")
	require.Contains(t, html, "<title>", "HTML must have title tag")
	require.Contains(t, html, eventName, "HTML title or content must include event name")
}

// TestHTMLValidStructure verifies HTML structure follows semantic HTML5
func TestHTMLValidStructure(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)

	// Verify semantic HTML5 structure
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, `lang="en"`, "HTML must specify language")

	// Verify proper tag nesting (basic checks)
	htmlStart := strings.Index(html, "<html")
	htmlEnd := strings.LastIndex(html, "</html>")
	require.NotEqual(t, -1, htmlStart)
	require.NotEqual(t, -1, htmlEnd)
	require.Less(t, htmlStart, htmlEnd, "HTML tags must be properly nested")

	headStart := strings.Index(html, "<head>")
	headEnd := strings.Index(html, "</head>")
	require.Less(t, headStart, headEnd, "Head tags must be properly nested")

	bodyStart := strings.Index(html, "<body>")
	bodyEnd := strings.Index(html, "</body>")
	require.Less(t, bodyStart, bodyEnd, "Body tags must be properly nested")
	require.Less(t, headEnd, bodyStart, "Head must come before body")
}

// TestJSONLDConsistencyBetweenFormats verifies that JSON-LD content is consistent
// between direct JSON-LD responses and embedded JSON-LD in HTML
func TestJSONLDConsistencyBetweenFormats(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	// Get JSON-LD response
	jsonReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	jsonReq.Header.Set("Accept", "application/ld+json")

	jsonResp, err := env.Server.Client().Do(jsonReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsonResp.Body.Close() })

	var directJSONLD map[string]any
	require.NoError(t, json.NewDecoder(jsonResp.Body).Decode(&directJSONLD))

	// Get HTML response with embedded JSON-LD
	htmlReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	htmlReq.Header.Set("Accept", "text/html")

	htmlResp, err := env.Server.Client().Do(htmlReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = htmlResp.Body.Close() })

	body, err := io.ReadAll(htmlResp.Body)
	require.NoError(t, err)
	html := string(body)

	// Extract embedded JSON-LD
	jsonldStart := strings.Index(html, `<script type="application/ld+json">`)
	jsonldStart += len(`<script type="application/ld+json">`)
	jsonldEnd := strings.Index(html[jsonldStart:], "</script>")
	jsonldContent := strings.TrimSpace(html[jsonldStart : jsonldStart+jsonldEnd])

	var embeddedJSONLD map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonldContent), &embeddedJSONLD))

	// Verify key fields match
	require.Equal(t, directJSONLD["@type"], embeddedJSONLD["@type"], "@type must match")
	require.Equal(t, directJSONLD["@id"], embeddedJSONLD["@id"], "@id must match")
	require.Equal(t, directJSONLD["name"], embeddedJSONLD["name"], "name must match")

	// Verify @context exists in both (structure may differ but must be present)
	require.Contains(t, directJSONLD, "@context")
	require.Contains(t, embeddedJSONLD, "@context")
}
