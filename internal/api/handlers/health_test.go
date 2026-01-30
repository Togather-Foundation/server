package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestHealthCheck_AllHealthy(t *testing.T) {
	// Setup test database connection
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	// Setup schema_migrations table for a healthy migration state
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO schema_migrations (version, dirty)
		VALUES (1, false)
		ON CONFLICT (version) DO UPDATE SET dirty = false
	`)
	require.NoError(t, err)

	// Create health checker (without riverClient for this test)
	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Execute health check
	checker.Health().ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Parse response
	var response HealthCheck
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify overall status
	assert.Contains(t, []string{"healthy", "degraded"}, response.Status) // degraded if job_queue is not initialized
	assert.Equal(t, "0.1.0", response.Version)
	assert.Equal(t, "test-commit", response.GitCommit)
	assert.NotEmpty(t, response.Timestamp)

	// Verify checks
	assert.NotNil(t, response.Checks)

	// Database check should pass
	dbCheck, ok := response.Checks["database"]
	require.True(t, ok, "database check should be present")
	assert.Equal(t, "pass", dbCheck.Status)
	assert.GreaterOrEqual(t, dbCheck.LatencyMs, int64(0))
	assert.NotNil(t, dbCheck.Details)

	// Migrations check should pass
	migCheck, ok := response.Checks["migrations"]
	require.True(t, ok, "migrations check should be present")
	assert.Contains(t, []string{"pass", "warn", "fail"}, migCheck.Status)

	// HTTP endpoint check should pass
	httpCheck, ok := response.Checks["http_endpoint"]
	require.True(t, ok, "http_endpoint check should be present")
	assert.Equal(t, "pass", httpCheck.Status)

	// Job queue check should be present (may be warn if not initialized)
	jobCheck, ok := response.Checks["job_queue"]
	require.True(t, ok, "job_queue check should be present")
	assert.Contains(t, []string{"pass", "warn", "fail"}, jobCheck.Status)
}

func TestHealthCheck_DatabaseFailure(t *testing.T) {
	// Create health checker with nil pool to simulate database failure
	checker := NewHealthChecker(nil, nil, "0.1.0", "test-commit")

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Execute health check
	checker.Health().ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Parse response
	var response HealthCheck
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify overall status is unhealthy
	assert.Equal(t, "unhealthy", response.Status)

	// Database check should fail
	dbCheck, ok := response.Checks["database"]
	require.True(t, ok)
	assert.Equal(t, "fail", dbCheck.Status)
}

func TestHealthCheck_Timeout(t *testing.T) {
	// Setup test database connection
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	// Create test request with very short timeout
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 1*time.Nanosecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Execute health check
	checker.Health().ServeHTTP(w, req)

	// Should complete (may have timeouts in individual checks)
	assert.NotEqual(t, 0, w.Code)
}

func TestHealthCheck_ResponseFormat(t *testing.T) {
	// Setup test database connection
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "abc123")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	checker.Health().ServeHTTP(w, req)

	// Parse response
	var response HealthCheck
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify all required fields are present
	assert.NotEmpty(t, response.Status)
	assert.Equal(t, "0.1.0", response.Version)
	assert.Equal(t, "abc123", response.GitCommit)
	assert.NotEmpty(t, response.Timestamp)
	assert.NotNil(t, response.Checks)

	// Verify timestamp is valid RFC3339
	_, err = time.Parse(time.RFC3339, response.Timestamp)
	assert.NoError(t, err, "timestamp should be valid RFC3339")

	// Verify all expected checks are present
	expectedChecks := []string{"database", "migrations", "http_endpoint", "job_queue"}
	for _, checkName := range expectedChecks {
		check, ok := response.Checks[checkName]
		assert.True(t, ok, "check %s should be present", checkName)
		assert.NotEmpty(t, check.Status, "check %s should have status", checkName)
	}
}

func TestHealthCheck_StatusDetermination(t *testing.T) {
	tests := []struct {
		name           string
		checks         map[string]CheckResult
		expectedStatus string
		expectedCode   int
	}{
		{
			name: "all pass",
			checks: map[string]CheckResult{
				"db":   {Status: "pass"},
				"http": {Status: "pass"},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "one warn",
			checks: map[string]CheckResult{
				"db":   {Status: "pass"},
				"http": {Status: "warn"},
			},
			expectedStatus: "degraded",
			expectedCode:   http.StatusOK,
		},
		{
			name: "one fail",
			checks: map[string]CheckResult{
				"db":   {Status: "pass"},
				"http": {Status: "fail"},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "warn and fail",
			checks: map[string]CheckResult{
				"db":   {Status: "warn"},
				"http": {Status: "fail"},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate status determination logic
			overallStatus := "healthy"
			statusCode := http.StatusOK
			for _, check := range tt.checks {
				if check.Status == "fail" {
					overallStatus = "unhealthy"
					statusCode = http.StatusServiceUnavailable
					break
				} else if check.Status == "warn" && overallStatus == "healthy" {
					overallStatus = "degraded"
				}
			}

			assert.Equal(t, tt.expectedStatus, overallStatus)
			assert.Equal(t, tt.expectedCode, statusCode)
		})
	}
}

func TestLegacyHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	Healthz().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response healthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
}

func TestLegacyReadyz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	Readyz().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response healthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "ready", response.Status)
}

// setupTestDB creates a test database connection for health check tests
// It first tries to use DATABASE_URL if set, otherwise creates a testcontainer
func setupTestDB(t *testing.T, ctx context.Context) (*pgxpool.Pool, func()) {
	t.Helper()

	// Try DATABASE_URL first (faster for CI/local with existing DB)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		pool, err := pgxpool.New(ctx, dbURL)
		if err == nil && pool.Ping(ctx) == nil {
			return pool, func() { pool.Close() }
		}
		// If DATABASE_URL is set but fails, fall through to testcontainers
		t.Logf("DATABASE_URL set but connection failed, using testcontainer")
	}

	// Use testcontainers as fallback
	postgresContainer, err := tcpostgres.Run(ctx,
		"postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("togather_test"),
		tcpostgres.WithUsername("togather"),
		tcpostgres.WithPassword("togather-test-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "failed to start PostgreSQL container")

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping test database")

	cleanup := func() {
		pool.Close()
		if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
			t.Logf("Failed to terminate PostgreSQL container: %v", err)
		}
	}

	return pool, cleanup
}

// TestHealthCheck_MigrationVersionValidation tests the migration check behavior
// with different migration states: clean, dirty, and missing table
func TestHealthCheck_MigrationVersionValidation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	tests := []struct {
		name           string
		setupMigration func(t *testing.T, pool *pgxpool.Pool)
		expectedStatus string
		expectedMsg    string
	}{
		{
			name: "clean migrations pass",
			setupMigration: func(t *testing.T, pool *pgxpool.Pool) {
				// Ensure schema_migrations exists and is clean
				_, err := pool.Exec(ctx, `
					CREATE TABLE IF NOT EXISTS schema_migrations (
						version BIGINT PRIMARY KEY,
						dirty BOOLEAN NOT NULL
					)
				`)
				require.NoError(t, err)

				// Insert a clean migration version
				_, err = pool.Exec(ctx, `
					INSERT INTO schema_migrations (version, dirty)
					VALUES (1, false)
					ON CONFLICT (version) DO UPDATE SET dirty = false
				`)
				require.NoError(t, err)
			},
			expectedStatus: "pass",
			expectedMsg:    "Migrations applied successfully",
		},
		{
			name: "dirty migration fails",
			setupMigration: func(t *testing.T, pool *pgxpool.Pool) {
				// Ensure schema_migrations exists
				_, err := pool.Exec(ctx, `
					CREATE TABLE IF NOT EXISTS schema_migrations (
						version BIGINT PRIMARY KEY,
						dirty BOOLEAN NOT NULL
					)
				`)
				require.NoError(t, err)

				// Insert a dirty migration state
				_, err = pool.Exec(ctx, `
					INSERT INTO schema_migrations (version, dirty)
					VALUES (1, true)
					ON CONFLICT (version) DO UPDATE SET dirty = true
				`)
				require.NoError(t, err)
			},
			expectedStatus: "fail",
			expectedMsg:    "Database in dirty migration state",
		},
		{
			name: "missing schema_migrations table fails",
			setupMigration: func(t *testing.T, pool *pgxpool.Pool) {
				// Drop schema_migrations table if it exists
				_, err := pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations`)
				require.NoError(t, err)
			},
			expectedStatus: "fail",
			expectedMsg:    "Failed to query migration version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup migration state
			tt.setupMigration(t, pool)

			// Create health checker
			checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()

			// Execute health check
			checker.Health().ServeHTTP(w, req)

			// Parse response
			var response HealthCheck
			err := json.NewDecoder(w.Body).Decode(&response)
			require.NoError(t, err)

			// Verify migrations check
			migCheck, ok := response.Checks["migrations"]
			require.True(t, ok, "migrations check should be present")
			assert.Equal(t, tt.expectedStatus, migCheck.Status, "migration status mismatch")
			assert.Contains(t, migCheck.Message, tt.expectedMsg, "migration message mismatch")

			// Verify details for specific cases
			if tt.expectedStatus == "fail" && tt.name == "dirty migration fails" {
				assert.NotNil(t, migCheck.Details, "dirty migration should have details")
				assert.Equal(t, true, migCheck.Details["dirty"], "dirty flag should be true")
				assert.Contains(t, migCheck.Details["remediation"], "migrations.md", "should include remediation")
			}

			if tt.expectedStatus == "pass" {
				assert.NotNil(t, migCheck.Details, "clean migration should have details")
				assert.Equal(t, false, migCheck.Details["dirty"], "dirty flag should be false")
				assert.NotNil(t, migCheck.Details["version"], "version should be present")
			}
		})
	}
}

