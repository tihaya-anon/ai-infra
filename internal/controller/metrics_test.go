package controller

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestControllerMetricClassificationAndRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)
	metrics.observe(time.Now(), reconcileSuccess)
	metrics.recordError(errorOperationJobSet)
	metrics.recordJobSetChange(jobSetOperationCreate)
	metrics.recordJobSetChange(jobSetChangeOperation("ignored"))
	metrics.recordStatusChange(true)

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"aijob_controller_reconciliations_total":      false,
		"aijob_controller_errors_total":               false,
		"aijob_controller_jobset_changes_total":       false,
		"aijob_controller_status_changes_total":       false,
		"aijob_controller_reconcile_duration_seconds": false,
	}
	for _, family := range families {
		if _, exists := want[family.GetName()]; exists {
			want[family.GetName()] = true
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetValue() == "ignored" {
					t.Fatal("unbounded or unsupported label was recorded")
				}
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("metric %s was not registered", name)
		}
	}
}

func TestOperationLabelIsBounded(t *testing.T) {
	if got := operationLabel("created"); got != jobSetOperationCreate {
		t.Fatalf("got %q", got)
	}
	if got := operationLabel("unexpected"); got != jobSetOperationUnchanged {
		t.Fatalf("got %q", got)
	}
}
