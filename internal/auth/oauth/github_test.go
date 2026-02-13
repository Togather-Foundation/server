package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateAuthURL(t *testing.T) {
	config := GitHubConfig{
		ClientID:    "test-client-id",
		CallbackURL: "http://localhost:8080/auth/github/callback",
	}

	client := NewGitHubClient(config)
	state := "test-state-value"

	authURL := client.GenerateAuthURL(state)

	// Parse the URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Failed to parse auth URL: %v", err)
	}

	// Verify base URL
	if parsedURL.Scheme != "https" {
		t.Errorf("Expected https scheme, got: %s", parsedURL.Scheme)
	}
	if parsedURL.Host != "github.com" {
		t.Errorf("Expected github.com host, got: %s", parsedURL.Host)
	}
	if parsedURL.Path != "/login/oauth/authorize" {
		t.Errorf("Expected /login/oauth/authorize path, got: %s", parsedURL.Path)
	}

	// Verify query parameters
	query := parsedURL.Query()

	requiredParams := map[string]string{
		"client_id":    config.ClientID,
		"redirect_uri": config.CallbackURL,
		"scope":        "user:email",
		"state":        state,
	}

	for param, expectedValue := range requiredParams {
		actualValue := query.Get(param)
		if actualValue != expectedValue {
			t.Errorf("Expected %s=%s, got %s=%s", param, expectedValue, param, actualValue)
		}
	}
}

func TestExchangeCode_Success(t *testing.T) {
	// Mock GitHub token endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept: application/json header")
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Expected Content-Type: application/x-www-form-urlencoded header")
		}

		// Verify request body
		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse form: %v", err)
		}
		if r.FormValue("code") != "test-code" {
			t.Errorf("Expected code=test-code, got code=%s", r.FormValue("code"))
		}

		// Return successful token response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "gho_test_token_123",
			"token_type":   "bearer",
			"scope":        "user:email",
		})
	}))
	defer server.Close()

	config := GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		CallbackURL:  "http://localhost:8080/callback",
	}

	client := NewGitHubClient(config)
	// Override the token endpoint for testing
	originalClient := client.httpClient
	client.httpClient = server.Client()

	// Create custom request to use test server
	ctx := context.Background()

	// We need to actually test against the mock server, so let's create a wrapper
	// that intercepts the GitHub URL and redirects to our test server
	testClient := &http.Client{
		Transport: &mockTransport{
			handler:   server.Config.Handler,
			serverURL: server.URL,
		},
		Timeout: originalClient.Timeout,
	}
	client.httpClient = testClient

	token, err := client.ExchangeCode(ctx, "test-code")
	if err != nil {
		t.Fatalf("ExchangeCode failed: %v", err)
	}

	if token != "gho_test_token_123" {
		t.Errorf("Expected token gho_test_token_123, got %s", token)
	}
}

func TestExchangeCode_InvalidCode(t *testing.T) {
	// Mock GitHub token endpoint returning error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "bad_verification_code",
			"error_description": "The code passed is incorrect or expired.",
		})
	}))
	defer server.Close()

	config := GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		CallbackURL:  "http://localhost:8080/callback",
	}

	client := NewGitHubClient(config)
	client.httpClient = &http.Client{
		Transport: &mockTransport{
			handler:   server.Config.Handler,
			serverURL: server.URL,
		},
		Timeout: client.httpClient.Timeout,
	}

	ctx := context.Background()
	_, err := client.ExchangeCode(ctx, "invalid-code")
	if err == nil {
		t.Fatal("Expected error for invalid code, got nil")
	}

	if !strings.Contains(err.Error(), "bad_verification_code") {
		t.Errorf("Expected error to contain 'bad_verification_code', got: %v", err)
	}
}

func TestExchangeCode_NetworkError(t *testing.T) {
	config := GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		CallbackURL:  "http://localhost:8080/callback",
	}

	client := NewGitHubClient(config)
	// Use invalid server URL to trigger network error
	client.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// This will never be called
			}),
			serverURL:  "http://invalid-server-that-does-not-exist:9999",
			forceError: true,
		},
		Timeout: client.httpClient.Timeout,
	}

	ctx := context.Background()
	_, err := client.ExchangeCode(ctx, "test-code")
	if err == nil {
		t.Fatal("Expected network error, got nil")
	}
}

func TestFetchUserProfile_Success(t *testing.T) {
	// Mock GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("Expected Bearer token in Authorization header, got: %s", authHeader)
		}

		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    int64(12345),
				"login": "testuser",
				"email": "testuser@example.com",
				"name":  "Test User",
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config := GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}

	client := NewGitHubClient(config)
	client.httpClient = &http.Client{
		Transport: &mockTransport{
			handler:   server.Config.Handler,
			serverURL: server.URL,
		},
		Timeout: client.httpClient.Timeout,
	}

	ctx := context.Background()
	user, err := client.FetchUserProfile(ctx, "test-token")
	if err != nil {
		t.Fatalf("FetchUserProfile failed: %v", err)
	}

	if user.ID != 12345 {
		t.Errorf("Expected user ID 12345, got %d", user.ID)
	}
	if user.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got %s", user.Login)
	}
	if user.Email != "testuser@example.com" {
		t.Errorf("Expected email 'testuser@example.com', got %s", user.Email)
	}
	if user.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got %s", user.Name)
	}
}

func TestFetchUserProfile_NoEmail_FallbackToEmailsEndpoint(t *testing.T) {
	// Mock GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/user" {
			// User profile without email (private email setting)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    int64(12345),
				"login": "testuser",
				"email": "", // Empty email
				"name":  "Test User",
			})
		} else if r.URL.Path == "/user/emails" {
			// Emails endpoint
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"email":    "secondary@example.com",
					"primary":  false,
					"verified": true,
				},
				{
					"email":    "primary@example.com",
					"primary":  true,
					"verified": true,
				},
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config := GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}

	client := NewGitHubClient(config)
	client.httpClient = &http.Client{
		Transport: &mockTransport{
			handler:   server.Config.Handler,
			serverURL: server.URL,
		},
		Timeout: client.httpClient.Timeout,
	}

	ctx := context.Background()
	user, err := client.FetchUserProfile(ctx, "test-token")
	if err != nil {
		t.Fatalf("FetchUserProfile failed: %v", err)
	}

	// Should have fetched the primary email from /user/emails
	if user.Email != "primary@example.com" {
		t.Errorf("Expected email 'primary@example.com', got %s", user.Email)
	}
}

func TestGenerateState(t *testing.T) {
	// Test that state generation works
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	// Verify it's base64url encoded
	decoded, err := base64.URLEncoding.DecodeString(state1)
	if err != nil {
		t.Errorf("State is not valid base64url: %v", err)
	}

	// Verify it's 32 bytes
	if len(decoded) != 32 {
		t.Errorf("Expected 32 bytes, got %d", len(decoded))
	}

	// Test randomness - generate another state and verify they're different
	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed on second call: %v", err)
	}

	if state1 == state2 {
		t.Error("Expected different state values, got identical values (possible randomness issue)")
	}
}

// mockTransport is a custom http.RoundTripper for testing that redirects requests to our test server
type mockTransport struct {
	handler    http.Handler
	serverURL  string
	forceError bool
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.forceError {
		return nil, http.ErrHandlerTimeout
	}

	// Rewrite the request URL to point to our test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(m.serverURL, "http://")

	// Create a response recorder
	rec := httptest.NewRecorder()

	// Execute the handler
	m.handler.ServeHTTP(rec, req)

	// Convert to http.Response
	return rec.Result(), nil
}
