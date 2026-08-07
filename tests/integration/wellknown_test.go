package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWellKnownSELProfile tests the /.well-known/sel-profile endpoint (Interoperability Profile §1.7)
func TestWellKnownSELProfile(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/.well-known/sel-profile", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Verify status code
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify content type
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "application/json", "content type should be application/json")

	// Decode response
	var profile map[string]any
	err = json.NewDecoder(resp.Body).Decode(&profile)
	require.NoError(t, err)

	// Verify required fields per Interoperability Profile §1.7
	assert.Equal(t, "https://sel.events/profiles/interop", profile["profile"], "profile should match")
	assert.NotEmpty(t, profile["version"], "version should be present")
	assert.NotEmpty(t, profile["node"], "node should be present")
	assert.NotEmpty(t, profile["updated"], "updated should be present")

	// Verify version format
	version, ok := profile["version"].(string)
	require.True(t, ok, "version should be a string")
	assert.Regexp(t, `^\d+\.\d+\.\d+$`, version, "version should match semantic versioning (e.g., 0.1.0)")

	// Verify node is a URL
	node, ok := profile["node"].(string)
	require.True(t, ok, "node should be a string")
	assert.Regexp(t, `^https?://`, node, "node should be a URL")

	// Verify updated is a date (YYYY-MM-DD format)
	updated, ok := profile["updated"].(string)
	require.True(t, ok, "updated should be a string")
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, updated, "updated should be in YYYY-MM-DD format")
}

// TestWellKnownUnknownPath404 verifies that unknown /.well-known/* paths return 404
// instead of the SPA landing page. OAuth/well-known clients (e.g. MCP connectors
// probing for /.well-known/oauth-authorization-server) must be able to detect "not
// available" rather than parsing an HTML page as JSON.
func TestWellKnownUnknownPath404(t *testing.T) {
	env := setupTestEnv(t)

	paths := []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource",
		"/.well-known/does-not-exist",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode,
				"unregistered .well-known path should 404, not fall through to the SPA catch-all")
			assert.NotContains(t, resp.Header.Get("Content-Type"), "text/html",
				"a 404 for .well-known must not serve the HTML landing page")
		})
	}
}
