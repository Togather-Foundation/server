// Package testauth provides authentication utilities for testing and development.
// This package should NEVER be used in production code.
//
// It provides convenient ways to authenticate during:
// - Load testing
// - CLI operations
// - Integration tests
// - Local development
//
// Security: These utilities use well-known development secrets and should only
// be enabled when the server is running in development mode.
package testauth

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
)

// AuthMode determines how to authenticate requests.
type AuthMode string

const (
	// AuthModeJWT generates JWT tokens using dev secrets
	AuthModeJWT AuthMode = "jwt"
	// AuthModeAPIKey uses API keys from environment
	AuthModeAPIKey AuthMode = "apikey"
	// AuthModeNone disables authentication (for testing)
	AuthModeNone AuthMode = "none"
)

// TestAuthenticator provides methods to add authentication to HTTP requests
// for testing and development purposes.
type TestAuthenticator struct {
	mode      AuthMode
	jwtSecret string
	jwtIssuer string
	apiKey    string
}

// Config configures the test authenticator.
type Config struct {
	// Mode determines the authentication method
	Mode AuthMode

	// JWTSecret is the secret used to sign JWTs (for AuthModeJWT)
	// Defaults to DEV_JWT_SECRET env var or well-known dev secret
	JWTSecret string

	// JWTIssuer is the JWT issuer claim (optional)
	JWTIssuer string

	// APIKey is the API key to use (for AuthModeAPIKey)
	// Defaults to DEV_API_KEY env var
	APIKey string

	// Role is the role to assign when generating JWTs
	// Common values: "admin", "organizer", "contributor"
	Role string

	// Subject is the JWT subject (user ID or identifier)
	Subject string
}

// NewTestAuthenticator creates a new test authenticator with the given config.
func NewTestAuthenticator(cfg Config) (*TestAuthenticator, error) {
	// Apply defaults
	if cfg.Mode == "" {
		cfg.Mode = AuthModeJWT
	}

	switch cfg.Mode {
	case AuthModeJWT:
		secret := cfg.JWTSecret
		if secret == "" {
			secret = os.Getenv("DEV_JWT_SECRET")
		}
		if secret == "" {
			// Well-known dev secret (matches .env default)
			secret = "dev_jwt_secret_change_me_in_production"
		}

		issuer := cfg.JWTIssuer
		if issuer == "" {
			issuer = "togather-test"
		}

		role := cfg.Role
		if role == "" {
			role = "admin"
		}

		subject := cfg.Subject
		if subject == "" {
			subject = "test-user"
		}

		// Generate a JWT token
		jwtManager := auth.NewJWTManager(secret, 24*time.Hour, issuer)
		token, err := jwtManager.Generate(subject, role)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JWT: %w", err)
		}

		return &TestAuthenticator{
			mode:      cfg.Mode,
			jwtSecret: token, // Store the generated token in jwtSecret field
			jwtIssuer: issuer,
		}, nil

	case AuthModeAPIKey:
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("DEV_API_KEY")
		}
		if apiKey == "" {
			// Well-known dev API key (matches .env default)
			apiKey = "dev_admin_key_change_me_in_production"
		}

		return &TestAuthenticator{
			mode:   cfg.Mode,
			apiKey: apiKey,
		}, nil

	case AuthModeNone:
		return &TestAuthenticator{mode: cfg.Mode}, nil

	default:
		return nil, fmt.Errorf("unknown auth mode: %s", cfg.Mode)
	}
}

// AddAuth adds authentication headers to an HTTP request.
func (ta *TestAuthenticator) AddAuth(req *http.Request) {
	if req == nil {
		return
	}

	switch ta.mode {
	case AuthModeJWT:
		req.Header.Set("Authorization", "Bearer "+ta.jwtSecret)
	case AuthModeAPIKey:
		req.Header.Set("Authorization", "Bearer "+ta.apiKey)
	case AuthModeNone:
		// No authentication
	}
}

// GetAuthHeader returns the Authorization header value without modifying the request.
func (ta *TestAuthenticator) GetAuthHeader() string {
	switch ta.mode {
	case AuthModeJWT:
		return "Bearer " + ta.jwtSecret
	case AuthModeAPIKey:
		return "Bearer " + ta.apiKey
	case AuthModeNone:
		return ""
	default:
		return ""
	}
}

// NewDevAuthenticator creates a test authenticator using dev defaults.
// This is a convenience function for common testing scenarios.
func NewDevAuthenticator() (*TestAuthenticator, error) {
	return NewTestAuthenticator(Config{
		Mode:    AuthModeJWT,
		Role:    "admin",
		Subject: "test-user",
	})
}

// NewAPIKeyAuthenticator creates a test authenticator using an API key.
func NewAPIKeyAuthenticator(apiKey string) (*TestAuthenticator, error) {
	return NewTestAuthenticator(Config{
		Mode:   AuthModeAPIKey,
		APIKey: apiKey,
	})
}

// DevJWTToken generates a JWT token using dev secrets for ad-hoc testing.
// This is useful for CLI commands that need a quick token.
func DevJWTToken(role, subject string) (string, error) {
	if role == "" {
		role = "admin"
	}
	if subject == "" {
		subject = "test-user"
	}

	secret := os.Getenv("DEV_JWT_SECRET")
	if secret == "" {
		secret = "dev_jwt_secret_change_me_in_production"
	}

	jwtManager := auth.NewJWTManager(secret, 24*time.Hour, "togather-test")
	return jwtManager.Generate(subject, role)
}
