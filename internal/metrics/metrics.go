package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Namespace for all Togather metrics
const namespace = "togather"

// Registry is the global Prometheus registry for all metrics
var Registry = prometheus.NewRegistry()

// AppInfo is a gauge that exposes application version information as labels
var AppInfo = promauto.With(Registry).NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "app_info",
		Help:      "Application version information (always set to 1, version info in labels)",
	},
	[]string{"version", "commit", "build_date", "active_slot"},
)

// HealthStatus is a gauge that tracks overall server health status
// Values: 0 = unhealthy, 1 = degraded, 2 = healthy
var HealthStatus = promauto.With(Registry).NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "health_status",
		Help:      "Overall server health status (0=unhealthy, 1=degraded, 2=healthy)",
	},
	[]string{"slot"},
)

// HealthCheckStatus tracks individual health check results
// Values: 0 = fail, 1 = warn, 2 = pass
var HealthCheckStatus = promauto.With(Registry).NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "health_check_status",
		Help:      "Individual health check status (0=fail, 1=warn, 2=pass)",
	},
	[]string{"check", "slot"},
)

// HealthCheckLatency tracks the latency of individual health checks in milliseconds
var HealthCheckLatency = promauto.With(Registry).NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "health_check_latency_ms",
		Help:      "Health check latency in milliseconds",
	},
	[]string{"check", "slot"},
)

// IdempotencyKeysDeleted tracks the total number of expired idempotency keys deleted
var IdempotencyKeysDeleted = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "idempotency_keys_deleted_total",
		Help:      "Total number of expired idempotency keys deleted by cleanup job",
	},
	[]string{"slot"},
)

// IdempotencyCleanupDuration tracks the duration of idempotency cleanup operations
var IdempotencyCleanupDuration = promauto.With(Registry).NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "idempotency_cleanup_duration_seconds",
		Help:      "Duration of idempotency cleanup job execution in seconds",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
	},
	[]string{"slot"},
)

// IdempotencyCleanupErrors tracks cleanup job failures
var IdempotencyCleanupErrors = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "idempotency_cleanup_errors_total",
		Help:      "Total number of idempotency cleanup job failures",
	},
	[]string{"slot", "error_type"},
)

// IdempotencyKeysTableSize tracks the current size of the idempotency_keys table (optional)
var IdempotencyKeysTableSize = promauto.With(Registry).NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "idempotency_keys_table_size",
		Help:      "Current number of rows in the idempotency_keys table",
	},
	[]string{"slot"},
)

// Geocoding metrics

// GeocodingRequestsTotal tracks total geocoding requests by type and source
var GeocodingRequestsTotal = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "geocoding_requests_total",
		Help:      "Total number of geocoding requests",
	},
	[]string{"type", "source"}, // type: forward|reverse, source: cache|nominatim
)

// GeocodingCacheHitsTotal tracks successful cache hits
var GeocodingCacheHitsTotal = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "geocoding_cache_hits_total",
		Help:      "Total number of geocoding cache hits",
	},
	[]string{"cache_type"}, // cache_type: forward|reverse
)

// GeocodingCacheMissesTotal tracks cache misses requiring API calls
var GeocodingCacheMissesTotal = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "geocoding_cache_misses_total",
		Help:      "Total number of geocoding cache misses",
	},
	[]string{"cache_type"}, // cache_type: forward|reverse
)

// GeocodingNominatimRequestsTotal tracks Nominatim API requests by endpoint and status
var GeocodingNominatimRequestsTotal = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "geocoding_nominatim_requests_total",
		Help:      "Total number of Nominatim API requests",
	},
	[]string{"endpoint", "status"}, // endpoint: search|reverse, status: success|error
)

// GeocodingNominatimLatency tracks Nominatim API request latency
var GeocodingNominatimLatency = promauto.With(Registry).NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "geocoding_nominatim_latency_seconds",
		Help:      "Nominatim API request latency in seconds",
		Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	},
	[]string{"endpoint"}, // endpoint: search|reverse
)

// GeocodingFailuresTotal tracks failed geocoding attempts by type and reason
var GeocodingFailuresTotal = promauto.With(Registry).NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "geocoding_failures_total",
		Help:      "Total number of failed geocoding attempts",
	},
	[]string{"type", "reason"}, // type: forward|reverse, reason: timeout|rate_limit|not_found|error
)

// Init initializes the metrics registry and sets version information
func Init(version, commit, buildDate, activeSlot string) {
	// Register default Go metrics (memory, goroutines, GC, etc.)
	Registry.MustRegister(collectors.NewGoCollector())

	// Register process metrics (CPU, memory, file descriptors)
	Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Set application version info (value is always 1, info is in labels)
	// activeSlot will be "true" for the active slot, "false" for inactive, or "unknown" if not set
	AppInfo.WithLabelValues(version, commit, buildDate, activeSlot).Set(1)
}
