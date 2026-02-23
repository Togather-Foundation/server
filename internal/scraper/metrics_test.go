package scraper

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/metrics"
)

// newMetricsTestScraper builds a Scraper with the given slot for metrics tests.
// ingest and queries are nil — only metrics recording is exercised.
func newMetricsTestScraper(slot string) *Scraper {
	return &Scraper{
		logger: zerolog.Nop(),
		slot:   slot,
	}
}

// TestRecordMetrics_Success verifies that a successful result increments
// ScraperRunsTotal with result="success" and the correct labels.
func TestRecordMetrics_Success(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "metrics-success-src",
		Tier:       0,
	}

	before := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-success-src", "0", "success", "test-slot"),
	)

	s.recordMetrics(result, 2*time.Second)

	after := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-success-src", "0", "success", "test-slot"),
	)

	if after-before != 1 {
		t.Errorf("ScraperRunsTotal (success) delta = %v, want 1", after-before)
	}
}

// TestRecordMetrics_Error verifies that an error result increments
// ScraperRunsTotal with result="error".
func TestRecordMetrics_Error(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "metrics-error-src",
		Tier:       1,
		Error:      errors.New("scrape failed"),
	}

	before := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-error-src", "1", "error", "test-slot"),
	)

	s.recordMetrics(result, 500*time.Millisecond)

	after := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-error-src", "1", "error", "test-slot"),
	)

	if after-before != 1 {
		t.Errorf("ScraperRunsTotal (error) delta = %v, want 1", after-before)
	}
}

// TestRecordMetrics_DryRun verifies that a dry-run result increments
// ScraperRunsTotal with result="dry_run".
func TestRecordMetrics_DryRun(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "metrics-dryrun-src",
		Tier:       0,
		DryRun:     true,
	}

	before := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-src", "0", "dry_run", "test-slot"),
	)

	s.recordMetrics(result, time.Second)

	after := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-src", "0", "dry_run", "test-slot"),
	)

	if after-before != 1 {
		t.Errorf("ScraperRunsTotal (dry_run) delta = %v, want 1", after-before)
	}
}

// TestRecordMetrics_NoSlot verifies that recordMetrics is a no-op when slot is
// empty — no panic and no counter increment.
func TestRecordMetrics_NoSlot(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("")
	result := ScrapeResult{
		SourceName: "noslot-src",
		Tier:       0,
	}

	// Calling recordMetrics with empty slot must not panic.
	s.recordMetrics(result, time.Second)
}

// TestRecordMetrics_EventCounts verifies that ScraperEventsTotal is incremented
// for each non-zero event outcome bucket.
func TestRecordMetrics_EventCounts(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("count-slot")
	result := ScrapeResult{
		SourceName:      "metrics-counts-src",
		Tier:            0,
		EventsFound:     10,
		EventsSubmitted: 8,
		EventsCreated:   5,
		EventsDuplicate: 2,
		EventsFailed:    1,
	}

	type labelExpect struct {
		outcome string
		delta   float64
	}
	checks := []labelExpect{
		{"found", 10},
		{"submitted", 8},
		{"created", 5},
		{"duplicate", 2},
		{"failed", 1},
	}

	befores := make(map[string]float64, len(checks))
	for _, c := range checks {
		befores[c.outcome] = testutil.ToFloat64(
			metrics.ScraperEventsTotal.WithLabelValues("metrics-counts-src", "0", c.outcome, "count-slot"),
		)
	}

	s.recordMetrics(result, time.Second)

	for _, c := range checks {
		after := testutil.ToFloat64(
			metrics.ScraperEventsTotal.WithLabelValues("metrics-counts-src", "0", c.outcome, "count-slot"),
		)
		got := after - befores[c.outcome]
		if got != c.delta {
			t.Errorf("ScraperEventsTotal[%s] delta = %v, want %v", c.outcome, got, c.delta)
		}
	}
}

// TestRecordMetrics_DryRunWithError verifies that when both DryRun=true and
// Error != nil, the result label is "error" — failures must never be silently
// hidden behind the "dry_run" bucket.
func TestRecordMetrics_DryRunWithError(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "metrics-dryrun-err-src",
		Tier:       0,
		DryRun:     true,
		Error:      errors.New("network timeout"),
	}

	beforeErr := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-err-src", "0", "error", "test-slot"),
	)
	beforeDry := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-err-src", "0", "dry_run", "test-slot"),
	)

	s.recordMetrics(result, time.Second)

	afterErr := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-err-src", "0", "error", "test-slot"),
	)
	afterDry := testutil.ToFloat64(
		metrics.ScraperRunsTotal.WithLabelValues("metrics-dryrun-err-src", "0", "dry_run", "test-slot"),
	)

	if afterErr-beforeErr != 1 {
		t.Errorf("ScraperRunsTotal (error) delta = %v, want 1 when DryRun+Error", afterErr-beforeErr)
	}
	if afterDry-beforeDry != 0 {
		t.Errorf("ScraperRunsTotal (dry_run) delta = %v, want 0 when Error is set", afterDry-beforeDry)
	}
}

// TestRecordMetrics_ZeroCountsNotEmitted verifies that outcomes with zero
// counts are not separately incremented (delta should be 0).
func TestRecordMetrics_ZeroCountsNotEmitted(t *testing.T) {
	t.Parallel()
	s := newMetricsTestScraper("zero-slot")
	result := ScrapeResult{
		SourceName:      "metrics-zero-src",
		Tier:            0,
		EventsFound:     0,
		EventsSubmitted: 0,
		EventsCreated:   0,
		EventsDuplicate: 0,
		EventsFailed:    0,
	}

	before := testutil.ToFloat64(
		metrics.ScraperEventsTotal.WithLabelValues("metrics-zero-src", "0", "found", "zero-slot"),
	)

	s.recordMetrics(result, time.Second)

	after := testutil.ToFloat64(
		metrics.ScraperEventsTotal.WithLabelValues("metrics-zero-src", "0", "found", "zero-slot"),
	)

	if after-before != 0 {
		t.Errorf("ScraperEventsTotal[found] delta = %v, want 0 for zero count", after-before)
	}
}
