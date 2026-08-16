package gputopology

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics contains bounded Scheduler plugin observations.
type Metrics struct {
	scores     *prometheus.CounterVec
	topologies *prometheus.CounterVec
	errors     *prometheus.CounterVec
	duration   *prometheus.HistogramVec
}

type scoreResult string

const (
	scoreSuccess scoreResult = "success"
	scoreError   scoreResult = "error"
)

type errorOperation string

const errorOperationScore errorOperation = "score"

type errorReason string

const errorReasonNodeMissing errorReason = "node_missing"

// NewMetrics constructs and registers one Scheduler plugin metric set.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	metrics := &Metrics{
		scores: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "scheduler", Name: "score_evaluations_total",
			Help: "Number of topology score evaluations by bounded result.",
		}, []string{"result"}),
		topologies: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "scheduler", Name: "topology_observations_total",
			Help: "Number of bounded requested-topology and observed-fabric pairs.",
		}, []string{"preference", "fabric"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aijob", Subsystem: "scheduler", Name: "errors_total",
			Help: "Number of plugin errors by bounded operation and reason.",
		}, []string{"operation", "reason"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aijob", Subsystem: "scheduler", Name: "score_duration_seconds",
			Help:    "Duration of topology score evaluations by bounded result.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
	}
	registerer.MustRegister(metrics.scores, metrics.topologies, metrics.errors, metrics.duration)
	return metrics
}

func (m *Metrics) observeScore(started time.Time, result scoreResult, preference, fabric string) {
	if m == nil {
		return
	}
	m.scores.WithLabelValues(string(result)).Inc()
	m.duration.WithLabelValues(string(result)).Observe(time.Since(started).Seconds())
	if result == scoreSuccess {
		m.topologies.WithLabelValues(
			normalizePreference(preference), normalizeFabric(fabric),
		).Inc()
	}
}

func (m *Metrics) recordError(reason errorReason) {
	if m != nil {
		m.errors.WithLabelValues(string(errorOperationScore), string(reason)).Inc()
	}
}

func normalizePreference(value string) string {
	switch value {
	case "", "any":
		return "any"
	case "nvlink", "pcie", "same-rack":
		return value
	default:
		return "other"
	}
}

func normalizeFabric(value string) string {
	switch value {
	case "":
		return "missing"
	case "nvlink", "pcie":
		return value
	default:
		return "other"
	}
}
