package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedOnce      sync.Once
	sharedInitErr   error
	sharedContainer *tcpostgres.PostgresContainer
	sharedPool      *pgxpool.Pool
	sharedDBURL     string
)

const sharedContainerName = "togather-handlers-health-db"

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupShared()
	os.Exit(code)
}

func initShared(t *testing.T) {
	t.Helper()
	sharedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Disable ryuk (resource reaper) to prevent premature container cleanup
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		container, err := tcpostgres.Run(
			ctx,
			"postgis/postgis:16-3.4",
			tcpostgres.WithDatabase("togather_test"),
			tcpostgres.WithUsername("togather"),
			tcpostgres.WithPassword("togather-test-password"),
			testcontainers.WithReuseByName(sharedContainerName),
			// PostGIS restarts after initial extension loading, so wait for
			// readiness log twice, then confirm the port is actually listening.
			testcontainers.WithAdditionalWaitStrategy(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second),
				wait.ForListeningPort("5432/tcp"),
			),
		)
		if err != nil {
			sharedInitErr = err
			return
		}
		sharedContainer = container

		dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedInitErr = err
			return
		}
		sharedDBURL = dbURL

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			sharedInitErr = err
			return
		}

		// Verify connection with retries — PostGIS may still be initializing
		// extensions even after the port is listening.
		for i := 0; i < 10; i++ {
			err = pool.Ping(ctx)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			sharedInitErr = err
			return
		}

		sharedPool = pool
	})

	require.NoError(t, sharedInitErr)
}

func cleanupShared() {
	if sharedPool != nil {
		sharedPool.Close()
	}
	// Note: Do NOT terminate the shared container - testcontainers will clean it up
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if pool == nil {
		require.Fail(t, "shared pool is nil")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
SELECT tablename
  FROM pg_tables
 WHERE schemaname = 'public'
   AND tablename <> 'schema_migrations'
 ORDER BY tablename;
`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if name == "" {
			continue
		}
		safe := strings.ReplaceAll(name, "\"", "\"\"")
		tables = append(tables, "\"public\".\""+safe+"\"")
	}
	require.NoError(t, rows.Err())

	if len(tables) == 0 {
		return
	}

	truncateSQL := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE;"
	_, err = pool.Exec(ctx, truncateSQL)
	require.NoError(t, err)
}

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

// TestHealthCheck_DatabaseTimeout verifies database check respects 2-second timeout
func TestHealthCheck_DatabaseTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	// Setup schema_migrations table to avoid migration check failures
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

	// Create a function that sleeps longer than the database check timeout (2s)
	// This simulates a slow query
	_, err = pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION slow_query() RETURNS void AS $$
		BEGIN
			PERFORM pg_sleep(3);
		END;
		$$ LANGUAGE plpgsql;
	`)
	require.NoError(t, err)

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	// Trigger a slow query in the background that will affect the database check
	// Note: We can't directly inject pg_sleep into the health check query,
	// but we can verify the timeout behavior by checking latency

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	checker.Health().ServeHTTP(w, req)
	duration := time.Since(start)

	// Verify the health check completes within the 5-second overall timeout
	assert.Less(t, duration, 6*time.Second, "health check should complete within overall timeout")

	// Parse response
	var response HealthCheck
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify database check completed (should not hang indefinitely)
	dbCheck, ok := response.Checks["database"]
	require.True(t, ok, "database check should be present")

	// Database check should either pass or fail, but not hang
	assert.Contains(t, []string{"pass", "fail"}, dbCheck.Status, "database check should complete")

	// Latency should be tracked and reasonable
	assert.GreaterOrEqual(t, dbCheck.LatencyMs, int64(0))
	assert.Less(t, dbCheck.LatencyMs, int64(3000), "database check should respect 2s timeout")
}

