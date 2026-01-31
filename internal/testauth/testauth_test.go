package testauth

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestNewTestAuthenticator_JWT(t *testing.T) {
	auth, err := NewTestAuthenticator(Config{
		Mode:      AuthModeJWT,
		JWTSecret: "test_secret",
		Role:      "admin",
		Subject:   "test-user",
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	if auth.mode != AuthModeJWT {
		t.Errorf("expected mode JWT, got %s", auth.mode)
	}

	header := auth.GetAuthHeader()
	if !strings.HasPrefix(header, "Bearer ") {
		t.Errorf("expected Bearer prefix, got %s", header)
	}
}

func TestNewTestAuthenticator_APIKey(t *testing.T) {
	auth, err := NewTestAuthenticator(Config{
		Mode:   AuthModeAPIKey,
		APIKey: "test_api_key_12345",
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	if auth.mode != AuthModeAPIKey {
		t.Errorf("expected mode APIKey, got %s", auth.mode)
	}

	header := auth.GetAuthHeader()
	expected := "Bearer test_api_key_12345"
	if header != expected {
		t.Errorf("expected %s, got %s", expected, header)
	}
}

func TestNewTestAuthenticator_None(t *testing.T) {
	auth, err := NewTestAuthenticator(Config{
		Mode: AuthModeNone,
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	if auth.mode != AuthModeNone {
		t.Errorf("expected mode None, got %s", auth.mode)
	}

	header := auth.GetAuthHeader()
	if header != "" {
		t.Errorf("expected empty header, got %s", header)
	}
}

func TestAddAuth_JWT(t *testing.T) {
	auth, err := NewTestAuthenticator(Config{
		Mode:      AuthModeJWT,
		JWTSecret: "test_secret",
		Role:      "admin",
		Subject:   "test-user",
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.AddAuth(req)

	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("expected Bearer prefix, got %s", authHeader)
	}
}

func TestAddAuth_None(t *testing.T) {
	auth, err := NewTestAuthenticator(Config{
		Mode: AuthModeNone,
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.AddAuth(req)

	authHeader := req.Header.Get("Authorization")
	if authHeader != "" {
		t.Errorf("expected no auth header, got %s", authHeader)
	}
}

func TestNewDevAuthenticator(t *testing.T) {
	auth, err := NewDevAuthenticator()
	if err != nil {
		t.Fatalf("failed to create dev authenticator: %v", err)
	}

	if auth.mode != AuthModeJWT {
		t.Errorf("expected mode JWT, got %s", auth.mode)
	}

	header := auth.GetAuthHeader()
	if !strings.HasPrefix(header, "Bearer ") {
		t.Errorf("expected Bearer prefix, got %s", header)
	}
}

func TestDevJWTToken(t *testing.T) {
	token, err := DevJWTToken("admin", "test-user")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}

	// Verify it looks like a JWT (3 parts separated by dots)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestDevJWTToken_EnvVar(t *testing.T) {
	// Set custom secret
	os.Setenv("DEV_JWT_SECRET", "custom_test_secret")
	defer os.Unsetenv("DEV_JWT_SECRET")

	token, err := DevJWTToken("admin", "test-user")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestNewAPIKeyAuthenticator(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator("my_test_api_key_123")
	if err != nil {
		t.Fatalf("failed to create API key authenticator: %v", err)
	}

	if auth.mode != AuthModeAPIKey {
		t.Errorf("expected mode APIKey, got %s", auth.mode)
	}

	header := auth.GetAuthHeader()
	expected := "Bearer my_test_api_key_123"
	if header != expected {
		t.Errorf("expected %s, got %s", expected, header)
	}
}
