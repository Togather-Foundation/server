package jobs

import (
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const (
	JobKindDeduplication       = "deduplication"
	JobKindReconciliation      = "reconciliation"
	JobKindEnrichment          = "enrichment"
	JobKindIdempotencyCleanup  = "idempotency_cleanup"
	JobKindBatchIngestion      = "batch_ingestion"
	JobKindBatchResultsCleanup = "batch_results_cleanup"
)

const (
	DeduplicationMaxAttempts  = 1
	ReconciliationMaxAttempts = 5
	EnrichmentMaxAttempts     = 10
	BatchIngestionMaxAttempts = 3
)

// RetryConfig controls per-kind retry behavior.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// RetryPolicy implements River's ClientRetryPolicy with per-kind exponential backoff.
type RetryPolicy struct {
	Default RetryConfig
	ByKind  map[string]RetryConfig
}

// NewRetryPolicy returns the default retry policy configuration.
func NewRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		Default: RetryConfig{
			MaxAttempts: ReconciliationMaxAttempts,
			BaseDelay:   30 * time.Second,
			MaxDelay:    30 * time.Minute,
		},
		ByKind: map[string]RetryConfig{
			JobKindDeduplication: {
				MaxAttempts: DeduplicationMaxAttempts,
				BaseDelay:   0,
				MaxDelay:    0,
			},
			JobKindReconciliation: {
				MaxAttempts: ReconciliationMaxAttempts,
				BaseDelay:   1 * time.Minute,
				MaxDelay:    1 * time.Hour,
			},
			JobKindEnrichment: {
				MaxAttempts: EnrichmentMaxAttempts,
				BaseDelay:   2 * time.Minute,
				MaxDelay:    2 * time.Hour,
			},
			JobKindBatchIngestion: {
				MaxAttempts: BatchIngestionMaxAttempts,
				BaseDelay:   30 * time.Second,
				MaxDelay:    5 * time.Minute,
			},
		},
	}
}

// NextRetry determines the next retry time for a failed job.
func (p *RetryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	config := p.configFor(job.Kind)
	if config.BaseDelay == 0 {
		return time.Now()
	}

	attempt := job.Attempt
	if attempt < 1 {
		attempt = 1
	}

	delay := time.Duration(float64(config.BaseDelay) * math.Pow(2, float64(attempt-1)))
	if config.MaxDelay > 0 && delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	if job.AttemptedAt != nil {
		return job.AttemptedAt.Add(delay)
	}

	return time.Now().Add(delay)
}

// InsertOptsForKind returns default insert options for a job kind.
func InsertOptsForKind(kind string) river.InsertOpts {
	config := NewRetryPolicy().configFor(kind)
	return river.InsertOpts{MaxAttempts: config.MaxAttempts}
}

// NewClientConfig builds a River client configuration with retry policy.
func NewClientConfig(workers *river.Workers, logger *slog.Logger) *river.Config {
	policy := NewRetryPolicy()
	config := &river.Config{
		Workers:     workers,
		RetryPolicy: policy,
		MaxAttempts: policy.Default.MaxAttempts,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
	}
	if logger != nil {
		config.Logger = logger
		config.ErrorHandler = NewAlertingErrorHandler(logger, nil)
	}
	return config
}

// NewClient creates a River client using pgx v5.
func NewClient(pool *pgxpool.Pool, workers *river.Workers, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), NewClientConfig(workers, logger))
}

func (p *RetryPolicy) configFor(kind string) RetryConfig {
	if p == nil {
		return RetryConfig{MaxAttempts: ReconciliationMaxAttempts, BaseDelay: 1 * time.Minute, MaxDelay: 1 * time.Hour}
	}
	if config, ok := p.ByKind[kind]; ok {
		return config
	}
	return p.Default
}
