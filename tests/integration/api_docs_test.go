package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIDocumentation verifies the /api/docs endpoint serves Scalar UI (server-6lnc)
func TestAPIDocumentation(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("GET /api/docs returns HTML with Scalar UI", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/docs", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

		// Verify HTML contains key Scalar UI elements
		body := readBody(t, resp)
		assert.Contains(t, body, "<!doctype html>")
		assert.Contains(t, body, "Togather API Documentation")
		assert.Contains(t, body, "Scalar")
		assert.Contains(t, body, "/api/v1/openapi.json")
	})

	t.Run("GET /api/docs/ with trailing slash returns HTML", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/docs/", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	})

	t.Run("GET /api/docs/scalar-standalone.js returns JavaScript", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/docs/scalar-standalone.js", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "javascript")

		// Verify JS file has substantial content
		body := readBody(t, resp)
		assert.Greater(t, len(body), 1000, "Scalar JS bundle should be substantial")
	})

	t.Run("POST /api/docs returns 405 Method Not Allowed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/docs", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("Cache headers are set correctly", func(t *testing.T) {
		// Test HTML cache headers (should not be cached)
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/docs", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, "no-cache, must-revalidate", resp.Header.Get("Cache-Control"))

		// Test JS cache headers (should have long-term caching)
		req, err = http.NewRequest(http.MethodGet, env.Server.URL+"/api/docs/scalar-standalone.js", nil)
		require.NoError(t, err)

		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
	})
}

// TestAPIDocsDiscovery verifies docs are discoverable via health and well-known endpoints (server-6lnc)
func TestAPIDocsDiscovery(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("Health check includes docs_url field", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/health", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var health map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&health)
		require.NoError(t, err)

		// Verify docs_url field exists and points to /api/docs
		docsURL, exists := health["docs_url"]
		assert.True(t, exists, "health response should include docs_url field")
		assert.Equal(t, "/api/docs", docsURL)
	})

	t.Run("Well-known SEL profile includes api_documentation field", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/.well-known/sel-profile", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var profile map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&profile)
		require.NoError(t, err)

		// Verify api_documentation field exists and includes /api/docs
		apiDocs, exists := profile["api_documentation"]
		assert.True(t, exists, "SEL profile should include api_documentation field")
		apiDocsStr, ok := apiDocs.(string)
		require.True(t, ok, "api_documentation should be a string")
		assert.True(t, strings.HasSuffix(apiDocsStr, "/api/docs"),
			"api_documentation should end with /api/docs, got: %s", apiDocsStr)
	})
}

// Helper function to read response body
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
