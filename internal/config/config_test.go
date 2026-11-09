package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_GeocodingBoundaryFallback(t *testing.T) {
	baseEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("ENVIRONMENT", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
		t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	}

	t.Run("defaults to disabled with a five-lookup budget", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_ENABLED", "")
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_MAX_LOOKUPS_PER_BATCH", "")
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_BUDGET_MS", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Geocoding.BoundaryFallbackEnabled {
			t.Error("BoundaryFallbackEnabled = true, want false by default")
		}
		if got := cfg.Geocoding.BoundaryFallbackMaxLookupsPerBatch; got != 5 {
			t.Errorf("BoundaryFallbackMaxLookupsPerBatch = %d, want 5", got)
		}
		if got := cfg.Geocoding.BoundaryFallbackBudgetMS; got != cfg.Scraper.PollTimeout/2 {
			t.Errorf("BoundaryFallbackBudgetMS = %d, want half the scraper poll window %d", got, cfg.Scraper.PollTimeout/2)
		}
		if got := cfg.Geocoding.BoundaryConflictRadiusKM; got != 50 {
			t.Errorf("BoundaryConflictRadiusKM = %v, want 50", got)
		}
	})

	t.Run("reads explicit env values", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_ENABLED", "true")
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_MAX_LOOKUPS_PER_BATCH", "9")
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_BUDGET_MS", "1234")
		t.Setenv("GEOCODING_BOUNDARY_CONFLICT_RADIUS_KM", "12.5")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !cfg.Geocoding.BoundaryFallbackEnabled {
			t.Error("BoundaryFallbackEnabled = false, want true")
		}
		if got := cfg.Geocoding.BoundaryFallbackMaxLookupsPerBatch; got != 9 {
			t.Errorf("BoundaryFallbackMaxLookupsPerBatch = %d, want 9", got)
		}
		if got := cfg.Geocoding.BoundaryFallbackBudgetMS; got != 1234 {
			t.Errorf("BoundaryFallbackBudgetMS = %d, want 1234", got)
		}
		if got := cfg.Geocoding.BoundaryConflictRadiusKM; got != 12.5 {
			t.Errorf("BoundaryConflictRadiusKM = %v, want 12.5", got)
		}
	})

	t.Run("adopts half the scraper poll window when the budget is unset", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("SCRAPER_POLL_TIMEOUT_MS", "45000")
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_BUDGET_MS", "0")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := cfg.Geocoding.BoundaryFallbackBudgetMS; got != 22500 {
			t.Errorf("BoundaryFallbackBudgetMS = %d, want half the poll window 22500", got)
		}
	})

	t.Run("negative budget keeps the wall-clock window disabled", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("GEOCODING_BOUNDARY_FALLBACK_BUDGET_MS", "-1")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := cfg.Geocoding.BoundaryFallbackBudgetMS; got != -1 {
			t.Errorf("BoundaryFallbackBudgetMS = %d, want -1 (no window)", got)
		}
	})
}

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

func TestLoad_ScraperPolling_NegativePastEventGrace(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":                    os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                   os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                     os.Getenv("JWT_SECRET"),
		"SCRAPER_PAST_EVENT_GRACE_HOURS": os.Getenv("SCRAPER_PAST_EVENT_GRACE_HOURS"),
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
	_ = os.Setenv("SCRAPER_PAST_EVENT_GRACE_HOURS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error for negative SCRAPER_PAST_EVENT_GRACE_HOURS, got nil")
	}
	if !strings.Contains(err.Error(), "SCRAPER_PAST_EVENT_GRACE_HOURS") {
		t.Errorf("Expected error to mention SCRAPER_PAST_EVENT_GRACE_HOURS, got: %v", err)
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

func TestGeographicBoundaryConfig_LoadsFromYAML(t *testing.T) {
	// Write a temporary YAML file
	tmpDir := t.TempDir()
	yamlPath := tmpDir + "/boundary.yaml"
	yamlContent := []byte("regions:\n  - Ontario\n  - Quebec\nlocalities:\n  - Toronto\n  - Montreal\n")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	boundary, err := loadGeographicBoundaryFile(yamlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boundary.Regions) != 2 || boundary.Regions[0] != "Ontario" || boundary.Regions[1] != "Quebec" {
		t.Errorf("expected regions [Ontario Quebec], got %v", boundary.Regions)
	}
	if len(boundary.Localities) != 2 || boundary.Localities[0] != "Toronto" || boundary.Localities[1] != "Montreal" {
		t.Errorf("expected localities [Toronto Montreal], got %v", boundary.Localities)
	}
}

func TestGeographicBoundaryConfig_ZeroWhenFileMissing(t *testing.T) {
	boundary, err := loadGeographicBoundaryFile("/nonexistent/path/boundary.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if len(boundary.Regions) != 0 || len(boundary.Localities) != 0 {
		t.Errorf("expected zero-valued config when file is missing, got %+v", boundary)
	}
}

func TestGeographicBoundaryConfig_MalformedYAMLReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := tmpDir + "/boundary.yaml"
	yamlContent := []byte("this: [ is: not: yaml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	boundary, err := loadGeographicBoundaryFile(yamlPath)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if len(boundary.Regions) != 0 || len(boundary.Localities) != 0 {
		t.Errorf("expected zero-valued config on malformed YAML, got %+v", boundary)
	}
}

