package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	Slot      string                 `json:"slot,omitempty"`
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
		// Check if server is shutting down (graceful shutdown in progress)
		select {
		case <-r.Context().Done():
			// Context cancelled - server is shutting down
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "shutting_down",
			})
			return
		default:
			// Continue with normal health check
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]CheckResult)

		// Run all checks
		checks["database"] = h.checkDatabase(ctx)
		checks["migrations"] = h.checkMigrations(ctx)
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

		// Get deployment slot identifier (for blue-green deployments)
		slot := os.Getenv("DEPLOYMENT_SLOT")
		if slot == "" {
			slot = os.Getenv("SLOT")
		}

		response := HealthCheck{
			Status:    overallStatus,
			Version:   h.version,
			GitCommit: h.gitCommit,
			Slot:      slot,
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
			Details: map[string]interface{}{
				"remediation": "Check that DATABASE_URL is set correctly and PostgreSQL is running",
			},
		}
	}

	// Create child context with per-operation timeout (2s)
	// This prevents one slow check from starving others
	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Test database connectivity with a simple query
	var result int
	err := h.pool.QueryRow(dbCtx, "SELECT 1").Scan(&result)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		// Provide operator-friendly error messages based on error type
		message := "Database query failed"
		details := map[string]interface{}{
			"error": err.Error(),
		}

		// Add specific remediation based on error type
		if ctx.Err() == context.DeadlineExceeded || dbCtx.Err() == context.DeadlineExceeded {
			message = "Database query timed out after 2 seconds"
			details["remediation"] = "Check PostgreSQL performance, network latency, or increase timeout"
		} else if strings.Contains(err.Error(), "connection refused") {
			message = "Database connection refused"
			details["remediation"] = "Verify PostgreSQL is running and DATABASE_URL host/port are correct"
			details["check"] = "Run: docker-compose ps postgres (or systemctl status postgresql)"
		} else if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "dial tcp") {
			message = "Cannot reach database host"
			details["remediation"] = "Check DATABASE_URL hostname and network connectivity"
		} else if strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), "password") {
			message = "Database authentication failed"
			details["remediation"] = "Verify DATABASE_URL username and password are correct"
		} else if strings.Contains(err.Error(), "database") && strings.Contains(err.Error(), "does not exist") {
			message = "Database does not exist"
			details["remediation"] = "Create database or check DATABASE_URL database name"
		} else {
			details["remediation"] = "Check DATABASE_URL environment variable and PostgreSQL service status"
		}

		return CheckResult{
			Status:    "fail",
			Message:   message,
			LatencyMs: latency,
			Details:   details,
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
			Details: map[string]interface{}{
				"remediation": "Check that DATABASE_URL is set correctly and PostgreSQL is running",
			},
		}
	}

	// Create child context with per-operation timeout (2s)
	migCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Query the schema_migrations table to get the latest migration version
	var version int64
	var dirty bool
	query := `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`
	err := h.pool.QueryRow(migCtx, query).Scan(&version, &dirty)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		message := "Failed to query migration version"
		details := map[string]interface{}{
			"error": err.Error(),
		}

		// Provide specific guidance based on error type
		if strings.Contains(err.Error(), "does not exist") {
			message = "Migrations table not found"
			details["remediation"] = "Run database migrations first: make migrate-up (or docker-compose exec server migrate up)"
		} else if strings.Contains(err.Error(), "connection") {
			details["remediation"] = "Database connection issue - check DATABASE_URL and PostgreSQL status"
		} else {
			details["remediation"] = "Verify migrations have been applied and schema_migrations table exists"
		}

		return CheckResult{
			Status:    "fail",
			Message:   message,
			LatencyMs: latency,
			Details:   details,
		}
	}

	if dirty {
		return CheckResult{
			Status:    "fail",
			Message:   "Database in dirty migration state - manual intervention required",
			LatencyMs: latency,
			Details: map[string]interface{}{
				"version":     version,
				"dirty":       dirty,
				"remediation": "Migration failed mid-transaction. See deploy/docs/migrations.md for recovery steps",
				"action":      "Do NOT run new migrations until this is resolved",
			},
		}
	}

	// Migration is healthy if not dirty
	// We don't check the exact version number since it changes with new migrations
	return CheckResult{
		Status:    "pass",
		Message:   fmt.Sprintf("Migrations applied successfully (version %d)", version),
		LatencyMs: latency,
		Details: map[string]interface{}{
			"version": version,
			"dirty":   false,
		},
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

	// Create child context with per-operation timeout (2s)
	jobCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// First check if the river_job table exists
	// This distinguishes between "River not initialized" (warn) vs "query failed" (fail)
	var tableExists bool
	tableCheckQuery := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'river_job'
		)
	`
	err := h.pool.QueryRow(jobCtx, tableCheckQuery).Scan(&tableExists)
	if err != nil {
		latency := time.Since(start).Milliseconds()
		return CheckResult{
			Status:    "fail",
			Message:   "Failed to check job queue table existence",
			LatencyMs: latency,
			Details: map[string]interface{}{
				"error":       err.Error(),
				"remediation": "Database connection issue - check DATABASE_URL and PostgreSQL status",
			},
		}
	}

	if !tableExists {
		latency := time.Since(start).Milliseconds()
		return CheckResult{
			Status:    "warn",
			Message:   "River job queue table not found",
			LatencyMs: latency,
			Details: map[string]interface{}{
				"remediation": "Run River migrations to create river_job table",
				"note":        "Job queue is optional during early development",
			},
		}
	}

	// Query River's jobs table to verify it's accessible
	query := `SELECT COUNT(*) FROM river_job WHERE state = ANY($1)`
	var activeJobs int64
	err = h.pool.QueryRow(jobCtx, query, []string{"available", "running"}).Scan(&activeJobs)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		message := "Failed to query job queue"
		details := map[string]interface{}{
			"error": err.Error(),
		}

		// Add specific guidance
		if strings.Contains(err.Error(), "syntax error") {
			details["remediation"] = "River table schema mismatch - check migration version"
		} else {
			details["remediation"] = "Check database connectivity and river_job table permissions"
		}

		return CheckResult{
			Status:    "fail",
			Message:   message,
			LatencyMs: latency,
			Details:   details,
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
