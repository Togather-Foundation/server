package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/google/uuid"
)

func TestDevCookieAuth_Success(t *testing.T) {
	secret := []byte("test-secret-key")
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	issuer := "https://test.togather.foundation"

	// Generate a valid developer token
	token, _, err := auth.GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Create middleware
	middleware := DevCookieAuth(secret)

	// Create test handler
	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		// Verify claims in context
		claims := DeveloperClaims(r)
		if claims == nil {
			t.Error("Expected developer claims in context")
			return
		}

		if claims.DeveloperID != devID {
			t.Errorf("Expected developer ID %v, got %v", devID, claims.DeveloperID)
		}

		if claims.Email != email {
			t.Errorf("Expected email %s, got %s", email, claims.Email)
		}

		if claims.Type != "developer" {
			t.Errorf("Expected type 'developer', got %s", claims.Type)
		}

		w.WriteHeader(http.StatusOK)
	}))

	// Create request with developer cookie
	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  DevAuthCookieName,
		Value: token,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Handler was not called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestDevCookieAuth_MissingCookie(t *testing.T) {
	secret := []byte("test-secret-key")
	middleware := DevCookieAuth(secret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestDevCookieAuth_InvalidToken(t *testing.T) {
	secret := []byte("test-secret-key")
	middleware := DevCookieAuth(secret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  DevAuthCookieName,
		Value: "invalid.token.here",
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestDevCookieAuth_RejectsBearerToken(t *testing.T) {
	secret := []byte("test-secret-key")
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	issuer := "https://test.togather.foundation"

	token, _, err := auth.GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	middleware := DevCookieAuth(secret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	// Send token as Bearer token in Authorization header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{
		Name:  DevAuthCookieName,
		Value: token,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should be rejected because Bearer tokens are not allowed
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestDevCookieAuth_EmptySecret(t *testing.T) {
	middleware := DevCookieAuth([]byte(""))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  DevAuthCookieName,
		Value: "some-token",
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestDevCookieAuth_RejectsAdminToken(t *testing.T) {
	secret := []byte("test-secret-key")

	// Generate an admin token
	manager := auth.NewJWTManager("test-secret-key", 24*time.Hour, "https://test.togather.foundation")
	adminToken, err := manager.Generate("admin-user", "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	middleware := DevCookieAuth(secret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for admin token")
	}))

	// Try to use admin token in developer cookie
	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  DevAuthCookieName,
		Value: adminToken,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should be rejected because admin tokens don't have type="developer"
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestDeveloperFromContext_NilContext(t *testing.T) {
	claims := DeveloperFromContext(context.Background())
	if claims != nil {
		t.Error("Expected nil claims for nil context")
	}
}

func TestDeveloperClaims_NilRequest(t *testing.T) {
	claims := DeveloperClaims(nil)
	if claims != nil {
		t.Error("Expected nil claims for nil request")
	}
}
