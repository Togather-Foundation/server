package integration

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuthStubRoutes404 verifies that the OAuth stub routes /authorize, /token,
// and /register return 404 instead of the SPA landing page. When a conformant MCP
// client mistakenly triggers an OAuth attempt on a server that does not implement
// OAuth (no discovery documents, no IdP), a 200 text/html response makes the flow
// appear to begin and then silently never complete (an endless "still unauthorized"
// loop). A 404 fails immediately and legibly. Regression for #17 (OAUTH-03).
func TestOAuthStubRoutes404(t *testing.T) {
	env := setupTestEnv(t)

	paths := []string{
		"/authorize?response_type=code&client_id=test",
		"/token",
		"/register",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode,
				"OAuth stub route should 404, not fall through to the SPA catch-all")
			assert.NotContains(t, resp.Header.Get("Content-Type"), "text/html",
				"a 404 for an OAuth stub route must not serve the HTML landing page")
		})
	}

	// The token grant is conventionally sent via POST (RFC 6749 §3.2); pin that
	// path too, not just GET.
	t.Run("POST /token", func(t *testing.T) {
		body := bytes.NewReader([]byte("grant_type=client_credentials"))
		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/token", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"POST /token should 404, not fall through to the SPA catch-all")
		assert.NotContains(t, resp.Header.Get("Content-Type"), "text/html",
			"a 404 for POST /token must not serve the HTML landing page")
	})

	// Guard against the 404 handlers over-matching: the SPA catch-all must
	// still serve the landing page for an unrelated path.
	t.Run("SPA catch-all still serves landing page", func(t *testing.T) {
		resp, err := http.Get(env.Server.URL + "/")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	})
}
