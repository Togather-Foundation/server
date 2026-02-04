package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminLoginFormStaticResourcesServed verifies that static assets (CSS, JS) are accessible
func TestAdminLoginFormStaticResourcesServed(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		path        string
		contentType string
	}{
		{"/admin/static/css/admin.css", "text/css"},
		{"/admin/static/js/login.js", "text/javascript"}, // Go serves JS as text/javascript
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+tt.path, nil)
			require.NoError(t, err)

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "static asset should be served")
			contentType := resp.Header.Get("Content-Type")
			assert.Contains(t, contentType, tt.contentType, "correct content type for static asset")

			// Verify non-empty content
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.NotEmpty(t, body, "static asset should have content")
		})
	}
}

// TestAdminLoginPageContainsJavaScript verifies login.html includes the login.js script
func TestAdminLoginPageContainsJavaScript(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/login", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	html := string(body)
	assert.Contains(t, html, "/admin/static/js/login.js", "login page should include login.js script")
	assert.Contains(t, html, "/admin/static/css/admin.css", "login page should include admin.css stylesheet")
	assert.Contains(t, html, `id="error-message"`, "login page should have error message div")
}

// TestAdminLoginFormRejectsFormUrlencoded verifies that form-urlencoded submission is rejected
// This is the test that would have caught the original bug
func TestAdminLoginFormRejectsFormUrlencoded(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Simulate what an HTML form would send without JavaScript (application/x-www-form-urlencoded)
	formData := url.Values{}
	formData.Set("username", "admin")
	formData.Set("password", "test123")

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/admin/login", strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should reject form-urlencoded (400 Bad Request)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"login endpoint should reject application/x-www-form-urlencoded (expects application/json)")
}

// TestAdminLoginJavaScriptIntegration simulates what the JavaScript does:
// intercepts form submission and sends JSON instead of form-urlencoded
func TestAdminLoginJavaScriptIntegration(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Step 1: Verify login page loads
	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/login", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "login page should load")

	// Step 2: Verify JavaScript file is accessible
	jsReq, err := http.NewRequest(http.MethodGet, server.URL+"/admin/static/js/login.js", nil)
	require.NoError(t, err)

	jsResp, err := server.Client().Do(jsReq)
	require.NoError(t, err)
	defer func() { _ = jsResp.Body.Close() }()
	assert.Equal(t, http.StatusOK, jsResp.StatusCode, "login.js should be served")

	jsBody, err := io.ReadAll(jsResp.Body)
	require.NoError(t, err)

	jsContent := string(jsBody)
	// Verify the JavaScript has the key functionality
	assert.Contains(t, jsContent, "fetch('/api/v1/admin/login'", "JS should make fetch request")
	assert.Contains(t, jsContent, "'Content-Type': 'application/json'", "JS should set JSON content type")
	assert.Contains(t, jsContent, "JSON.stringify", "JS should send JSON body")
	assert.Contains(t, jsContent, "e.preventDefault()", "JS should prevent default form submission")

	// Step 3: Verify the API accepts JSON (simulating what the JavaScript sends)
	jsonLogin := `{"username":"admin","password":"test123"}`
	apiReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/admin/login", strings.NewReader(jsonLogin))
	require.NoError(t, err)
	apiReq.Header.Set("Content-Type", "application/json")

	apiResp, err := server.Client().Do(apiReq)
	require.NoError(t, err)
	defer func() { _ = apiResp.Body.Close() }()

	// Should either succeed (200) or fail with auth error (401), but not bad request (400)
	assert.NotEqual(t, http.StatusBadRequest, apiResp.StatusCode,
		"API should accept JSON login (what the JavaScript sends)")
}

// TestAdminLoginCSSLoadable verifies CSS is served correctly
func TestAdminLoginCSSLoadable(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/static/css/admin.css", nil)
	require.NoError(t, err)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "CSS file should be served")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	css := string(body)
	// Verify key CSS rules exist
	assert.Contains(t, css, ".login-container", "CSS should contain login container styles")
	assert.Contains(t, css, ".error-message", "CSS should contain error message styles")
}

// TestAdminLoginFormErrorHandling verifies error message display structure
func TestAdminLoginFormErrorHandling(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/login", nil)
	require.NoError(t, err)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	html := string(body)
	// Verify error handling structure
	assert.Contains(t, html, `id="error-message"`, "should have error message div")
	assert.Contains(t, html, `style="display: none;"`, "error message should be hidden by default")
	assert.Contains(t, html, `class="error-message"`, "should have error message class for styling")
}
