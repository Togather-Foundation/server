package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateDeveloperToken(t *testing.T) {
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	secret := []byte("test-secret-key")
	expiryHours := 24
	issuer := "https://test.togather.foundation"

	token, expiresAt, err := GenerateDeveloperToken(devID, email, name, secret, expiryHours, issuer)
	if err != nil {
		t.Fatalf("GenerateDeveloperToken failed: %v", err)
	}

	if token == "" {
		t.Error("Expected non-empty token")
	}

	if time.Until(expiresAt) > time.Duration(expiryHours+1)*time.Hour {
		t.Error("Token expiry time is too far in the future")
	}
}

func TestGenerateDeveloperToken_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		devID       uuid.UUID
		email       string
		secret      []byte
		expectError bool
	}{
		{"nil developer ID", uuid.Nil, "dev@example.com", []byte("secret"), true},
		{"empty email", uuid.New(), "", []byte("secret"), true},
		{"empty secret", uuid.New(), "dev@example.com", []byte(""), true},
		{"valid inputs", uuid.New(), "dev@example.com", []byte("secret"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GenerateDeveloperToken(tt.devID, tt.email, "Test Dev", tt.secret, 24, "https://test.togather.foundation")
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDeveloperToken(t *testing.T) {
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	secret := []byte("test-secret-key")
	issuer := "https://test.togather.foundation"

	token, _, err := GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("GenerateDeveloperToken failed: %v", err)
	}

	claims, err := ValidateDeveloperToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateDeveloperToken failed: %v", err)
	}

	if claims.DeveloperID != devID {
		t.Errorf("Expected developer ID %v, got %v", devID, claims.DeveloperID)
	}

	if claims.Email != email {
		t.Errorf("Expected email %s, got %s", email, claims.Email)
	}

	if claims.Name != name {
		t.Errorf("Expected name %s, got %s", name, claims.Name)
	}

	if claims.Type != "developer" {
		t.Errorf("Expected type 'developer', got %s", claims.Type)
	}

	if claims.Subject != "developer" {
		t.Errorf("Expected subject 'developer', got %s", claims.Subject)
	}
}

func TestValidateDeveloperToken_Invalid(t *testing.T) {
	secret := []byte("test-secret-key")

	tests := []struct {
		name        string
		token       string
		expectError error
	}{
		{"empty token", "", ErrMissingToken},
		{"invalid token", "invalid.token.here", ErrInvalidToken},
		{"whitespace token", "   ", ErrMissingToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateDeveloperToken(tt.token, secret)
			if err != tt.expectError {
				t.Errorf("Expected error %v, got %v", tt.expectError, err)
			}
		})
	}
}

func TestValidateDeveloperToken_WrongSecret(t *testing.T) {
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	secret := []byte("correct-secret")
	issuer := "https://test.togather.foundation"

	token, _, err := GenerateDeveloperToken(devID, email, name, secret, 24, issuer)
	if err != nil {
		t.Fatalf("GenerateDeveloperToken failed: %v", err)
	}

	_, err = ValidateDeveloperToken(token, []byte("wrong-secret"))
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken, got %v", err)
	}
}

func TestValidateDeveloperToken_ExpiredToken(t *testing.T) {
	devID := uuid.New()
	email := "dev@example.com"
	name := "Test Developer"
	secret := []byte("test-secret-key")
	issuer := "https://test.togather.foundation"

	// Generate token with negative expiry (already expired)
	token, _, err := GenerateDeveloperToken(devID, email, name, secret, -1, issuer)
	if err != nil {
		t.Fatalf("GenerateDeveloperToken failed: %v", err)
	}

	// Validation should fail for expired token
	_, err = ValidateDeveloperToken(token, secret)
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestValidateDeveloperToken_RejectsAdminToken(t *testing.T) {
	// Create an admin JWT token using the existing JWTManager
	manager := NewJWTManager("test-secret", 24*time.Hour, "https://test.togather.foundation")
	adminToken, err := manager.Generate("admin-user", "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	// Try to validate admin token as developer token - should fail
	_, err = ValidateDeveloperToken(adminToken, []byte("test-secret"))
	if err == nil {
		t.Error("Expected error when validating admin token as developer token")
	}
}
