package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Scraper metrics
var (
	// ScraperRunsTotal counts completed scrape runs by outcome.
	// Labels: source (source name), tier ("0" or "1"), result ("success"/"error"/"dry_run"), slot
	ScraperRunsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scraper_runs_total",
			Help:      "Total number of completed scrape runs",
		},
		[]string{"source", "tier", "result", "slot"},
	)

	// ScraperRunDuration observes scrape run duration in seconds.
	// Labels: source, tier, slot
	// Buckets cover the expected range from sub-second to 5-minute runs.
	ScraperRunDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "scraper_run_duration_seconds",
			Help:      "Scrape run duration in seconds",
			Buckets:   []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		},
		[]string{"source", "tier", "slot"},
	)

	// ScraperEventsTotal counts events processed by outcome.
	// Labels: source, tier, outcome ("found"/"submitted"/"created"/"duplicate"/"failed"), slot
	ScraperEventsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scraper_events_total",
			Help:      "Total number of events processed by the scraper, by outcome",
		},
		[]string{"source", "tier", "outcome", "slot"},
	)
)
