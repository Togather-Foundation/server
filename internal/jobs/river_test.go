package jobs

import (
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
)

func TestNewRetryPolicy(t *testing.T) {
	policy := NewRetryPolicy()

	if policy == nil {
		t.Fatal("NewRetryPolicy() returned nil")
	}

	// Test default config
	if policy.Default.MaxAttempts != ReconciliationMaxAttempts {
		t.Errorf("Default.MaxAttempts = %d, want %d", policy.Default.MaxAttempts, ReconciliationMaxAttempts)
	}
	if policy.Default.BaseDelay != 30*time.Second {
		t.Errorf("Default.BaseDelay = %v, want 30s", policy.Default.BaseDelay)
	}
	if policy.Default.MaxDelay != 30*time.Minute {
		t.Errorf("Default.MaxDelay = %v, want 30m", policy.Default.MaxDelay)
	}

	// Test per-kind configs
	tests := []struct {
		kind                string
		expectedMaxAttempts int
		expectedBaseDelay   time.Duration
		expectedMaxDelay    time.Duration
	}{
		{
			kind:                JobKindDeduplication,
			expectedMaxAttempts: DeduplicationMaxAttempts,
			expectedBaseDelay:   0,
			expectedMaxDelay:    0,
		},
		{
			kind:                JobKindReconciliation,
			expectedMaxAttempts: ReconciliationMaxAttempts,
			expectedBaseDelay:   1 * time.Minute,
			expectedMaxDelay:    1 * time.Hour,
		},
		{
			kind:                JobKindEnrichment,
			expectedMaxAttempts: EnrichmentMaxAttempts,
			expectedBaseDelay:   2 * time.Minute,
			expectedMaxDelay:    2 * time.Hour,
		},
		{
			kind:                JobKindBatchIngestion,
			expectedMaxAttempts: BatchIngestionMaxAttempts,
			expectedBaseDelay:   30 * time.Second,
			expectedMaxDelay:    5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			config, ok := policy.ByKind[tt.kind]
			if !ok {
				t.Fatalf("kind %s not found in ByKind map", tt.kind)
			}

			if config.MaxAttempts != tt.expectedMaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", config.MaxAttempts, tt.expectedMaxAttempts)
			}
			if config.BaseDelay != tt.expectedBaseDelay {
				t.Errorf("BaseDelay = %v, want %v", config.BaseDelay, tt.expectedBaseDelay)
			}
			if config.MaxDelay != tt.expectedMaxDelay {
				t.Errorf("MaxDelay = %v, want %v", config.MaxDelay, tt.expectedMaxDelay)
			}
		})
	}
}

func TestRetryPolicy_NextRetry(t *testing.T) {
	policy := NewRetryPolicy()
	now := time.Now()

	tests := []struct {
		name           string
		kind           string
		attempt        int
		expectedDelay  time.Duration
		toleranceRange time.Duration // Allow some time difference due to execution
	}{
		{
			name:           "deduplication no retry",
			kind:           JobKindDeduplication,
			attempt:        1,
			expectedDelay:  0,
			toleranceRange: 1 * time.Second,
		},
		{
			name:           "reconciliation first attempt",
			kind:           JobKindReconciliation,
			attempt:        1,
			expectedDelay:  1 * time.Minute,
			toleranceRange: 2 * time.Second,
		},
		{
			name:           "reconciliation second attempt (exponential backoff)",
			kind:           JobKindReconciliation,
			attempt:        2,
			expectedDelay:  2 * time.Minute,
			toleranceRange: 2 * time.Second,
		},
		{
			name:           "reconciliation third attempt",
			kind:           JobKindReconciliation,
			attempt:        3,
			expectedDelay:  4 * time.Minute,
			toleranceRange: 2 * time.Second,
		},
		{
			name:           "enrichment first attempt",
			kind:           JobKindEnrichment,
			attempt:        1,
			expectedDelay:  2 * time.Minute,
			toleranceRange: 2 * time.Second,
		},
		{
			name:           "enrichment second attempt",
			kind:           JobKindEnrichment,
			attempt:        2,
			expectedDelay:  4 * time.Minute,
			toleranceRange: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &rivertype.JobRow{
				Kind:        tt.kind,
				Attempt:     tt.attempt,
				AttemptedAt: &now,
			}

			nextRetry := policy.NextRetry(job)
			actualDelay := nextRetry.Sub(now)

			// Check if delay is within expected range
			diff := actualDelay - tt.expectedDelay
			if diff < 0 {
				diff = -diff
			}

			if diff > tt.toleranceRange {
				t.Errorf("NextRetry() delay = %v, want approximately %v (diff: %v)", actualDelay, tt.expectedDelay, diff)
			}
		})
	}
}

func TestInsertOptsForKind(t *testing.T) {
	tests := []struct {
		kind                string
		expectedMaxAttempts int
	}{
		{JobKindDeduplication, DeduplicationMaxAttempts},
		{JobKindReconciliation, ReconciliationMaxAttempts},
		{JobKindEnrichment, EnrichmentMaxAttempts},
		{JobKindBatchIngestion, BatchIngestionMaxAttempts},
		{"unknown-kind", ReconciliationMaxAttempts}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			opts := InsertOptsForKind(tt.kind)

			if opts.MaxAttempts != tt.expectedMaxAttempts {
				t.Errorf("InsertOptsForKind(%s).MaxAttempts = %d, want %d",
					tt.kind, opts.MaxAttempts, tt.expectedMaxAttempts)
			}
		})
	}
}

func TestNewPeriodicJobs(t *testing.T) {
	jobs := NewPeriodicJobs()

	if len(jobs) != 3 {
		t.Errorf("NewPeriodicJobs() returned %d jobs, want 3", len(jobs))
	}

	// Verify jobs are created
	for i, job := range jobs {
		if job == nil {
			t.Errorf("NewPeriodicJobs()[%d] is nil", i)
		}
	}
}

func TestJobKindConstants(t *testing.T) {
	// Test that job kind constants are unique and non-empty
	kinds := []string{
		JobKindDeduplication,
		JobKindReconciliation,
		JobKindEnrichment,
		JobKindIdempotencyCleanup,
		JobKindBatchIngestion,
		JobKindBatchResultsCleanup,
		JobKindReviewQueueCleanup,
	}

	seen := make(map[string]bool)
	for _, kind := range kinds {
		if kind == "" {
			t.Errorf("job kind constant is empty")
		}

		if seen[kind] {
			t.Errorf("duplicate job kind: %s", kind)
		}
		seen[kind] = true
	}
}