func TestGeographicBoundaryConfig_ReadErrorReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	childDir := tmpDir + "/nodir"
	if err := os.Mkdir(childDir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(childDir, 0700); err != nil {
			t.Logf("failed to restore permissions on test dir: %v", err)
		}
	})

	yamlPath := childDir + "/boundary.yaml"
	boundary, err := loadGeographicBoundaryFile(yamlPath)
	if err == nil {
		t.Fatal("expected permission-denied error when directory is inaccessible")
	}
	if len(boundary.Regions) != 0 || len(boundary.Localities) != 0 {
		t.Errorf("expected zero-valued config on read error, got %+v", boundary)
	}
}

func TestLoad_BoundaryConfig_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := tmpDir + "/boundary.yaml"
	yamlContent := []byte("this: [ is: not: yaml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	originalEnv := map[string]string{
		"ENVIRONMENT":                     os.Getenv("ENVIRONMENT"),
		"DATABASE_URL":                    os.Getenv("DATABASE_URL"),
		"JWT_SECRET":                      os.Getenv("JWT_SECRET"),
		"GEOGRAPHIC_BOUNDARY_CONFIG_FILE": os.Getenv("GEOGRAPHIC_BOUNDARY_CONFIG_FILE"),
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
	_ = os.Setenv("GEOGRAPHIC_BOUNDARY_CONFIG_FILE", yamlPath)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for malformed boundary YAML, got nil")
	}
	if !strings.Contains(err.Error(), "geographic boundary") {
		t.Errorf("expected error to mention geographic boundary, got: %v", err)
	}
}

func TestAmbiguousDateMaxFutureDays_Default(t *testing.T) {
	v := ValidationConfig{}.WithDefaults()
	if v.AmbiguousDateMaxFutureDays != 60 {
		t.Errorf("AmbiguousDateMaxFutureDays default = %d, want 60", v.AmbiguousDateMaxFutureDays)
	}
}

func TestAmbiguousDateMaxFutureDays_EnvVar(t *testing.T) {
	originalEnv := map[string]string{
		"ENVIRONMENT":  os.Getenv("ENVIRONMENT"),
		"DATABASE_URL": os.Getenv("DATABASE_URL"),
		"JWT_SECRET":   os.Getenv("JWT_SECRET"),
		"VALIDATION_AMBIGUOUS_DATE_MAX_FUTURE_DAYS": os.Getenv("VALIDATION_AMBIGUOUS_DATE_MAX_FUTURE_DAYS"),
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
	_ = os.Setenv("VALIDATION_AMBIGUOUS_DATE_MAX_FUTURE_DAYS", "90")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if cfg.Validation.AmbiguousDateMaxFutureDays != 90 {
		t.Errorf("AmbiguousDateMaxFutureDays = %d, want 90", cfg.Validation.AmbiguousDateMaxFutureDays)
	}
}

func TestLoad_CORSPublicReadsOpen(t *testing.T) {
	for _, k := range []string{"ENVIRONMENT", "CORS_PUBLIC_READS_OPEN", "DATABASE_URL", "JWT_SECRET"} {
		old, had := os.LookupEnv(k)
		defer func(k, old string, had bool) {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		}(k, old, had)
	}
	_ = os.Setenv("ENVIRONMENT", "development")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	_ = os.Setenv("JWT_SECRET", "12345678901234567890123456789012")

	_ = os.Unsetenv("CORS_PUBLIC_READS_OPEN")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.CORS.PublicReadsOpen {
		t.Error("PublicReadsOpen should default to true")
	}

	_ = os.Setenv("CORS_PUBLIC_READS_OPEN", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CORS.PublicReadsOpen {
		t.Error("CORS_PUBLIC_READS_OPEN=false should disable public reads")
	}
}
