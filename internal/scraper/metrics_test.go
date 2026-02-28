package scraper

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/metrics"
)

// newMetricsTestScraper builds a Scraper with its own prometheus.Registry and
// ScraperMetrics for isolated metrics tests. Returns both the scraper and the
// metrics so assertions can read directly from the local Vecs (starting at 0).
func newMetricsTestScraper(slot string) (*Scraper, *metrics.ScraperMetrics) {
	reg := prometheus.NewRegistry()
	sm := metrics.NewScraperMetrics(reg)
	s := &Scraper{
		logger:         zerolog.Nop(),
		slot:           slot,
		scraperMetrics: sm,
	}
	return s, sm
}

// TestRecordMetrics_Success verifies that a successful result increments
// RunsTotal with result="success" and the correct labels.
func TestRecordMetrics_Success(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("test-slot")
	result := ScrapeResult{SourceName: "any-src", Tier: 0}

	s.recordMetrics(result, 2*time.Second)

	got := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("any-src", "0", "success", "test-slot"))
	if got != 1 {
		t.Errorf("RunsTotal = %v, want 1", got)
	}
}

// TestRecordMetrics_Error verifies that an error result increments
// RunsTotal with result="error".
func TestRecordMetrics_Error(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "any-src",
		Tier:       1,
		Error:      errors.New("scrape failed"),
	}

	s.recordMetrics(result, 500*time.Millisecond)

	got := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("any-src", "1", "error", "test-slot"))
	if got != 1 {
		t.Errorf("RunsTotal (error) = %v, want 1", got)
	}
}

// TestRecordMetrics_DryRun verifies that a dry-run result increments
// RunsTotal with result="dry_run".
func TestRecordMetrics_DryRun(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "any-src",
		Tier:       0,
		DryRun:     true,
	}

	s.recordMetrics(result, time.Second)

	got := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("any-src", "0", "dry_run", "test-slot"))
	if got != 1 {
		t.Errorf("RunsTotal (dry_run) = %v, want 1", got)
	}
}

// TestRecordMetrics_NoSlot verifies that recordMetrics is a no-op when slot is
// empty — no panic and no counter increment.
func TestRecordMetrics_NoSlot(t *testing.T) {
	t.Parallel()
	s := &Scraper{logger: zerolog.Nop(), slot: ""}
	result := ScrapeResult{SourceName: "noslot-src", Tier: 0}

	// Calling recordMetrics with empty slot must not panic.
	s.recordMetrics(result, time.Second)
}

// TestRecordMetrics_EventCounts verifies that EventsTotal is incremented
// for each non-zero event outcome bucket.
func TestRecordMetrics_EventCounts(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("count-slot")
	result := ScrapeResult{
		SourceName:      "any-src",
		Tier:            0,
		EventsFound:     10,
		EventsSubmitted: 8,
		EventsCreated:   5,
		EventsDuplicate: 2,
		EventsFailed:    1,
	}

	s.recordMetrics(result, time.Second)

	type labelExpect struct {
		outcome string
		want    float64
	}
	checks := []labelExpect{
		{"found", 10},
		{"submitted", 8},
		{"created", 5},
		{"duplicate", 2},
		{"failed", 1},
	}
	for _, c := range checks {
		got := testutil.ToFloat64(sm.EventsTotal.WithLabelValues("any-src", "0", c.outcome, "count-slot"))
		if got != c.want {
			t.Errorf("EventsTotal[%s] = %v, want %v", c.outcome, got, c.want)
		}
	}
}

// TestRecordMetrics_DryRunWithError verifies that when both DryRun=true and
// Error != nil, the result label is "error" — failures must never be silently
// hidden behind the "dry_run" bucket.
func TestRecordMetrics_DryRunWithError(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("test-slot")
	result := ScrapeResult{
		SourceName: "any-src",
		Tier:       0,
		DryRun:     true,
		Error:      errors.New("network timeout"),
	}

	s.recordMetrics(result, time.Second)

	errCount := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("any-src", "0", "error", "test-slot"))
	dryCount := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("any-src", "0", "dry_run", "test-slot"))

	if errCount != 1 {
		t.Errorf("RunsTotal (error) = %v, want 1 when DryRun+Error", errCount)
	}
	if dryCount != 0 {
		t.Errorf("RunsTotal (dry_run) = %v, want 0 when Error is set", dryCount)
	}
}

// TestRecordMetrics_ZeroCountsNotEmitted verifies that outcomes with zero
// counts are not separately incremented.
func TestRecordMetrics_ZeroCountsNotEmitted(t *testing.T) {
	t.Parallel()
	s, sm := newMetricsTestScraper("zero-slot")
	result := ScrapeResult{
		SourceName:      "any-src",
		Tier:            0,
		EventsFound:     0,
		EventsSubmitted: 0,
		EventsCreated:   0,
		EventsDuplicate: 0,
		EventsFailed:    0,
	}

	s.recordMetrics(result, time.Second)

	got := testutil.ToFloat64(sm.EventsTotal.WithLabelValues("any-src", "0", "found", "zero-slot"))
	if got != 0 {
		t.Errorf("EventsTotal[found] = %v, want 0 for zero count", got)
	}
}
