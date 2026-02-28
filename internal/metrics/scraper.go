package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ScraperMetrics holds the Prometheus metric vectors for scraper instrumentation.
// Use NewScraperMetrics to construct with a custom registry (useful in tests).
//
// Fields are set once by NewScraperMetrics and must not be replaced afterwards;
// all three are guaranteed non-nil after construction.
type ScraperMetrics struct {
	RunsTotal   *prometheus.CounterVec
	RunDuration *prometheus.HistogramVec
	EventsTotal *prometheus.CounterVec
}

// NewScraperMetrics creates a new ScraperMetrics registered against reg.
// Use metrics.Registry for production; use prometheus.NewRegistry() in tests.
func NewScraperMetrics(reg prometheus.Registerer) *ScraperMetrics {
	f := promauto.With(reg)
	return &ScraperMetrics{
		RunsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scraper_runs_total",
			Help:      "Total number of completed scrape runs",
		}, []string{"source", "tier", "result", "slot"}),
		RunDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "scraper_run_duration_seconds",
			Help:      "Scrape run duration in seconds",
			Buckets:   []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"source", "tier", "slot"}),
		EventsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scraper_events_total",
			Help:      "Total number of events processed by the scraper, by outcome",
		}, []string{"source", "tier", "outcome", "slot"}),
	}
}
