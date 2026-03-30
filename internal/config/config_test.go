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
	_ = os.Setenv("ENVIRONMENT", "production")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

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
	_ = os.Setenv("ENVIRONMENT", "production")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com,https://app.example.com")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

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
	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

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
	_ = os.Setenv("ENVIRONMENT", "test")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error in test environment, got: %v", err)
	}
	if !cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be true in test environment")
	}
}

func TestLoad_ProductionCORS_WildcardAllowsAll(t *testing.T) {
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

	// Test case: production with CORS_ALLOWED_ORIGINS='*' should allow all origins
	_ = os.Setenv("ENVIRONMENT", "production")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error with wildcard CORS_ALLOWED_ORIGINS, got: %v", err)
	}
	if !cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be true with wildcard '*'")
	}
	if cfg.CORS.AllowedOrigins != nil {
		t.Error("Expected AllowedOrigins to be nil with wildcard '*'")
	}
}

func TestLoad_StagingCORS_WildcardAllowsAll(t *testing.T) {
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

	// Test case: staging with CORS_ALLOWED_ORIGINS='*' should allow all origins
	_ = os.Setenv("ENVIRONMENT", "staging")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error with wildcard CORS_ALLOWED_ORIGINS in staging, got: %v", err)
	}
	if !cfg.CORS.AllowAllOrigins {
		t.Error("Expected AllowAllOrigins to be true in staging with wildcard '*'")
	}
	if cfg.CORS.AllowedOrigins != nil {
		t.Error("Expected AllowedOrigins to be nil in staging with wildcard '*'")
	}
}

func TestLoad_ScraperPolling_InvalidBackoffStart(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                   os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                  os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                    os.Getenv("JWT_SECRET"),
		"SCRAPER_POLL_BACKOFF_START_MS": os.Getenv("SCRAPER_POLL_BACKOFF_START_MS"),
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

	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_START_MS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error for invalid SCRAPER_POLL_BACKOFF_START_MS, got nil")
	}
	if !strings.Contains(err.Error(), "SCRAPER_POLL_BACKOFF_START_MS") {
		t.Errorf("Expected error to mention SCRAPER_POLL_BACKOFF_START_MS, got: %v", err)
	}
}

func TestLoad_ScraperPolling_InvalidBackoffMax(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                 os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                  os.Getenv("JWT_SECRET"),
		"SCRAPER_POLL_BACKOFF_MAX_MS": os.Getenv("SCRAPER_POLL_BACKOFF_MAX_MS"),
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

	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_MAX_MS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error for invalid SCRAPER_POLL_BACKOFF_MAX_MS, got nil")
	}
	if !strings.Contains(err.Error(), "SCRAPER_POLL_BACKOFF_MAX_MS") {
		t.Errorf("Expected error to mention SCRAPER_POLL_BACKOFF_MAX_MS, got: %v", err)
	}
}

func TestLoad_ScraperPolling_BackoffMaxLessThanStart(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                   os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                  os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                    os.Getenv("JWT_SECRET"),
		"SCRAPER_POLL_BACKOFF_START_MS": os.Getenv("SCRAPER_POLL_BACKOFF_START_MS"),
		"SCRAPER_POLL_BACKOFF_MAX_MS":   os.Getenv("SCRAPER_POLL_BACKOFF_MAX_MS"),
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

	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_START_MS", "500")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_MAX_MS", "100")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error when SCRAPER_POLL_BACKOFF_MAX_MS < SCRAPER_POLL_BACKOFF_START_MS, got nil")
	}
	if !strings.Contains(err.Error(), "SCRAPER_POLL_BACKOFF_MAX_MS") {
		t.Errorf("Expected error to mention SCRAPER_POLL_BACKOFF_MAX_MS, got: %v", err)
	}
}

func TestLoad_ScraperPolling_InvalidHTTPClientTimeout(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                    os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                   os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                     os.Getenv("JWT_SECRET"),
		"SCRAPER_HTTP_CLIENT_TIMEOUT_MS": os.Getenv("SCRAPER_HTTP_CLIENT_TIMEOUT_MS"),
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

	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	_ = os.Setenv("SCRAPER_HTTP_CLIENT_TIMEOUT_MS", "-5000")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error for invalid SCRAPER_HTTP_CLIENT_TIMEOUT_MS, got nil")
	}
	if !strings.Contains(err.Error(), "SCRAPER_HTTP_CLIENT_TIMEOUT_MS") {
		t.Errorf("Expected error to mention SCRAPER_HTTP_CLIENT_TIMEOUT_MS, got: %v", err)
	}
}

func TestLoad_ScraperPolling_ValidConfig(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                    os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                   os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                     os.Getenv("JWT_SECRET"),
		"SCRAPER_POLL_BACKOFF_START_MS":  os.Getenv("SCRAPER_POLL_BACKOFF_START_MS"),
		"SCRAPER_POLL_BACKOFF_MAX_MS":    os.Getenv("SCRAPER_POLL_BACKOFF_MAX_MS"),
		"SCRAPER_POLL_TIMEOUT_MS":        os.Getenv("SCRAPER_POLL_TIMEOUT_MS"),
		"SCRAPER_HTTP_CLIENT_TIMEOUT_MS": os.Getenv("SCRAPER_HTTP_CLIENT_TIMEOUT_MS"),
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

	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_START_MS", "100")
	_ = os.Setenv("SCRAPER_POLL_BACKOFF_MAX_MS", "5000")
	_ = os.Setenv("SCRAPER_POLL_TIMEOUT_MS", "60000")
	_ = os.Setenv("SCRAPER_HTTP_CLIENT_TIMEOUT_MS", "15000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error with valid polling config, got: %v", err)
	}
	if cfg.Scraper.PollBackoffStart != 100 {
		t.Errorf("PollBackoffStart = %d, want 100", cfg.Scraper.PollBackoffStart)
	}
	if cfg.Scraper.PollBackoffMax != 5000 {
		t.Errorf("PollBackoffMax = %d, want 5000", cfg.Scraper.PollBackoffMax)
	}
}
