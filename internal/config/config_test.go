package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_ProductionCORS_EmptyOrigins(t *testing.T) {
	// Setup
	originalEnv := map[string]string{
		"ENVIRONMENT":          os.Getenv("ENVIRONMENT"),
		"CORS_ALLOWED_ORIGINS": os.Getenv("CORS_ALLOWED_ORIGINS"),
		"DATABASE_URL":         os.Getenv("DATABASE_URL"),
		"JWT_SECRET":           os.Getenv("JWT_SECRET"),
	}
	defer func() {
		for k, v := range originalEnv {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	// Test case: production with empty CORS_ALLOWED_ORIGINS should fail
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("CORS_ALLOWED_ORIGINS", "")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error when CORS_ALLOWED_ORIGINS is empty in production, got nil")
	}
	if !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
		t.Errorf("Expected error message to mention CORS_ALLOWED_ORIGINS, got: %v", err)
	}
}

func TestLoad_ProductionCORS_ValidOrigins(t *testing.T) {
	// Setup
	originalEnv := map[string]string{
		"ENVIRONMENT":          os.Getenv("ENVIRONMENT"),
		"CORS_ALLOWED_ORIGINS": os.Getenv("CORS_ALLOWED_ORIGINS"),
		"DATABASE_URL":         os.Getenv("DATABASE_URL"),
		"JWT_SECRET":           os.Getenv("JWT_SECRET"),
	}
	defer func() {
		for k, v := range originalEnv {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	// Test case: production with valid CORS_ALLOWED_ORIGINS should succeed
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com,https://app.example.com")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error with valid CORS_ALLOWED_ORIGINS, got: %v", err)
	}
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 allowed origins, got %d", len(cfg.CORS.AllowedOrigins))
	}
	if cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be false in production")
	}
}

func TestLoad_DevelopmentCORS_AllowsAll(t *testing.T) {
	// Setup
	originalEnv := map[string]string{
		"ENVIRONMENT":          os.Getenv("ENVIRONMENT"),
		"CORS_ALLOWED_ORIGINS": os.Getenv("CORS_ALLOWED_ORIGINS"),
		"DATABASE_URL":         os.Getenv("DATABASE_URL"),
		"JWT_SECRET":           os.Getenv("JWT_SECRET"),
	}
	defer func() {
		for k, v := range originalEnv {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	// Test case: development should allow all origins regardless of CORS_ALLOWED_ORIGINS
	os.Setenv("ENVIRONMENT", "development")
	os.Setenv("CORS_ALLOWED_ORIGINS", "")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error in development, got: %v", err)
	}
	if !cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be true in development")
	}
}

func TestLoad_TestEnvironment_AllowsAll(t *testing.T) {
	// Setup
	originalEnv := map[string]string{
		"ENVIRONMENT":          os.Getenv("ENVIRONMENT"),
		"CORS_ALLOWED_ORIGINS": os.Getenv("CORS_ALLOWED_ORIGINS"),
		"DATABASE_URL":         os.Getenv("DATABASE_URL"),
		"JWT_SECRET":           os.Getenv("JWT_SECRET"),
	}
	defer func() {
		for k, v := range originalEnv {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	// Test case: test environment should allow all origins
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("CORS_ALLOWED_ORIGINS", "")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error in test environment, got: %v", err)
	}
	if !cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be true in test environment")
	}
}
