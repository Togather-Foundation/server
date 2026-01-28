package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// HealthCheck represents the health status of the server
type HealthCheck struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	GitCommit string                 `json:"git_commit"`
	Checks    map[string]CheckResult `json:"checks"`
	Timestamp string                 `json:"timestamp"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthChecker provides comprehensive health checks for the server
type HealthChecker struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
	version     string
	gitCommit   string
}

// NewHealthChecker creates a new health checker with the given dependencies
func NewHealthChecker(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx], version, gitCommit string) *HealthChecker {
	return &HealthChecker{
		pool:        pool,
		riverClient: riverClient,
		version:     version,
		gitCommit:   gitCommit,
	}
}

// Health returns a comprehensive health check handler
func (h *HealthChecker) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]CheckResult)

		// Run all checks
		checks["database"] = h.checkDatabase(ctx)
		checks["migrations"] = h.checkMigrations(ctx)
		checks["http_endpoint"] = h.checkHTTPEndpoint()
		checks["job_queue"] = h.checkJobQueue(ctx)

		// Determine overall status
		overallStatus := "healthy"
		statusCode := http.StatusOK
		for _, check := range checks {
			if check.Status == "fail" {
				overallStatus = "unhealthy"
				statusCode = http.StatusServiceUnavailable
				break
			} else if check.Status == "warn" && overallStatus == "healthy" {
				overallStatus = "degraded"
			}
		}

		response := HealthCheck{
			Status:    overallStatus,
			Version:   h.version,
			GitCommit: h.gitCommit,
			Checks:    checks,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	}
}

// checkDatabase verifies PostgreSQL connection and query execution
func (h *HealthChecker) checkDatabase(ctx context.Context) CheckResult {
	start := time.Now()

	if h.pool == nil {
		return CheckResult{
			Status:  "fail",
			Message: "Database pool not initialized",
		}
	}

	// Test database connectivity with a simple query
	var result int
	err := h.pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    "fail",
			Message:   fmt.Sprintf("Database query failed: %v", err),
			LatencyMs: latency,
		}
	}

	// Get pool statistics
	stats := h.pool.Stat()
	details := map[string]interface{}{
		"max_connections":      stats.MaxConns(),
		"total_connections":    stats.TotalConns(),
		"idle_connections":     stats.IdleConns(),
		"acquired_connections": stats.AcquiredConns(),
	}

	return CheckResult{
		Status:    "pass",
		Message:   "PostgreSQL connection successful",
		LatencyMs: latency,
		Details:   details,
	}
}

// checkMigrations verifies migration version matches expected
func (h *HealthChecker) checkMigrations(ctx context.Context) CheckResult {
	start := time.Now()

	if h.pool == nil {
		return CheckResult{
			Status:  "fail",
			Message: "Database pool not initialized",
		}
	}

	// Query the schema_migrations table to get the latest migration version
	var version int64
	var dirty bool
	query := `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`
	err := h.pool.QueryRow(ctx, query).Scan(&version, &dirty)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    "fail",
			Message:   fmt.Sprintf("Failed to query migration version: %v", err),
			LatencyMs: latency,
		}
	}

	if dirty {
		return CheckResult{
			Status:    "fail",
			Message:   fmt.Sprintf("Migration version %d is in dirty state", version),
			LatencyMs: latency,
			Details: map[string]interface{}{
				"version": version,
				"dirty":   dirty,
			},
		}
	}

	// Expected version is 15 (latest migration: 000015_batch_ingestion_results)
	expectedVersion := int64(15)
	if version != expectedVersion {
		return CheckResult{
			Status:    "warn",
			Message:   fmt.Sprintf("Migration version mismatch: expected %d, got %d", expectedVersion, version),
			LatencyMs: latency,
			Details: map[string]interface{}{
				"expected_version": expectedVersion,
				"actual_version":   version,
			},
		}
	}

	return CheckResult{
		Status:    "pass",
		Message:   fmt.Sprintf("Schema version matches expected: %d", version),
		LatencyMs: latency,
		Details: map[string]interface{}{
			"version": version,
		},
	}
}

// checkHTTPEndpoint performs a self-check that HTTP server is responding
func (h *HealthChecker) checkHTTPEndpoint() CheckResult {
	// This check always passes since if we reached this handler, the HTTP server is responding
	return CheckResult{
		Status:  "pass",
		Message: "HTTP endpoint responding",
	}
}

// checkJobQueue verifies River job queue is operational
func (h *HealthChecker) checkJobQueue(ctx context.Context) CheckResult {
	start := time.Now()

	if h.riverClient == nil {
		// Job queue is optional during early development
		return CheckResult{
			Status:  "warn",
			Message: "Job queue not initialized (optional)",
		}
	}

	// Query River's jobs table to verify it's accessible
	// We'll check if we can query the river_job table
	query := `SELECT COUNT(*) FROM river_job WHERE state = 'available' OR state = 'running'`
	var activeJobs int64
	err := h.pool.QueryRow(ctx, query).Scan(&activeJobs)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    "fail",
			Message:   fmt.Sprintf("Failed to query job queue: %v", err),
			LatencyMs: latency,
		}
	}

	return CheckResult{
		Status:    "pass",
		Message:   "River job queue operational",
		LatencyMs: latency,
		Details: map[string]interface{}{
			"active_jobs": activeJobs,
		},
	}
}

// Healthz returns a lightweight liveness response (legacy, kept for compatibility)
func Healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondHealth(w, http.StatusOK, "ok")
	})
}

// Readyz returns a readiness response (legacy, kept for compatibility)
func Readyz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondHealth(w, http.StatusOK, "ready")
	})
}

type healthResponse struct {
	Status string `json:"status"`
}

func respondHealth(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: value})
}