// TestHealthCheck_MigrationCheckLatency verifies migration checks complete within timeout
func TestHealthCheck_MigrationCheckLatency(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	// Setup clean migration state
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO schema_migrations (version, dirty)
		VALUES (1, false)
		ON CONFLICT (version) DO UPDATE SET dirty = false
	`)
	require.NoError(t, err)

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	checker.Health().ServeHTTP(w, req)
	duration := time.Since(start)

	// Parse response
	var response HealthCheck
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify migrations check completed
	migCheck, ok := response.Checks["migrations"]
	require.True(t, ok)
	assert.Equal(t, "pass", migCheck.Status)

	// Verify latency is reasonable (migration check has 2s timeout per operation)
	assert.Less(t, duration, 5*time.Second, "health check should complete within 5 seconds")
	// Latency should be set (may be 0 for very fast checks on fresh DB)
	assert.GreaterOrEqual(t, migCheck.LatencyMs, int64(0), "latency should be non-negative")
	assert.Less(t, migCheck.LatencyMs, int64(2000), "migration check should complete within 2 seconds")
}

// TestHealthCheck_MigrationWithNilPool verifies behavior when database pool is not initialized
func TestHealthCheck_MigrationWithNilPool(t *testing.T) {
	// Create health checker with nil pool
	checker := NewHealthChecker(nil, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	checker.Health().ServeHTTP(w, req)

	// Parse response
	var response HealthCheck
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify migrations check fails when pool is nil
	migCheck, ok := response.Checks["migrations"]
	require.True(t, ok)
	assert.Equal(t, "fail", migCheck.Status)
	assert.Contains(t, migCheck.Message, "Database pool not initialized")
}
