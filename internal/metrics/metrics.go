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
