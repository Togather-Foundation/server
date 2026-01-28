package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type healthPayload struct {
	Status string `json:"status"`
}

type comprehensiveHealthPayload struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	GitCommit string                 `json:"git_commit"`
	Checks    map[string]checkResult `json:"checks"`
	Timestamp string                 `json:"timestamp"`
}

type checkResult struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

func TestHealthz(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := env.Server.Client().Get(env.Server.URL + "/healthz")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload healthPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "ok", payload.Status)
}

func TestReadyz(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := env.Server.Client().Get(env.Server.URL + "/readyz")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload healthPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "ready", payload.Status)
}

// TestHealthComprehensive tests the comprehensive health check endpoint (T011)
func TestHealthComprehensive(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := env.Server.Client().Get(env.Server.URL + "/health")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var payload comprehensiveHealthPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	// Verify overall status
	require.Contains(t, []string{"healthy", "degraded"}, payload.Status)
	require.Equal(t, "test", payload.Version)
	require.Equal(t, "test-commit", payload.GitCommit)
	require.NotEmpty(t, payload.Timestamp)

	// Verify timestamp is valid RFC3339
	_, err = time.Parse(time.RFC3339, payload.Timestamp)
	require.NoError(t, err, "timestamp should be valid RFC3339")

	// Verify all required checks are present
	require.NotNil(t, payload.Checks)
	require.Contains(t, payload.Checks, "database")
	require.Contains(t, payload.Checks, "migrations")
	require.Contains(t, payload.Checks, "http_endpoint")
	require.Contains(t, payload.Checks, "job_queue")

	// Database check should pass
	dbCheck := payload.Checks["database"]
	require.Equal(t, "pass", dbCheck.Status)
	require.NotEmpty(t, dbCheck.Message)
	require.GreaterOrEqual(t, dbCheck.LatencyMs, int64(0))
	require.NotNil(t, dbCheck.Details)

	// Migrations check should pass
	migCheck := payload.Checks["migrations"]
	require.Contains(t, []string{"pass", "warn"}, migCheck.Status)
	require.NotEmpty(t, migCheck.Message)
	require.GreaterOrEqual(t, migCheck.LatencyMs, int64(0))

	// HTTP endpoint check should pass
	httpCheck := payload.Checks["http_endpoint"]
	require.Equal(t, "pass", httpCheck.Status)
	require.NotEmpty(t, httpCheck.Message)

	// Job queue check should pass or warn
	jobCheck := payload.Checks["job_queue"]
	require.Contains(t, []string{"pass", "warn"}, jobCheck.Status)
	require.NotEmpty(t, jobCheck.Message)
}
