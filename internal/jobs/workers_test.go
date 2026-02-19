package jobs

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

func TestDeduplicationArgs_Kind(t *testing.T) {
	args := DeduplicationArgs{EventID: "test-event-123"}
	if args.Kind() != JobKindDeduplication {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindDeduplication)
	}
}

func TestReconciliationArgs_Kind(t *testing.T) {
	args := ReconciliationArgs{EntityType: "place", EntityID: "test-place-123"}
	if args.Kind() != JobKindReconciliation {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindReconciliation)
	}
}

func TestEnrichmentArgs_Kind(t *testing.T) {
	args := EnrichmentArgs{EntityType: "place", EntityID: "test-place-789", IdentifierURI: "http://example.org/123"}
	if args.Kind() != JobKindEnrichment {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindEnrichment)
	}
}

func TestIdempotencyCleanupArgs_Kind(t *testing.T) {
	args := IdempotencyCleanupArgs{}
	if args.Kind() != JobKindIdempotencyCleanup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindIdempotencyCleanup)
	}
}

func TestBatchResultsCleanupArgs_Kind(t *testing.T) {
	args := BatchResultsCleanupArgs{}
	if args.Kind() != JobKindBatchResultsCleanup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindBatchResultsCleanup)
	}
}

func TestBatchIngestionArgs_Kind(t *testing.T) {
	args := BatchIngestionArgs{BatchID: "batch-123"}
	if args.Kind() != JobKindBatchIngestion {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindBatchIngestion)
	}
}

func TestDeduplicationWorker_Kind(t *testing.T) {
	worker := DeduplicationWorker{}
	if worker.Kind() != JobKindDeduplication {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindDeduplication)
	}
}

func TestReconciliationWorker_Kind(t *testing.T) {
	worker := ReconciliationWorker{}
	if worker.Kind() != JobKindReconciliation {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindReconciliation)
	}
}

func TestEnrichmentWorker_Kind(t *testing.T) {
	worker := EnrichmentWorker{}
	if worker.Kind() != JobKindEnrichment {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindEnrichment)
	}
}

func TestIdempotencyCleanupWorker_Kind(t *testing.T) {
	worker := IdempotencyCleanupWorker{}
	if worker.Kind() != JobKindIdempotencyCleanup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindIdempotencyCleanup)
	}
}

func TestBatchResultsCleanupWorker_Kind(t *testing.T) {
	worker := BatchResultsCleanupWorker{}
	if worker.Kind() != JobKindBatchResultsCleanup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindBatchResultsCleanup)
	}
}

func TestBatchIngestionWorker_Kind(t *testing.T) {
	worker := BatchIngestionWorker{}
	if worker.Kind() != JobKindBatchIngestion {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindBatchIngestion)
	}
}

func TestDeduplicationWorker_WorkWithNilJob(t *testing.T) {
	worker := DeduplicationWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestReconciliationWorker_WorkWithNilJob(t *testing.T) {
	worker := ReconciliationWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestEnrichmentWorker_WorkWithNilJob(t *testing.T) {
	worker := EnrichmentWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestIdempotencyCleanupWorker_WorkWithNilPool(t *testing.T) {
	worker := IdempotencyCleanupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[IdempotencyCleanupArgs]{
		Args: IdempotencyCleanupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}

func TestBatchResultsCleanupWorker_WorkWithNilPool(t *testing.T) {
	worker := BatchResultsCleanupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[BatchResultsCleanupArgs]{
		Args: BatchResultsCleanupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}

func TestBatchIngestionWorker_WorkWithNilIngestService(t *testing.T) {
	ctx := context.Background()
	worker := BatchIngestionWorker{
		IngestService: nil,
		Pool:          nil,
	}

	job := &river.Job[BatchIngestionArgs]{
		Args: BatchIngestionArgs{BatchID: "test"},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() should return error when IngestService is nil")
	}
}

func TestBatchIngestionWorker_WorkWithNilJob(t *testing.T) {
	ctx := context.Background()
	worker := BatchIngestionWorker{}

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestNewWorkers(t *testing.T) {
	workers := NewWorkers()

	if workers == nil {
		t.Fatal("NewWorkers() returned nil")
	}
}

func TestDeduplicationWorker_WorkWithValidJob(t *testing.T) {
	worker := DeduplicationWorker{}
	ctx := context.Background()

	job := &river.Job[DeduplicationArgs]{
		Args: DeduplicationArgs{
			EventID: "test-event-id",
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Errorf("Work() with valid job should not error, got: %v", err)
	}
}

func TestReconciliationWorker_WorkWithValidJob(t *testing.T) {
	worker := ReconciliationWorker{}
	ctx := context.Background()

	job := &river.Job[ReconciliationArgs]{
		Args: ReconciliationArgs{
			EntityType: "place",
			EntityID:   "test-place-id",
		},
	}

	err := worker.Work(ctx, job)
	// Expect error because Pool and ReconciliationService are nil
	if err == nil {
		t.Error("Work() should return error when dependencies are nil")
	}
}

func TestEnrichmentWorker_WorkWithValidJob(t *testing.T) {
	worker := EnrichmentWorker{}
	ctx := context.Background()

	job := &river.Job[EnrichmentArgs]{
		Args: EnrichmentArgs{
			EntityType:    "place",
			EntityID:      "test-place-id",
			IdentifierURI: "http://example.org/123",
		},
	}

	err := worker.Work(ctx, job)
	// Expect error because Pool and ReconciliationService are nil
	if err == nil {
		t.Error("Work() should return error when dependencies are nil")
	}
}

func TestUsageRollupArgs_Kind(t *testing.T) {
	args := UsageRollupArgs{}
	if args.Kind() != JobKindUsageRollup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindUsageRollup)
	}
}

func TestUsageRollupWorker_Kind(t *testing.T) {
	worker := UsageRollupWorker{}
	if worker.Kind() != JobKindUsageRollup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindUsageRollup)
	}
}

func TestUsageRollupWorker_WorkWithNilPool(t *testing.T) {
	worker := UsageRollupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[UsageRollupArgs]{
		Args: UsageRollupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}
