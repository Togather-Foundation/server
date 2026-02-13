package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

const (
	// JobKindUsageRollup identifies the daily usage rollup job
	JobKindUsageRollup = "usage_rollup"

	// DefaultUsageRetentionDays defines how long to keep detailed daily usage data
	// Older records are cleaned up to prevent table growth
	DefaultUsageRetentionDays = 90
)

// UsageRollupArgs defines the job for daily usage data reconciliation and rollup.
type UsageRollupArgs struct {
	// RetentionDays specifies how many days of detailed usage to retain (default: 90)
	RetentionDays int `json:"retention_days,omitempty"`
}

func (UsageRollupArgs) Kind() string { return JobKindUsageRollup }

// UsageRollupWorker performs daily usage data maintenance:
// 1. Reconciles usage data to ensure consistency (idempotent)
// 2. Cleans up old detailed usage data beyond retention period
// 3. Optionally computes rolling aggregates for dashboard queries (future enhancement)
//
// The job is safe to run multiple times per day - all operations are idempotent.
type UsageRollupWorker struct {
	river.WorkerDefaults[UsageRollupArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	Slot   string // Deployment slot (blue/green) for metrics labeling
}

func (UsageRollupWorker) Kind() string { return JobKindUsageRollup }

func (w UsageRollupWorker) Work(ctx context.Context, job *river.Job[UsageRollupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	start := time.Now()
	retentionDays := job.Args.RetentionDays
	if retentionDays <= 0 {
		retentionDays = DefaultUsageRetentionDays
	}

	logger.Info("starting usage rollup job",
		"slot", w.Slot,
		"attempt", job.Attempt,
		"retention_days", retentionDays,
	)

	// 1. Reconcile usage data (ensures consistency, no-op if data is already correct)
	reconciledRows, err := w.reconcileUsageData(ctx)
	if err != nil {
		logger.Error("failed to reconcile usage data",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("reconcile usage data: %w", err)
	}
	if reconciledRows > 0 {
		logger.Info("reconciled usage data",
			"slot", w.Slot,
			"reconciled_rows", reconciledRows,
		)
	}

	// 2. Clean up old usage data beyond retention period
	deletedRows, err := w.cleanupOldUsageData(ctx, retentionDays)
	if err != nil {
		logger.Error("failed to cleanup old usage data",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup old usage data: %w", err)
	}
	if deletedRows > 0 {
		logger.Info("cleaned up old usage data",
			"slot", w.Slot,
			"deleted_rows", deletedRows,
			"retention_days", retentionDays,
		)
	}

	// 3. Future enhancement: compute rolling aggregates (7-day, 30-day)
	// This would involve creating a separate api_key_usage_summary table
	// and computing aggregates for fast dashboard queries.
	// For now, we use the existing queries that aggregate on-demand.

	duration := time.Since(start).Seconds()

	logger.Info("usage rollup job completed",
		"slot", w.Slot,
		"reconciled_rows", reconciledRows,
		"deleted_rows", deletedRows,
		"duration_seconds", duration,
	)

	return nil
}

// reconcileUsageData ensures usage data consistency.
// This is primarily a safety mechanism in case the in-memory flush fails.
// Returns the number of rows that were updated (0 if already consistent).
//
// Currently, the usage recorder flushes every 30 seconds, so missed flushes
// are rare. This reconciliation is defensive programming.
//
// Future enhancement: Could check for gaps in dates or other inconsistencies.
func (w UsageRollupWorker) reconcileUsageData(ctx context.Context) (int64, error) {
	// For now, reconciliation is a no-op since the usage recorder's UpsertAPIKeyUsage
	// is already idempotent and handles incremental updates correctly.
	//
	// If we detect missed flushes in production, we could add logic here to:
	// 1. Query for gaps in usage data (missing dates for active API keys)
	// 2. Reprocess data from a backup source if available
	// 3. Send alerts for data inconsistencies
	//
	// The current implementation relies on:
	// - UsageRecorder's periodic flush (every 30s)
	// - UsageRecorder's shutdown flush (on server stop)
	// - MaxBufferSize flush trigger (at 100 keys)

	// Return 0 to indicate no rows were reconciled (data is consistent)
	return 0, nil
}

// cleanupOldUsageData deletes usage records older than the retention period.
// This prevents the api_key_usage table from growing indefinitely.
// Returns the number of rows deleted.
func (w UsageRollupWorker) cleanupOldUsageData(ctx context.Context, retentionDays int) (int64, error) {
	// Delete usage data older than retention period
	// Use parameterized interval to prevent SQL injection
	const deleteQuery = `
		DELETE FROM api_key_usage 
		WHERE date < CURRENT_DATE - $1::interval
	`

	// Format interval as PostgreSQL interval string
	interval := fmt.Sprintf("%d days", retentionDays)

	result, err := w.Pool.Exec(ctx, deleteQuery, interval)
	if err != nil {
		return 0, fmt.Errorf("delete old usage data: %w", err)
	}

	return result.RowsAffected(), nil
}
