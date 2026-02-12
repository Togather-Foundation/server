package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/google/uuid"
)

func TestAdminAuthCookie_RejectsDeveloperToken(t *testing.T) {
	secret := "test-secret-key"
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	issuer := "https://test.togather.foundation"

	// Generate a developer token
	devToken, _, err := auth.GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("Failed to generate developer token: %v", err)
	}

	// Create admin middleware
	manager := auth.NewJWTManager(secret, 24, issuer)
	middleware := AdminAuthCookie(manager)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for developer token")
	}))

	// Try to use developer token in admin cookie
	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  AdminAuthCookieName,
		Value: devToken,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should be rejected (403 Forbidden due to isDeveloperToken check)
	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden for developer token, got %d", rec.Code)
	}
}

func TestIsDeveloperToken(t *testing.T) {
	secret := "test-secret-key"
	devID := uuid.New()
	issuer := "https://test.togather.foundation"

	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "developer token",
			token:    mustGenerateDeveloperToken(t, devID, "dev@example.com", "Dev Name", secret, issuer),
			expected: true,
		},
		{
			name:     "admin token",
			token:    mustGenerateAdminToken(t, "admin-user", "admin", secret, issuer),
			expected: false,
		},
		{
			name:     "empty token",
			token:    "",
			expected: false,
		},
		{
			name:     "invalid token",
			token:    "not-a-valid-token",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDeveloperToken(tt.token)
			if result != tt.expected {
				t.Errorf("isDeveloperToken(%s) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func mustGenerateDeveloperToken(t *testing.T, devID uuid.UUID, email, name, secret, issuer string) string {
	t.Helper()
	token, _, err := auth.GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("Failed to generate developer token: %v", err)
	}
	return token
}

func mustGenerateAdminToken(t *testing.T, subject, role, secret, issuer string) string {
	t.Helper()
	manager := auth.NewJWTManager(secret, 24, issuer)
	token, err := manager.Generate(subject, role)
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}
	return token
}
