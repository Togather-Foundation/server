package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminLoginSuccess tests successful admin login and JWT generation
func TestAdminLoginSuccess(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Attempt login
	loginPayload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "expected successful login")

	// Verify JWT in response body
	var loginResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&loginResp))
	token, ok := loginResp["token"].(string)
	require.True(t, ok, "expected token in response")
	require.NotEmpty(t, token, "expected non-empty token")

	// Verify JWT can be validated
	jwtManager := auth.NewJWTManager(env.Config.Auth.JWTSecret, env.Config.Auth.JWTExpiry, "togather")
	claims, err := jwtManager.Validate(token)
	require.NoError(t, err, "expected valid JWT")
	require.Equal(t, username, claims.Subject)
	require.Equal(t, "admin", claims.Role)

	// Verify HttpOnly cookie is set
	cookies := resp.Cookies()
	var authCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	require.NotNil(t, authCookie, "expected auth_token cookie")
	require.True(t, authCookie.HttpOnly, "expected HttpOnly cookie")
	require.True(t, authCookie.Secure || env.Config.Environment == "test", "expected Secure cookie in non-test env")
	require.NotEmpty(t, authCookie.Value, "expected cookie value")
}

// TestAdminLoginInvalidCredentials tests login with wrong password
func TestAdminLoginInvalidCredentials(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	// Attempt login with wrong password
	loginPayload := map[string]string{
		"username": username,
		"password": "wrong-password",
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected unauthorized")
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

// TestAdminLoginNonexistentUser tests login with username that doesn't exist
func TestAdminLoginNonexistentUser(t *testing.T) {
	env := setupTestEnv(t)

	loginPayload := map[string]string{
		"username": "nonexistent",
		"password": "any-password",
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected unauthorized")
}

// TestAdminLoginMissingFields tests login with missing required fields
func TestAdminLoginMissingFields(t *testing.T) {
	env := setupTestEnv(t)

	tests := []struct {
		name    string
		payload map[string]string
	}{
		{
			name:    "missing username",
			payload: map[string]string{"password": "test123"},
		},
		{
			name:    "missing password",
			payload: map[string]string{"username": "admin"},
		},
		{
			name:    "empty username",
			payload: map[string]string{"username": "", "password": "test123"},
		},
		{
			name:    "empty password",
			payload: map[string]string{"username": "admin", "password": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "expected bad request")
		})
	}
}

// TestAdminJWTAuthorizationHeader tests using JWT via Authorization header
func TestAdminJWTAuthorizationHeader(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")

	token := adminLogin(t, env, username, password)

	// Use JWT to access protected admin API endpoint
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed (or return 200 with empty list if endpoint exists)
	// If endpoint not implemented yet, might get 404, but shouldn't get 401
	assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "should not be unauthorized with valid JWT")
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "should not be forbidden with admin role")
}

// TestAdminJWTCookie tests using JWT via HttpOnly cookie
func TestAdminJWTCookie(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT cookie
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
	defer loginResp.Body.Close()
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

	// Use cookie to access protected HTML admin route
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/admin/dashboard", nil)
	require.NoError(t, err)
	req.AddCookie(authCookie)
	req.Header.Set("Accept", "text/html")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should not be unauthorized (might be 404 if route not implemented, but not 401)
	assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "should not be unauthorized with valid cookie")
}

// TestAdminJWTExpired tests that expired JWT is rejected
func TestAdminJWTExpired(t *testing.T) {
	env := setupTestEnv(t)

	// Create an expired JWT manually
	jwtManager := auth.NewJWTManager(env.Config.Auth.JWTSecret, -1, "togather") // negative expiry = expired
	expiredToken, err := jwtManager.Generate("admin", "admin")
	require.NoError(t, err)

	// Try to use expired JWT
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected unauthorized for expired token")
}

// TestAdminJWTMalformed tests that malformed JWT is rejected
func TestAdminJWTMalformed(t *testing.T) {
	env := setupTestEnv(t)

	malformedTokens := []string{
		"not.a.jwt",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid",
		"",
		"Bearer token-without-bearer-prefix",
	}

	for _, token := range malformedTokens {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
		require.NoError(t, err)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected unauthorized for malformed token: %s", token)
	}
}

// TestAdminInsufficientRole tests that non-admin roles are rejected
func TestAdminInsufficientRole(t *testing.T) {
	env := setupTestEnv(t)

	// Insert viewer user (not admin)
	username := "viewer"
	password := "viewer-password-123"
	email := "viewer@example.com"
	insertAdminUser(t, env, username, password, email, "viewer")

	token := adminLogin(t, env, username, password)

	// Try to access admin endpoint with viewer role
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should be forbidden (403) not unauthorized (401)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "expected forbidden for non-admin role")
}