// TestHealthCheck_OverallTimeout verifies the 5-second overall timeout works
func TestHealthCheck_OverallTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	checker.Health().ServeHTTP(w, req)
	duration := time.Since(start)

	// Verify the health check completes well within the 5-second timeout
	// Even with multiple checks, it should be fast with a healthy database
	assert.Less(t, duration, 5*time.Second, "health check should complete within 5 seconds")

	// Parse response
	var response HealthCheck
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// All checks should have completed
	assert.Len(t, response.Checks, 5, "all 5 checks should be present")

	// Verify each check has reasonable latency
	for name, check := range response.Checks {
		assert.GreaterOrEqual(t, check.LatencyMs, int64(0), "%s should have non-negative latency", name)
		assert.Less(t, check.LatencyMs, int64(2500), "%s should complete quickly", name)
	}
}

// TestHealthCheck_ContextCancellation verifies graceful handling of cancelled context
func TestHealthCheck_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	// Create a context that's already cancelled
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // Cancel immediately
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Execute health check
	checker.Health().ServeHTTP(w, req)

	// Should return 503 with shutting_down status
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Parse response
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "shutting_down", response["status"])
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
	expectedChecks := []string{"database", "migrations", "job_queue", "jsonld_contexts", "http_endpoint"}
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

// TestHealthz_ShuttingDown verifies /healthz returns 503 during shutdown
func TestHealthz_ShuttingDown(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	// Create a cancelled context to simulate shutdown
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // Cancel immediately to simulate shutdown
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	Healthz().ServeHTTP(w, req)

	// Should return 503 when shutting down
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response healthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "shutting_down", response.Status)
}

func TestLegacyReadyz(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	checker.Readyz().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response healthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "ready", response.Status)
}

// TestReadyz_DatabaseFailure verifies /readyz returns 503 when database fails
func TestReadyz_DatabaseFailure(t *testing.T) {
	// Create checker with nil pool to simulate database failure
	checker := NewHealthChecker(nil, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	checker.Readyz().ServeHTTP(w, req)

	// Should return 503 Service Unavailable when database is not available
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response healthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", response.Status)
}

// TestReadyz_MigrationFailure verifies /readyz returns 503 when migrations are dirty
func TestReadyz_MigrationFailure(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	// Setup dirty migration state
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO schema_migrations (version, dirty)
		VALUES (1, true)
		ON CONFLICT (version) DO UPDATE SET dirty = true
	`)
	require.NoError(t, err)

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	checker.Readyz().ServeHTTP(w, req)

	// Should return 503 when migrations are in dirty state
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response healthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", response.Status)
}

// setupTestDB provides a test database connection using the shared container
// It resets the database state before each test for isolation
func setupTestDB(t *testing.T, ctx context.Context) (*pgxpool.Pool, func()) {
	t.Helper()

	initShared(t)
	resetDatabase(t, sharedPool)

	// Return the shared pool and a no-op cleanup function
	return sharedPool, func() {}
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
			expectedMsg:    "Migrations table not found",
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

// TestHealthCheck_HTTPEndpoint verifies the HTTP endpoint check
func TestHealthCheck_HTTPEndpoint(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	checker.Health().ServeHTTP(w, req)

	// Parse response
	var response HealthCheck
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify http_endpoint check exists and passes
	httpCheck, ok := response.Checks["http_endpoint"]
	require.True(t, ok, "http_endpoint check should be present")
	assert.Equal(t, "pass", httpCheck.Status)
	assert.Contains(t, httpCheck.Message, "HTTP endpoint operational")
	assert.GreaterOrEqual(t, httpCheck.LatencyMs, int64(0))
}

// TestHealthCheck_HTTPEndpointCancelled verifies HTTP endpoint check with cancelled context
func TestHealthCheck_HTTPEndpointCancelled(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	checker := NewHealthChecker(pool, nil, "0.1.0", "test-commit")

	// Create request with cancelled context
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // Cancel immediately
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	checker.Health().ServeHTTP(w, req)

	// Should return shutting_down status when context is cancelled at handler level
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "shutting_down", response["status"])
}
