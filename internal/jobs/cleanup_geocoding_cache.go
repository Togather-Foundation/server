package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// CleanupGeocodingCacheArgs defines the job for cleaning expired geocoding cache entries.
type CleanupGeocodingCacheArgs struct{}

func (CleanupGeocodingCacheArgs) Kind() string { return JobKindGeocodingCacheCleanup }

// CleanupGeocodingCacheWorker removes expired cache entries while preserving popular queries.
// Runs daily to prevent cache tables from growing indefinitely.
//
// Cleanup logic:
// 1. Delete expired forward geocoding cache entries (older than TTL)
// 2. Preserve top 10k most-hit entries past TTL (configurable via GEOCODING_POPULAR_PRESERVE_COUNT)
// 3. Delete expired reverse geocoding cache entries
// 4. Delete expired failure records
type CleanupGeocodingCacheWorker struct {
	river.WorkerDefaults[CleanupGeocodingCacheArgs]
	Pool             *pgxpool.Pool
	Logger           *slog.Logger
	Slot             string // Deployment slot (blue/green) for metrics labeling
	PreserveTopCount int    // Number of top queries to preserve (default: 10000)
}

func (CleanupGeocodingCacheWorker) Kind() string { return JobKindGeocodingCacheCleanup }

func (w CleanupGeocodingCacheWorker) Work(ctx context.Context, job *river.Job[CleanupGeocodingCacheArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	preserveCount := w.PreserveTopCount
	if preserveCount <= 0 {
		preserveCount = 10000 // Default
	}

	start := time.Now()

	logger.Info("starting geocoding cache cleanup job",
		"slot", w.Slot,
		"preserve_top_count", preserveCount,
		"attempt", job.Attempt,
	)

	// Track total rows affected across all cleanup operations
	var totalDeleted int64

	// 1. Clean up expired forward geocoding cache (preserving popular queries)
	forwardDeleted, err := w.cleanupForwardCache(ctx, preserveCount)
	if err != nil {
		logger.Error("failed to cleanup forward geocoding cache",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup forward cache: %w", err)
	}
	totalDeleted += forwardDeleted
	logger.Info("cleaned up forward geocoding cache",
		"slot", w.Slot,
		"deleted_count", forwardDeleted,
	)

	// 2. Clean up expired reverse geocoding cache (preserving popular queries)
	reverseDeleted, err := w.cleanupReverseCache(ctx, preserveCount)
	if err != nil {
		logger.Error("failed to cleanup reverse geocoding cache",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup reverse cache: %w", err)
	}
	totalDeleted += reverseDeleted
	logger.Info("cleaned up reverse geocoding cache",
		"slot", w.Slot,
		"deleted_count", reverseDeleted,
	)

	// 3. Clean up expired failure records
	failuresDeleted, err := w.cleanupFailures(ctx)
	if err != nil {
		logger.Error("failed to cleanup geocoding failures",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup failures: %w", err)
	}
	totalDeleted += failuresDeleted
	logger.Info("cleaned up geocoding failures",
		"slot", w.Slot,
		"deleted_count", failuresDeleted,
	)

	duration := time.Since(start).Seconds()

	logger.Info("geocoding cache cleanup job completed",
		"slot", w.Slot,
		"total_deleted", totalDeleted,
		"duration_seconds", duration,
	)

	return nil
}

// cleanupForwardCache deletes expired forward geocoding cache entries.
// Preserves top N most-hit entries past TTL to keep popular queries cached.
func (w CleanupGeocodingCacheWorker) cleanupForwardCache(ctx context.Context, preserveTopCount int) (int64, error) {
	const deleteQuery = `
		DELETE FROM geocoding_cache
		WHERE expires_at < NOW()
		AND id NOT IN (
			SELECT id FROM geocoding_cache
			ORDER BY hit_count DESC
			LIMIT $1
		)
	`

	result, err := w.Pool.Exec(ctx, deleteQuery, preserveTopCount)
	if err != nil {
		return 0, fmt.Errorf("delete expired forward cache: %w", err)
	}

	return result.RowsAffected(), nil
}

// cleanupReverseCache deletes expired reverse geocoding cache entries.
// Preserves top N most-hit entries past TTL to keep popular queries cached.
func (w CleanupGeocodingCacheWorker) cleanupReverseCache(ctx context.Context, preserveTopCount int) (int64, error) {
	const deleteQuery = `
		DELETE FROM reverse_geocoding_cache
		WHERE expires_at < NOW()
		AND id NOT IN (
			SELECT id FROM reverse_geocoding_cache
			ORDER BY hit_count DESC
			LIMIT $1
		)
	`

	result, err := w.Pool.Exec(ctx, deleteQuery, preserveTopCount)
	if err != nil {
		return 0, fmt.Errorf("delete expired reverse cache: %w", err)
	}

	return result.RowsAffected(), nil
}

// cleanupFailures deletes expired geocoding failure records.
func (w CleanupGeocodingCacheWorker) cleanupFailures(ctx context.Context) (int64, error) {
	const deleteQuery = `
		DELETE FROM geocoding_failures
		WHERE expires_at < NOW()
	`

	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return 0, fmt.Errorf("delete expired failures: %w", err)
	}

	return result.RowsAffected(), nil
}
