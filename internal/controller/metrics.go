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

type reconcileResult string

const (
	reconcileSuccess  reconcileResult = "success"
	reconcileError    reconcileResult = "error"
	reconcileNotFound reconcileResult = "not_found"
)

type errorOperation string

const (
	errorOperationGet    errorOperation = "get"
	errorOperationJobSet errorOperation = "jobset"
	errorOperationStatus errorOperation = "status"
)

type jobSetChangeOperation string

const (
	jobSetOperationCreate    jobSetChangeOperation = "create"
	jobSetOperationUpdate    jobSetChangeOperation = "update"
	jobSetOperationUnchanged jobSetChangeOperation = "unchanged"
)

type statusChangeResult string

const (
	statusChangeUpdated   statusChangeResult = "updated"
	statusChangeUnchanged statusChangeResult = "unchanged"
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

func (m *Metrics) observe(start time.Time, result reconcileResult) {
	if m == nil {
		return
	}
	m.reconciliations.WithLabelValues(string(result)).Inc()
	m.duration.WithLabelValues(string(result)).Observe(time.Since(start).Seconds())
}

func (m *Metrics) recordError(operation errorOperation) {
	if m != nil {
		m.errors.WithLabelValues(string(operation)).Inc()
	}
}

func (m *Metrics) recordJobSetChange(operation jobSetChangeOperation) {
	if m != nil && (operation == jobSetOperationCreate || operation == jobSetOperationUpdate) {
		m.jobSetChanges.WithLabelValues(string(operation)).Inc()
	}
}

func (m *Metrics) recordStatusChange(changed bool) {
	if m == nil {
		return
	}
	result := statusChangeUnchanged
	if changed {
		result = statusChangeUpdated
	}
	m.statusChanges.WithLabelValues(string(result)).Inc()
}
