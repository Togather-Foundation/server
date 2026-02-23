package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	. "github.com/Togather-Foundation/server/internal/metrics"
)

// TestScraperMetrics_Registered verifies that all three scraper metric
// families are registered in the global Registry. We trigger a label lookup
// (which creates the time series) then gather to confirm presence.
func TestScraperMetrics_Registered(t *testing.T) {
	t.Parallel()
	// Touch each metric so Gather returns a non-empty descriptor for each family.
	ScraperRunsTotal.WithLabelValues("_probe", "0", "success", "_probe")
	ScraperRunDuration.WithLabelValues("_probe", "0", "_probe")
	ScraperEventsTotal.WithLabelValues("_probe", "0", "found", "_probe")

	mfs, err := Registry.Gather()
	if err != nil {
		t.Fatalf("Registry.Gather() returned error: %v", err)
	}

	want := map[string]bool{
		"togather_scraper_runs_total":           false,
		"togather_scraper_run_duration_seconds": false,
		"togather_scraper_events_total":         false,
	}

	for _, mf := range mfs {
		if _, ok := want[mf.GetName()]; ok {
			want[mf.GetName()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("metric %q not found in Registry", name)
		}
	}
}

// TestScraperRunsTotal_LabelCardinality verifies the counter accepts the
// expected label set without panicking.
func TestScraperRunsTotal_LabelCardinality(t *testing.T) {
	t.Parallel()
	// This call will panic if label names don't match the registered set.
	_ = ScraperRunsTotal.WithLabelValues("my-source", "0", "success", "blue")
}

// TestScraperRunDuration_LabelCardinality verifies histogram label set.
func TestScraperRunDuration_LabelCardinality(t *testing.T) {
	t.Parallel()
	_ = ScraperRunDuration.WithLabelValues("my-source", "1", "blue")
}

// TestScraperEventsTotal_LabelCardinality verifies events counter label set.
func TestScraperEventsTotal_LabelCardinality(t *testing.T) {
	t.Parallel()
	_ = ScraperEventsTotal.WithLabelValues("my-source", "0", "found", "blue")
}

// TestScraperRunsTotal_CounterIncrements verifies that the counter increments
// are observable via testutil.ToFloat64.
func TestScraperRunsTotal_CounterIncrements(t *testing.T) {
	t.Parallel()
	before := testutil.ToFloat64(ScraperRunsTotal.WithLabelValues("label-test-src", "0", "success", "test-slot"))
	ScraperRunsTotal.WithLabelValues("label-test-src", "0", "success", "test-slot").Inc()
	after := testutil.ToFloat64(ScraperRunsTotal.WithLabelValues("label-test-src", "0", "success", "test-slot"))

	if after-before != 1 {
		t.Errorf("ScraperRunsTotal delta = %v, want 1", after-before)
	}
}

// TestScraperEventsTotal_CounterIncrements verifies events counter increments.
func TestScraperEventsTotal_CounterIncrements(t *testing.T) {
	t.Parallel()
	before := testutil.ToFloat64(ScraperEventsTotal.WithLabelValues("label-test-src", "1", "found", "test-slot"))
	ScraperEventsTotal.WithLabelValues("label-test-src", "1", "found", "test-slot").Add(5)
	after := testutil.ToFloat64(ScraperEventsTotal.WithLabelValues("label-test-src", "1", "found", "test-slot"))

	if after-before != 5 {
		t.Errorf("ScraperEventsTotal delta = %v, want 5", after-before)
	}
}
