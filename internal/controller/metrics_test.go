package controller

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestGivenControllerActivityWhenGatheringMetricsThenMetricsAreBounded(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)

	// when
	metrics.observe(time.Now(), reconcileSuccess)
	metrics.recordError(errorOperationJobSet)
	metrics.recordJobSetChange(jobSetOperationCreate)
	metrics.recordJobSetChange(jobSetChangeOperation("ignored"))
	metrics.recordStatusChange(true)

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
		"aijob_controller_reconciliations_total",
		"aijob_controller_errors_total",
		"aijob_controller_jobset_changes_total",
		"aijob_controller_status_changes_total",
		"aijob_controller_reconcile_duration_seconds",
	))
	assert.Expect(labelValues).NotTo(gomega.ContainElement("ignored"))
}

func TestGivenOperationValuesWhenNormalizingLabelsThenValuesAreBounded(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	knownOperation := controllerutil.OperationResultCreated
	unknownOperation := controllerutil.OperationResult("unexpected")

	// when
	knownLabel := operationLabel(knownOperation)
	unknownLabel := operationLabel(unknownOperation)

	// then
	assert.Expect(knownLabel).To(gomega.Equal(jobSetOperationCreate))
	assert.Expect(unknownLabel).To(gomega.Equal(jobSetOperationUnchanged))
}
