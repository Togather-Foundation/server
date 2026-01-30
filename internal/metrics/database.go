package metrics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Database metrics
var (
	// DBConnectionsOpen is the total number of open connections to the database
	DBConnectionsOpen = promauto.With(Registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_open",
			Help:      "Total number of open database connections",
		},
	)

	// DBConnectionsInUse is the number of database connections currently in use
	DBConnectionsInUse = promauto.With(Registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_in_use",
			Help:      "Number of database connections currently in use (acquired)",
		},
	)

	// DBConnectionsIdle is the number of idle database connections
	DBConnectionsIdle = promauto.With(Registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_idle",
			Help:      "Number of idle database connections",
		},
	)

	// DBConnectionsMaxOpen is the maximum number of open database connections
	DBConnectionsMaxOpen = promauto.With(Registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_max_open",
			Help:      "Maximum number of open database connections allowed",
		},
	)

	// DBQueryDuration records database query latency
	DBQueryDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
			// Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"operation"},
	)

	// DBErrors counts database errors by type
	DBErrors = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_errors_total",
			Help:      "Total number of database errors",
		},
		[]string{"operation", "error_type"},
	)
)

// DBCollector periodically collects database pool statistics
type DBCollector struct {
	pool     *pgxpool.Pool
	stopChan chan struct{}
}

// NewDBCollector creates a new database metrics collector
func NewDBCollector(pool *pgxpool.Pool) *DBCollector {
	return &DBCollector{
		pool:     pool,
		stopChan: make(chan struct{}),
	}
}

// Start begins collecting database metrics at the specified interval
func (c *DBCollector) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	c.collect()

	for {
		select {
		case <-ticker.C:
			c.collect()
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop stops the metrics collector
func (c *DBCollector) Stop() {
	close(c.stopChan)
}

// collect gathers current pool statistics and updates metrics
func (c *DBCollector) collect() {
	if c.pool == nil {
		return
	}

	stat := c.pool.Stat()

	// Update gauge metrics
	DBConnectionsOpen.Set(float64(stat.TotalConns()))
	DBConnectionsInUse.Set(float64(stat.AcquiredConns()))
	DBConnectionsIdle.Set(float64(stat.IdleConns()))
	DBConnectionsMaxOpen.Set(float64(stat.MaxConns()))
}

// RecordQuery records metrics for a database query
// Call this function with defer to capture duration:
//
//	start := time.Now()
//	defer metrics.RecordQuery("select_events", start, err)
func RecordQuery(operation string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	DBQueryDuration.WithLabelValues(operation).Observe(duration)

	if err != nil {
		// Classify error type (simplified - can be expanded based on pgx error types)
		errorType := "query_error"
		if err == context.Canceled {
			errorType = "canceled"
		} else if err == context.DeadlineExceeded {
			errorType = "timeout"
		}
		DBErrors.WithLabelValues(operation, errorType).Inc()
	}
}
