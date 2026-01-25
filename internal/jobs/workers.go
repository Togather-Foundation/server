package jobs

import (
	"context"
	"fmt"

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

func NewWorkers() *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker[DeduplicationArgs](workers, DeduplicationWorker{})
	river.AddWorker[ReconciliationArgs](workers, ReconciliationWorker{})
	river.AddWorker[EnrichmentArgs](workers, EnrichmentWorker{})
	return workers
}
