package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
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

// BatchIngestionArgs defines the job for processing batch event submissions.
type BatchIngestionArgs struct {
	BatchID string              `json:"batch_id"`
	Events  []events.EventInput `json:"events"`
}

func (BatchIngestionArgs) Kind() string { return JobKindBatchIngestion }

// BatchIngestionWorker processes batch event ingestion requests.
type BatchIngestionWorker struct {
	river.WorkerDefaults[BatchIngestionArgs]
	IngestService *events.IngestService
	Pool          *pgxpool.Pool
}

func (BatchIngestionWorker) Kind() string { return JobKindBatchIngestion }

func (w BatchIngestionWorker) Work(ctx context.Context, job *river.Job[BatchIngestionArgs]) error {
	if w.IngestService == nil {
		return fmt.Errorf("ingest service not configured")
	}
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}
	if job == nil {
		return fmt.Errorf("batch ingestion job missing")
	}

	batchID := job.Args.BatchID
	if batchID == "" {
		return fmt.Errorf("batch ID is required")
	}

	// Process each event in the batch
	results := make([]map[string]any, 0, len(job.Args.Events))
	for i, eventInput := range job.Args.Events {
		result, err := w.IngestService.Ingest(ctx, eventInput)
		itemResult := map[string]any{
			"index": i,
		}

		if err != nil {
			itemResult["status"] = "failed"
			itemResult["error"] = err.Error()
		} else if result.IsDuplicate {
			itemResult["status"] = "duplicate"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID
			}
		} else {
			itemResult["status"] = "created"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID
			}
		}

		results = append(results, itemResult)
	}

	// Store batch results in a table for status queries using SQLc
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal batch results: %w", err)
	}

	// Use SQLc to store batch results
	queries := postgres.New(w.Pool)
	err = queries.CreateBatchIngestionResult(ctx, postgres.CreateBatchIngestionResultParams{
		BatchID:     batchID,
		Results:     resultsJSON,
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("store batch results: %w", err)
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
func NewWorkersWithPool(pool *pgxpool.Pool, ingestService *events.IngestService) *river.Workers {
	workers := NewWorkers()
	river.AddWorker[IdempotencyCleanupArgs](workers, IdempotencyCleanupWorker{Pool: pool})
	river.AddWorker[BatchIngestionArgs](workers, BatchIngestionWorker{
		IngestService: ingestService,
		Pool:          pool,
	})
	return workers
}
