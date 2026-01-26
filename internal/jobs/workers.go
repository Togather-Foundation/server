package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type DeduplicationArgs struct {
	EventID string `json:"event_id"`
}

func (DeduplicationArgs) Kind() string { return JobKindDeduplication }

type ReconciliationArgs struct {
	EventID string `json:"event_id"`
}

func (ReconciliationArgs) Kind() string { return JobKindReconciliation }

type EnrichmentArgs struct {
	EventID string `json:"event_id"`
}

func (EnrichmentArgs) Kind() string { return JobKindEnrichment }

type DeduplicationWorker struct {
	river.WorkerDefaults[DeduplicationArgs]
}

func (DeduplicationWorker) Kind() string { return JobKindDeduplication }

func (DeduplicationWorker) Work(ctx context.Context, job *river.Job[DeduplicationArgs]) error {
	if job == nil {
		return fmt.Errorf("deduplication job missing")
	}
	return nil
}

type ReconciliationWorker struct {
	river.WorkerDefaults[ReconciliationArgs]
}

func (ReconciliationWorker) Kind() string { return JobKindReconciliation }

func (ReconciliationWorker) Work(ctx context.Context, job *river.Job[ReconciliationArgs]) error {
	if job == nil {
		return fmt.Errorf("reconciliation job missing")
	}
	return nil
}

type EnrichmentWorker struct {
	river.WorkerDefaults[EnrichmentArgs]
}

func (EnrichmentWorker) Kind() string { return JobKindEnrichment }

func (EnrichmentWorker) Work(ctx context.Context, job *river.Job[EnrichmentArgs]) error {
	if job == nil {
		return fmt.Errorf("enrichment job missing")
	}
	return nil
}

// IdempotencyCleanupArgs defines the job for cleaning expired idempotency keys.
type IdempotencyCleanupArgs struct{}

func (IdempotencyCleanupArgs) Kind() string { return JobKindIdempotencyCleanup }

// IdempotencyCleanupWorker removes expired idempotency keys (>24h old).
type IdempotencyCleanupWorker struct {
	river.WorkerDefaults[IdempotencyCleanupArgs]
	Pool *pgxpool.Pool
}

func (IdempotencyCleanupWorker) Kind() string { return JobKindIdempotencyCleanup }

func (w IdempotencyCleanupWorker) Work(ctx context.Context, job *river.Job[IdempotencyCleanupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	// Delete expired idempotency keys
	const deleteQuery = `DELETE FROM idempotency_keys WHERE expires_at <= now()`
	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return fmt.Errorf("delete expired idempotency keys: %w", err)
	}

	rows := result.RowsAffected()
	if rows > 0 {
		// Log cleanup success (context available through River's logger)
		_ = rows // Successfully cleaned up expired keys
	}

	return nil
}

func NewWorkers() *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker[DeduplicationArgs](workers, DeduplicationWorker{})
	river.AddWorker[ReconciliationArgs](workers, ReconciliationWorker{})
	river.AddWorker[EnrichmentArgs](workers, EnrichmentWorker{})
	return workers
}

// NewWorkersWithPool creates workers including cleanup jobs that need DB access.
func NewWorkersWithPool(pool *pgxpool.Pool) *river.Workers {
	workers := NewWorkers()
	river.AddWorker[IdempotencyCleanupArgs](workers, IdempotencyCleanupWorker{Pool: pool})
	return workers
}
