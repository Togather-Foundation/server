package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// River job metrics
var (
	// RiverJobsQueued tracks total number of jobs queued
	RiverJobsQueued = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "river_jobs_queued_total",
			Help:      "Total number of River jobs queued",
		},
		[]string{"kind", "slot"},
	)

	// RiverJobsInFlight tracks currently executing jobs
	RiverJobsInFlight = promauto.With(Registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "river_jobs_in_flight",
			Help:      "Current number of River jobs executing",
		},
		[]string{"kind", "slot"},
	)

	// RiverJobDuration tracks job execution duration
	RiverJobDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "river_job_duration_seconds",
			Help:      "River job execution duration in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300},
		},
		[]string{"kind", "slot"},
	)

	// RiverJobsCompleted tracks completed jobs by result
	RiverJobsCompleted = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "river_jobs_completed_total",
			Help:      "Total number of River jobs completed",
		},
		[]string{"kind", "slot", "result"}, // result: success, error
	)
)

// RiverMetricsHook implements River's Hook interface for Prometheus metrics
type RiverMetricsHook struct {
	river.HookDefaults
	Slot      string
	startTime map[int64]time.Time // Track job start times for duration calculation
}

// NewRiverMetricsHook creates a new metrics hook for River
func NewRiverMetricsHook(slot string) *RiverMetricsHook {
	return &RiverMetricsHook{
		Slot:      slot,
		startTime: make(map[int64]time.Time),
	}
}

// InsertBegin is called when a job is queued
func (h *RiverMetricsHook) InsertBegin(ctx context.Context, params *rivertype.JobInsertParams) error {
	if h.Slot != "" {
		RiverJobsQueued.WithLabelValues(params.Kind, h.Slot).Inc()
	}
	return nil
}

// WorkBegin is called when a job starts executing
func (h *RiverMetricsHook) WorkBegin(ctx context.Context, job *rivertype.JobRow) error {
	if h.Slot != "" {
		RiverJobsInFlight.WithLabelValues(job.Kind, h.Slot).Inc()
		h.startTime[job.ID] = time.Now()
	}
	return nil
}

// WorkEnd is called when a job finishes executing
func (h *RiverMetricsHook) WorkEnd(ctx context.Context, job *rivertype.JobRow, err error) error {
	if h.Slot == "" {
		return nil
	}

	// Decrement in-flight gauge
	RiverJobsInFlight.WithLabelValues(job.Kind, h.Slot).Dec()

	// Record duration if we tracked start time
	if startTime, ok := h.startTime[job.ID]; ok {
		duration := time.Since(startTime).Seconds()
		RiverJobDuration.WithLabelValues(job.Kind, h.Slot).Observe(duration)
		delete(h.startTime, job.ID) // Clean up
	}

	// Record completion result
	result := "success"
	if err != nil {
		result = "error"
	}
	RiverJobsCompleted.WithLabelValues(job.Kind, h.Slot, result).Inc()

	return nil
}
