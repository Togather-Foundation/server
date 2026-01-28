package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminAPIRoutesRequireBearerToken tests that /api/v1/admin/* routes require Authorization header with Bearer token
func TestAdminAPIRoutesRequireBearerToken(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Get JWT token
	token := adminLogin(t, env, username, password)

	apiRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/events/pending"},
		{http.MethodGet, "/api/v1/admin/events"},
		{http.MethodPut, "/api/v1/admin/events/01234567890123456789012345"},
		{http.MethodDelete, "/api/v1/admin/events/01234567890123456789012345"},
		{http.MethodPost, "/api/v1/admin/events/merge"},
		{http.MethodGet, "/api/v1/admin/api-keys"},
		{http.MethodPost, "/api/v1/admin/api-keys"},
	}

	for _, route := range apiRoutes {
		t.Run(route.method+" "+route.path+" with Bearer token", func(t *testing.T) {
			req, err := http.NewRequest(route.method, env.Server.URL+route.path, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "application/json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should NOT be unauthorized or forbidden (might be 404 if endpoint not implemented yet)
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "should accept Bearer token")
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "should not be forbidden with admin role")
		})

		t.Run(route.method+" "+route.path+" without token", func(t *testing.T) {
			req, err := http.NewRequest(route.method, env.Server.URL+route.path, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "application/json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should be unauthorized without token
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "should be unauthorized without token")
		})
	}
}

// TestAdminHTMLRoutesRequireCookie tests that /admin/* HTML routes require auth_token cookie
func TestAdminHTMLRoutesRequireCookie(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Login to get cookie
	loginPayload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	loginReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := env.Server.Client().Do(loginReq)
	require.NoError(t, err)
	defer func() { _ = loginResp.Body.Close() }()
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	// Extract cookie
	var authCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	require.NotNil(t, authCookie, "expected auth_token cookie")

	htmlRoutes := []string{
		"/admin",
		"/admin/dashboard",
		"/admin/events",
		"/admin/events/pending",
		"/admin/events/01234567890123456789012345/edit",
		"/admin/duplicates",
		"/admin/api-keys",
	}

	for _, path := range htmlRoutes {
		t.Run(path+" with cookie", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)
			req.AddCookie(authCookie)
			req.Header.Set("Accept", "text/html")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should NOT be unauthorized (might be 404 if route not implemented yet)
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "should accept cookie")
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "should not be forbidden with admin role")
		})

		t.Run(path+" without cookie", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "text/html")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should redirect to login or return unauthorized
			assert.True(t,
				resp.StatusCode == http.StatusUnauthorized ||
					resp.StatusCode == http.StatusFound ||
					resp.StatusCode == http.StatusSeeOther,
				"should be unauthorized or redirect without cookie, got %d", resp.StatusCode)
		})
	}
}

// TestAdminHTMLRoutesRejectBearerToken tests that /admin/* HTML routes don't accept Bearer tokens (only cookies)
func TestAdminHTMLRoutesRejectBearerToken(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Get JWT token
	token := adminLogin(t, env, username, password)

	htmlRoutes := []string{
		"/admin/dashboard",
		"/admin/events",
	}

	for _, path := range htmlRoutes {
		t.Run(path+" with Bearer token (should reject)", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "text/html")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// HTML routes should NOT accept Bearer tokens (require cookies instead)
			// Should be unauthorized or redirect to login
			assert.True(t,
				resp.StatusCode == http.StatusUnauthorized ||
					resp.StatusCode == http.StatusFound ||
					resp.StatusCode == http.StatusSeeOther,
				"HTML routes should not accept Bearer token, got %d", resp.StatusCode)
		})
	}
}

// TestAdminAPIRoutesRejectCookie tests that /api/v1/admin/* routes don't accept cookies (only Bearer tokens)
func TestAdminAPIRoutesRejectCookie(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Login to get cookie
	loginPayload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	loginReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := env.Server.Client().Do(loginReq)
	require.NoError(t, err)
	defer func() { _ = loginResp.Body.Close() }()
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	// Extract cookie
	var authCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	require.NotNil(t, authCookie, "expected auth_token cookie")

	apiRoutes := []string{
		"/api/v1/admin/events/pending",
		"/api/v1/admin/api-keys",
	}

	for _, path := range apiRoutes {
		t.Run(path+" with cookie only (should reject)", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
			require.NoError(t, err)
			req.AddCookie(authCookie)
			req.Header.Set("Accept", "application/json")
			// Explicitly do NOT set Authorization header

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// API routes should NOT accept cookies (require Bearer token instead)
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "API routes should not accept cookie without Bearer token")
		})
	}
}

// TestPublicRoutesNoAuth tests that public routes don't require authentication
func TestPublicRoutesNoAuth(t *testing.T) {
	env := setupTestEnv(t)

	publicRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/events"},
		{http.MethodGet, "/api/v1/places"},
		{http.MethodGet, "/api/v1/organizations"},
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/api/v1/openapi.json"},
	}

	for _, route := range publicRoutes {
		t.Run(route.method+" "+route.path+" without auth", func(t *testing.T) {
			req, err := http.NewRequest(route.method, env.Server.URL+route.path, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "application/json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should NOT be unauthorized or forbidden
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "public route should not require auth")
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "public route should not be forbidden")
		})
	}
}

// TestAdminLoginPageNoAuth tests that /admin/login page is accessible without authentication
func TestAdminLoginPageNoAuth(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/admin/login", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Login page should be accessible without auth (might be 404 if not implemented yet)
	assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "login page should be accessible without auth")
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "login page should not be forbidden")
}
