package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// BatchResultsCleanupArgs defines the job for cleaning expired batch ingestion results.
type BatchResultsCleanupArgs struct{}

func (BatchResultsCleanupArgs) Kind() string { return JobKindBatchResultsCleanup }

// BatchResultsCleanupWorker removes old batch ingestion results (>7 days old).
// This prevents the batch_ingestion_results table from growing indefinitely.
// Clients should poll for results within 7 days of submission.
type BatchResultsCleanupWorker struct {
	river.WorkerDefaults[BatchResultsCleanupArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (BatchResultsCleanupWorker) Kind() string { return JobKindBatchResultsCleanup }

func (w BatchResultsCleanupWorker) Work(ctx context.Context, job *river.Job[BatchResultsCleanupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Delete batch results older than 7 days
	const deleteQuery = `DELETE FROM batch_ingestion_results WHERE completed_at < now() - INTERVAL '7 days'`
	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		logger.Error("failed to delete expired batch results", "error", err)
		return fmt.Errorf("delete expired batch results: %w", err)
	}

	rows := result.RowsAffected()
	if rows > 0 {
		logger.Info("cleaned up expired batch results",
			"deleted_count", rows,
		)
	}

	return nil
}

// BatchIngestionArgs defines the job arguments for processing batch event submissions.
// Each batch job processes multiple events asynchronously and stores results
// in the batch_ingestion_results table for status queries.
type BatchIngestionArgs struct {
	// BatchID is the unique identifier for this batch job (ULID format)
	BatchID string `json:"batch_id"`
	// Events is the list of event inputs to process (max 100 per batch)
	Events []events.EventInput `json:"events"`
}

func (BatchIngestionArgs) Kind() string { return JobKindBatchIngestion }

// BatchIngestionWorker processes batch event ingestion requests asynchronously.
// It ingests each event individually, tracks success/failure/duplicate status,
// and stores aggregate results in the database for client polling via GET /batch-status/{id}.
//
// The worker logs structured information about batch progress including counts of
// created, duplicate, and failed events. Individual event failures do not fail the
// entire batch - partial success is supported.
type BatchIngestionWorker struct {
	river.WorkerDefaults[BatchIngestionArgs]
	// IngestService handles individual event ingestion logic
	IngestService *events.IngestService
	// Pool provides database access for storing batch results
	Pool *pgxpool.Pool
	// Logger provides structured logging (defaults to slog.Default() if nil)
	Logger *slog.Logger
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

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("starting batch ingestion",
		"batch_id", batchID,
		"event_count", len(job.Args.Events),
		"attempt", job.Attempt,
	)

	// Process each event in the batch
	var successCount, duplicateCount, failureCount int
	results := make([]map[string]any, 0, len(job.Args.Events))
	for i, eventInput := range job.Args.Events {
		result, err := w.IngestService.Ingest(ctx, eventInput)
		itemResult := map[string]any{
			"index": i,
		}

		if err != nil {
			itemResult["status"] = "failed"
			itemResult["error"] = err.Error()
			failureCount++
			logger.Warn("batch event ingestion failed",
				"batch_id", batchID,
				"index", i,
				"error", err,
			)
		} else if result.IsDuplicate {
			itemResult["status"] = "duplicate"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID
			}
			duplicateCount++
		} else {
			itemResult["status"] = "created"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID
			}
			successCount++
		}

		results = append(results, itemResult)
	}

	logger.Info("batch ingestion processing complete",
		"batch_id", batchID,
		"total", len(job.Args.Events),
		"created", successCount,
		"duplicates", duplicateCount,
		"failures", failureCount,
	)

	// Store batch results in a table for status queries using SQLc
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		logger.Error("failed to marshal batch results",
			"batch_id", batchID,
			"error", err,
		)
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
		logger.Error("failed to store batch results",
			"batch_id", batchID,
			"error", err,
		)
		return fmt.Errorf("store batch results: %w", err)
	}

	logger.Info("batch ingestion completed successfully",
		"batch_id", batchID,
	)

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
func NewWorkersWithPool(pool *pgxpool.Pool, ingestService *events.IngestService, logger *slog.Logger) *river.Workers {
	workers := NewWorkers()
	river.AddWorker[IdempotencyCleanupArgs](workers, IdempotencyCleanupWorker{Pool: pool})
	river.AddWorker[BatchResultsCleanupArgs](workers, BatchResultsCleanupWorker{
		Pool:   pool,
		Logger: logger,
	})
	river.AddWorker[BatchIngestionArgs](workers, BatchIngestionWorker{
		IngestService: ingestService,
		Pool:          pool,
		Logger:        logger,
	})
	return workers
}
