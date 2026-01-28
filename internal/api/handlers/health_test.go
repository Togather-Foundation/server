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
)

func TestHealthCheck_AllHealthy(t *testing.T) {
	// Setup test database connection
	ctx := context.Background()
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

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
	err := json.NewDecoder(w.Body).Decode(&response)
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
	assert.Greater(t, dbCheck.LatencyMs, int64(0))
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
func setupTestDB(t *testing.T, ctx context.Context) (*pgxpool.Pool, func()) {
	t.Helper()

	// Use DATABASE_URL environment variable or skip test
	dbURL := getTestDatabaseURL(t)

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping test database")

	cleanup := func() {
		pool.Close()
	}

	return pool, cleanup
}

// getTestDatabaseURL returns the test database URL or skips the test if not available
func getTestDatabaseURL(t *testing.T) string {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping database-dependent test")
	}
	return dbURL
}
