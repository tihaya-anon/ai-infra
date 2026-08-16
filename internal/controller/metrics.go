package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics contains the bounded Controller observations used by the lab.
type Metrics struct {
	reconciliations *prometheus.CounterVec
	errors          *prometheus.CounterVec
	jobSetChanges   *prometheus.CounterVec
	statusChanges   *prometheus.CounterVec
	duration        *prometheus.HistogramVec
}

type errorOperation string

const (
	errorOperationGet    errorOperation = "get"
	errorOperationJobSet errorOperation = "jobset"
	errorOperationStatus errorOperation = "status"
)

// NewMetrics constructs and registers one Controller metric set.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	metrics := &Metrics{
		reconciliations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "controller", Name: "reconciliations_total",
			Help: "Number of AIJob reconciliation attempts by bounded result.",
		}, []string{"result"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "controller", Name: "errors_total",
			Help: "Number of AIJob reconciliation errors by operation.",
		}, []string{"operation"}),
		jobSetChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "controller", Name: "jobset_changes_total",
			Help: "Number of owned JobSet changes by operation.",
		}, []string{"operation"}),
		statusChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "controller", Name: "status_changes_total",
			Help: "Number of AIJob status updates.",
		}, []string{"result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aijob", Subsystem: "controller", Name: "reconcile_duration_seconds",
			Help:    "Duration of AIJob reconciliations by bounded result.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
	}
	registerer.MustRegister(
		metrics.reconciliations, metrics.errors, metrics.jobSetChanges,
		metrics.statusChanges, metrics.duration,
	)
	return metrics
}

func (m *Metrics) observe(start time.Time, result string) {
	if m == nil {
		return
	}
	m.reconciliations.WithLabelValues(result).Inc()
	m.duration.WithLabelValues(result).Observe(time.Since(start).Seconds())
}

func (m *Metrics) recordError(operation errorOperation) {
	if m != nil {
		m.errors.WithLabelValues(string(operation)).Inc()
	}
}

func (m *Metrics) recordJobSetChange(operation string) {
	if m != nil && (operation == "create" || operation == "update") {
		m.jobSetChanges.WithLabelValues(operation).Inc()
	}
}

func (m *Metrics) recordStatusChange(changed bool) {
	if m == nil {
		return
	}
	result := "unchanged"
	if changed {
		result = "updated"
	}
	m.statusChanges.WithLabelValues(result).Inc()
}
