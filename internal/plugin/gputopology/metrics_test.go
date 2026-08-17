package gputopology

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

func TestGivenSchedulerActivityWhenGatheringMetricsThenLabelsAreBounded(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)

	// when
	metrics.observeScore(time.Now(), scoreSuccess, "custom-user-value", "custom-node-value")
	metrics.observeScore(time.Now(), scoreError, "nvlink", "")
	metrics.recordError(errorReasonNodeMissing)

	families, err := registry.Gather()
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	metricNames := make([]string, 0, len(families))
	labelValues := []string{}
	for _, family := range families {
		metricNames = append(metricNames, family.GetName())
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				labelValues = append(labelValues, label.GetValue())
			}
		}
	}

	// then
	assert.Expect(metricNames).To(gomega.ContainElements(
		"aijob_scheduler_score_evaluations_total",
		"aijob_scheduler_topology_observations_total",
		"aijob_scheduler_errors_total",
		"aijob_scheduler_score_duration_seconds",
	))
	assert.Expect(labelValues).NotTo(gomega.ContainElement(gomega.Or(
		gomega.Equal("custom-user-value"),
		gomega.Equal("custom-node-value"),
	)))
}

func TestGivenUnboundedTopologyValuesWhenNormalizingThenLabelsUseBoundedCategories(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	preference := "user-input"
	fabric := "node-unique"

	// when
	normalizedPreference := normalizePreference(preference)
	normalizedFabric := normalizeFabric(fabric)
	missingFabric := normalizeFabric("")

	// then
	assert.Expect(normalizedPreference).To(gomega.Equal("other"))
	assert.Expect(normalizedFabric).To(gomega.Equal("other"))
	assert.Expect(missingFabric).To(gomega.Equal("missing"))
}
