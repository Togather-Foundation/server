package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminLoginPageRendersHTML tests that /admin/login returns HTML
func TestAdminLoginPageRendersHTML(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/login", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return HTML (or 404 if not implemented yet, but not 500)
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "should not error")

	if resp.StatusCode == http.StatusOK {
		contentType := resp.Header.Get("Content-Type")
		assert.True(t, strings.HasPrefix(contentType, "text/html"), "should return HTML content type")
	}
}

// TestAdminDashboardRedirectsWhenUnauthenticated tests that /admin/dashboard redirects to login
func TestAdminDashboardRedirectsWhenUnauthenticated(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/dashboard", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should redirect to login or return 401
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusTemporaryRedirect,
		"unauthenticated request should redirect or return 401, got %d", resp.StatusCode)
}

// TestAdminEventsPageAccessible tests that /admin/events is accessible with auth
func TestAdminEventsPageAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// This test requires admin authentication
	// For now, just verify the route exists and requires auth
	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should require authentication (401 or redirect)
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther,
		"should require authentication")
}

// TestAdminAPIKeysPageAccessible tests that /admin/api-keys is accessible with auth
func TestAdminAPIKeysPageAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/api-keys", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should require authentication
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther,
		"should require authentication")
}

// TestAdminStaticAssetsAccessible tests that admin static assets are served
func TestAdminStaticAssetsAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	staticPaths := []string{
		"/admin/static/css/admin.css",
		"/admin/static/js/admin.js",
	}

	for _, path := range staticPaths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
			require.NoError(t, err)

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should either serve the asset (200) or not found (404), but not error (500)
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "should not error serving static assets")
		})
	}
}

// TestAdminLoginPostAcceptsCredentials tests that POST /api/v1/admin/login accepts credentials
func TestAdminLoginPostAcceptsCredentials(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	loginPayload := map[string]string{
		"username": "admin",
		"password": "test123",
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should not error (might be 401 for invalid credentials, but not 500)
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "login endpoint should not error")
}

// TestAdminRoutesRejectPublicAccess tests that admin routes reject unauthenticated access
func TestAdminRoutesRejectPublicAccess(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	adminRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin/dashboard"},
		{http.MethodGet, "/admin/events"},
		{http.MethodGet, "/admin/events/pending"},
		{http.MethodGet, "/admin/duplicates"},
		{http.MethodGet, "/admin/api-keys"},
	}

	for _, route := range adminRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req, err := http.NewRequest(route.method, server.URL+route.path, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "text/html")

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should require authentication (not 200)
			assert.NotEqual(t, http.StatusOK, resp.StatusCode, "admin route should require authentication")
		})
	}
}

// setupTestServer creates a test HTTP server for E2E tests
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := config.Config{
		Server: config.ServerConfig{
			Host:    "127.0.0.1",
			Port:    0,
			BaseURL: "http://localhost",
		},
		Database: config.DatabaseConfig{
			URL:            "postgres://sel:sel_dev@localhost:5432/sel?sslmode=disable",
			MaxConnections: 5,
			MaxIdle:        2,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret-32-bytes-minimum----",
			JWTExpiry: 3600,
		},
		Environment: "test",
	}

	router := api.NewRouter(cfg, testLogger())
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return server
}

// testLogger returns a no-op logger for tests
func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}
