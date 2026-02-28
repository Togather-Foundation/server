package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

const (
	JobKindSubmissionsCleanup = "submissions_cleanup"

	// submissionsRetentionDays is the number of days to retain processed/rejected
	// submissions before deletion.  Pending and pending_validation rows are never
	// deleted by this job.
	submissionsRetentionDays = 90
)

// SubmissionsCleanupArgs defines the periodic job that purges old submissions.
type SubmissionsCleanupArgs struct{}

func (SubmissionsCleanupArgs) Kind() string { return JobKindSubmissionsCleanup }

// SubmissionsCleanupWorker deletes accepted and rejected scraper_submissions rows
// older than submissionsRetentionDays (90 days).  Pending and pending_validation
// rows are never touched.  Runs daily (srv-3sac0).
type SubmissionsCleanupWorker struct {
	river.WorkerDefaults[SubmissionsCleanupArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (w SubmissionsCleanupWorker) Work(ctx context.Context, job *river.Job[SubmissionsCleanupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("submissions_cleanup: database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	start := time.Now()
	logger.InfoContext(ctx, "submissions_cleanup: starting", "retention_days", submissionsRetentionDays)

	const microsPerDay = int64(24 * 3600 * 1_000_000)
	interval := pgtype.Interval{
		Microseconds: submissionsRetentionDays * microsPerDay,
		Valid:        true,
	}

	deleted, err := postgres.New(w.Pool).DeleteOldScraperSubmissions(ctx, interval)
	if err != nil {
		logger.ErrorContext(ctx, "submissions_cleanup: delete failed", "error", err)
		return fmt.Errorf("submissions_cleanup: delete old submissions: %w", err)
	}

	logger.InfoContext(ctx, "submissions_cleanup: completed",
		"deleted_count", deleted,
		"duration_seconds", time.Since(start).Seconds(),
	)
	return nil
}
