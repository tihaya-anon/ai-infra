package lab

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	schedulerscheme "k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
	"sigs.k8s.io/yaml"
)

func TestSchedulerProfilesDifferOnlyByScoringStrategy(t *testing.T) {
	baseline := readProfile(t, "../../deploy/scheduler-profiles/baseline.yaml")
	optimized := readProfile(t, "../../deploy/scheduler-profiles/optimized.yaml")

	baselineType := replaceStrategy(t, baseline, "PROFILE_STRATEGY")
	optimizedType := replaceStrategy(t, optimized, "PROFILE_STRATEGY")
	if baselineType != "LeastAllocated" || optimizedType != "MostAllocated" {
		t.Fatalf("unexpected strategies: baseline=%s optimized=%s", baselineType, optimizedType)
	}
	if !reflect.DeepEqual(baseline, optimized) {
		t.Fatal("Scheduler profiles differ outside the simulated-GPU scoring strategy")
	}
}

func TestCleanupCannotSelectAnotherRun(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aiv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, batchv1.AddToScheme,
		jobsetv1alpha2.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	addKueueToScheme(scheme)
	wanted := testRunAIJob("wanted", "run-a")
	unrelated := testRunAIJob("unrelated", "run-b")
	apiClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(wanted, unrelated).Build()
	runner := &BenchmarkRunner{
		cluster: &Cluster{Client: apiClient},
		options: BenchmarkOptions{Namespace: "default", Timeout: time.Second},
	}
	if err := runner.cleanup(context.Background(), "run-a"); err != nil {
		t.Fatal(err)
	}
	remaining := &aiv1alpha1.AIJobList{}
	err := apiClient.List(context.Background(), remaining, client.InNamespace("default"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].Name != "unrelated" {
		t.Fatalf("cleanup selected unrelated resources: %#v", remaining.Items)
	}
}

func TestWriteVersionedResult(t *testing.T) {
	result := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: "run-1", Timestamp: time.Now(),
		Profile: "baseline", Outcomes: map[string]string{},
	}
	if err := writeResult(t.TempDir(), "baseline", 1, result); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleDurationsUseWorkloadAndPodCreation(t *testing.T) {
	base := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		AIJobs: []aiv1alpha1.AIJob{{ObjectMeta: metav1.ObjectMeta{
			Name: "job", CreationTimestamp: metav1.NewTime(base),
		}}},
		Workloads: []Workload{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "workload", CreationTimestamp: metav1.NewTime(base.Add(10 * time.Second)),
				Labels: map[string]string{topology.JobLabel: "job"},
			},
			Status: WorkloadStatus{Conditions: []metav1.Condition{{
				Type: "Admitted", Status: metav1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(base.Add(12 * time.Second)),
			}}},
		}},
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod", CreationTimestamp: metav1.NewTime(base.Add(20 * time.Second)),
				Labels: map[string]string{topology.JobLabel: "job"},
			},
			Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(base.Add(23 * time.Second)),
			}}},
		}},
	}
	got := lifecycleFromSnapshot([]WorkloadDefinition{{Name: "job"}}, snapshot)[0]
	if got.AdmissionWaitSeconds == nil || *got.AdmissionWaitSeconds != 2 {
		t.Fatalf("got admission wait %#v, want 2 seconds", got.AdmissionWaitSeconds)
	}
	if got.SchedulingLatencySeconds == nil || *got.SchedulingLatencySeconds != 3 {
		t.Fatalf("got scheduling latency %#v, want 3 seconds", got.SchedulingLatencySeconds)
	}
}

func readProfile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	configData := result["data"].(map[string]any)["config.yaml"].(string)
	if _, _, err := schedulerscheme.Codecs.UniversalDecoder().Decode(
		[]byte(configData), nil, nil,
	); err != nil {
		t.Fatalf("strict Scheduler config decode failed: %v", err)
	}
	return result
}

func replaceStrategy(t *testing.T, profile map[string]any, replacement string) string {
	t.Helper()
	data := profile["data"].(map[string]any)
	config := map[string]any{}
	if err := yaml.Unmarshal([]byte(data["config.yaml"].(string)), &config); err != nil {
		t.Fatal(err)
	}
	profiles := config["profiles"].([]any)
	pluginConfig := profiles[0].(map[string]any)["pluginConfig"].([]any)
	args := pluginConfig[0].(map[string]any)["args"].(map[string]any)
	strategy := args["scoringStrategy"].(map[string]any)
	original := strategy["type"].(string)
	strategy["type"] = replacement
	normalized, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	data["config.yaml"] = string(normalized)
	return original
}

func testRunAIJob(name, runID string) *aiv1alpha1.AIJob {
	return &aiv1alpha1.AIJob{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "default",
		Labels: map[string]string{topology.RunIDLabel: runID},
	}}
}
