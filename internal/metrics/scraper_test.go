package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	. "github.com/Togather-Foundation/server/internal/metrics"
)

// TestScraperMetrics_Registered verifies that NewScraperMetrics registers all
// three metric families against the provided registry. We touch each Vec once
// so that Gather returns a non-empty MetricFamily for each family (counters and
// histograms are only included in Gather output once observed or touched).
func TestScraperMetrics_Registered(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	sm := NewScraperMetrics(reg)

	// Touch each Vec to produce at least one time series per family.
	sm.RunsTotal.WithLabelValues("_probe", "0", "success", "_probe")
	sm.RunDuration.WithLabelValues("_probe", "0", "_probe")
	sm.EventsTotal.WithLabelValues("_probe", "0", "found", "_probe")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("reg.Gather() returned error: %v", err)
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
			t.Errorf("metric %q not found in registry", name)
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
// are observable via testutil.ToFloat64 using a per-test registry.
func TestScraperRunsTotal_CounterIncrements(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	sm := NewScraperMetrics(reg)

	sm.RunsTotal.WithLabelValues("src", "0", "success", "slot").Inc()
	got := testutil.ToFloat64(sm.RunsTotal.WithLabelValues("src", "0", "success", "slot"))

	if got != 1 {
		t.Errorf("RunsTotal = %v, want 1", got)
	}
}

// TestScraperEventsTotal_CounterIncrements verifies events counter increments
// using a per-test registry.
func TestScraperEventsTotal_CounterIncrements(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	sm := NewScraperMetrics(reg)

	sm.EventsTotal.WithLabelValues("src", "1", "found", "slot").Add(5)
	got := testutil.ToFloat64(sm.EventsTotal.WithLabelValues("src", "1", "found", "slot"))

	if got != 5 {
		t.Errorf("EventsTotal = %v, want 5", got)
	}
}
