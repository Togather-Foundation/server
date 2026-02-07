package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// ReviewQueueCleanupArgs defines the job for cleaning expired review queue entries.
type ReviewQueueCleanupArgs struct{}

func (ReviewQueueCleanupArgs) Kind() string { return JobKindReviewQueueCleanup }

// ReviewQueueCleanupWorker removes expired review queue entries and marks unreviewed events as deleted.
// Runs daily to prevent review queue from growing indefinitely.
//
// Cleanup logic:
// 1. Delete rejected reviews 7 days after event ends (grace period for resubmission)
// 2. Mark unreviewed events as deleted when event starts (too late to review)
// 3. Delete pending reviews for past events
// 4. Archive approved/superseded reviews after 90 days
type ReviewQueueCleanupWorker struct {
	river.WorkerDefaults[ReviewQueueCleanupArgs]
	Repo   events.Repository
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	Slot   string // Deployment slot (blue/green) for metrics labeling
}

func (ReviewQueueCleanupWorker) Kind() string { return JobKindReviewQueueCleanup }

func (w ReviewQueueCleanupWorker) Work(ctx context.Context, job *river.Job[ReviewQueueCleanupArgs]) error {
	if w.Repo == nil {
		return fmt.Errorf("repository not configured")
	}
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	start := time.Now()

	logger.Info("starting review queue cleanup job",
		"slot", w.Slot,
		"attempt", job.Attempt,
	)

	// Track total rows affected across all cleanup operations
	var totalDeleted int64

	// 1. Delete rejected reviews for past events (7 day grace period)
	rejectedDeleted, err := w.cleanupExpiredRejections(ctx)
	if err != nil {
		logger.Error("failed to cleanup expired rejections",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup expired rejections: %w", err)
	}
	totalDeleted += rejectedDeleted
	logger.Info("cleaned up expired rejections",
		"slot", w.Slot,
		"deleted_count", rejectedDeleted,
	)

	// 2. Mark unreviewed events as deleted & delete pending reviews
	// Must do UPDATE before DELETE so subquery returns rows
	unreviewedDeleted, err := w.cleanupUnreviewedEvents(ctx)
	if err != nil {
		logger.Error("failed to cleanup unreviewed events",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup unreviewed events: %w", err)
	}
	totalDeleted += unreviewedDeleted
	logger.Info("cleaned up unreviewed events",
		"slot", w.Slot,
		"deleted_count", unreviewedDeleted,
	)

	// 3. Archive old approved/superseded reviews (90 day retention)
	archivedDeleted, err := w.cleanupArchivedReviews(ctx)
	if err != nil {
		logger.Error("failed to cleanup archived reviews",
			"slot", w.Slot,
			"error", err,
		)
		return fmt.Errorf("cleanup archived reviews: %w", err)
	}
	totalDeleted += archivedDeleted
	logger.Info("cleaned up archived reviews",
		"slot", w.Slot,
		"deleted_count", archivedDeleted,
	)

	duration := time.Since(start).Seconds()

	logger.Info("review queue cleanup job completed",
		"slot", w.Slot,
		"total_deleted", totalDeleted,
		"duration_seconds", duration,
	)

	return nil
}

// cleanupExpiredRejections deletes rejected reviews 7 days after event ends.
// This allows sources time to fix and resubmit, but doesn't keep rejections forever.
func (w ReviewQueueCleanupWorker) cleanupExpiredRejections(ctx context.Context) (int64, error) {
	const deleteQuery = `
		DELETE FROM event_review_queue
		WHERE status = 'rejected'
		AND (
			event_end_time < NOW() - INTERVAL '7 days'
			OR (event_end_time IS NULL AND event_start_time < NOW() - INTERVAL '7 days')
		)
	`

	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// cleanupUnreviewedEvents marks pending reviews as deleted when event starts.
// If an event wasn't reviewed before it started, it's too late - mark it deleted.
// Must run UPDATE before DELETE so subquery returns rows.
func (w ReviewQueueCleanupWorker) cleanupUnreviewedEvents(ctx context.Context) (int64, error) {
	// First, mark events as deleted
	const updateQuery = `
		UPDATE events SET lifecycle_state = 'deleted'
		WHERE id IN (
			SELECT event_id FROM event_review_queue
			WHERE status = 'pending' AND event_start_time < NOW()
		)
	`

	_, err := w.Pool.Exec(ctx, updateQuery)
	if err != nil {
		return 0, fmt.Errorf("mark unreviewed events deleted: %w", err)
	}

	// Then delete the review queue entries
	const deleteQuery = `
		DELETE FROM event_review_queue
		WHERE status = 'pending'
		AND event_start_time < NOW()
	`

	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return 0, fmt.Errorf("delete unreviewed entries: %w", err)
	}

	return result.RowsAffected(), nil
}

// cleanupArchivedReviews deletes approved/superseded reviews after 90 days.
// These are historical records - we don't need them forever.
func (w ReviewQueueCleanupWorker) cleanupArchivedReviews(ctx context.Context) (int64, error) {
	const deleteQuery = `
		DELETE FROM event_review_queue
		WHERE status IN ('approved', 'superseded')
		AND reviewed_at < NOW() - INTERVAL '90 days'
	`

	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}
