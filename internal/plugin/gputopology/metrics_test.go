package gputopology

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestSchedulerMetricNormalizationAndErrorAccounting(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)
	metrics.observeScore(time.Now(), scoreSuccess, "custom-user-value", "custom-node-value")
	metrics.observeScore(time.Now(), scoreError, "nvlink", "")
	metrics.recordError(errorReasonNodeMissing)

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"aijob_scheduler_score_evaluations_total":     false,
		"aijob_scheduler_topology_observations_total": false,
		"aijob_scheduler_errors_total":                false,
		"aijob_scheduler_score_duration_seconds":      false,
	}
	for _, family := range families {
		if _, exists := want[family.GetName()]; exists {
			want[family.GetName()] = true
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetValue() == "custom-user-value" ||
					label.GetValue() == "custom-node-value" {
					t.Fatal("raw unbounded label reached the metric")
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

func TestBoundedLabelNormalization(t *testing.T) {
	if got := normalizePreference("user-input"); got != "other" {
		t.Fatalf("got preference %q", got)
	}
	if got := normalizeFabric("node-unique"); got != "other" {
		t.Fatalf("got fabric %q", got)
	}
	if got := normalizeFabric(""); got != "missing" {
		t.Fatalf("got empty fabric %q", got)
	}
}
