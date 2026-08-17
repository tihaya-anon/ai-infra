package lab

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/onsi/gomega"
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

func TestGivenSchedulerProfilesWhenNormalizingStrategyThenOnlyScoringStrategyDiffers(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	baseline := readProfile(t, "../../deploy/scheduler-profiles/baseline.yaml")
	optimized := readProfile(t, "../../deploy/scheduler-profiles/optimized.yaml")

	// when
	baselineType := replaceStrategy(t, baseline, "PROFILE_STRATEGY")
	optimizedType := replaceStrategy(t, optimized, "PROFILE_STRATEGY")

	// then
	assert.Expect(baselineType).To(gomega.Equal("LeastAllocated"))
	assert.Expect(optimizedType).To(gomega.Equal("MostAllocated"))
	assert.Expect(baseline).To(gomega.Equal(optimized))
	assertControlledScorePlugins(t, baseline)
}

func assertControlledScorePlugins(t *testing.T, profile map[string]any) {
	t.Helper()
	assert := gomega.NewWithT(t)
	data := profile["data"].(map[string]any)
	config := map[string]any{}
	assert.Expect(yaml.Unmarshal([]byte(data["config.yaml"].(string)), &config)).To(gomega.Succeed())
	profiles := config["profiles"].([]any)
	plugins := profiles[0].(map[string]any)["plugins"].(map[string]any)
	score := plugins["score"].(map[string]any)

	assert.Expect(score["disabled"]).To(gomega.Equal([]any{map[string]any{"name": "*"}}))
	assert.Expect(score["enabled"]).To(gomega.Equal([]any{
		map[string]any{"name": "NodeResourcesFit", "weight": float64(10)},
		map[string]any{"name": "GPUTopology", "weight": float64(5)},
	}))
}

func TestGivenResourcesFromMultipleRunsWhenCleaningUpThenOnlySelectedRunIsRemoved(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	scheme := runtime.NewScheme()
	assert.Expect(aiv1alpha1.AddToScheme(scheme)).To(gomega.Succeed())
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, batchv1.AddToScheme,
		jobsetv1alpha2.AddToScheme,
	} {
		assert.Expect(add(scheme)).To(gomega.Succeed())
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

	// when
	err := runner.cleanup(context.Background(), "run-a")
	remaining := &aiv1alpha1.AIJobList{}
	listError := apiClient.List(context.Background(), remaining, client.InNamespace("default"))

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(listError).NotTo(gomega.HaveOccurred())
	assert.Expect(remaining.Items).To(gomega.HaveLen(1))
	assert.Expect(remaining.Items[0].Name).To(gomega.Equal("unrelated"))
}

func TestGivenVersionedResultWhenWritingThenFileIsCreated(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	result := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: "run-1", Timestamp: time.Now(),
		Profile: "baseline", Outcomes: map[string]string{},
	}

	// when
	err := writeResult(t.TempDir(), "baseline", 1, result)

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
}

func TestGivenLifecycleTransitionsWhenCalculatingThenDurationsUseCreationTimes(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
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

	// when
	got := lifecycleFromSnapshot([]WorkloadDefinition{{Name: "job"}}, snapshot)[0]

	// then
	assert.Expect(got.AdmissionWaitSeconds).NotTo(gomega.BeNil())
	assert.Expect(*got.AdmissionWaitSeconds).To(gomega.Equal(float64(2)))
	assert.Expect(got.SchedulingLatencySeconds).NotTo(gomega.BeNil())
	assert.Expect(*got.SchedulingLatencySeconds).To(gomega.Equal(float64(3)))
}

func TestGivenJobSetOwnerWhenSelectingWorkloadsThenRelatedWorkloadIsReturned(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	workloads := []Workload{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "wanted",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "jobset.x-k8s.io/v1alpha2",
					Kind:       "JobSet",
					Name:       "job-a",
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "unrelated"},
		},
	}
	jobSets := []jobsetv1alpha2.JobSet{{ObjectMeta: metav1.ObjectMeta{Name: "job-a"}}}

	// when
	got := relatedWorkloads(workloads, nil, jobSets)

	// then
	assert.Expect(got).To(gomega.HaveLen(1))
	assert.Expect(got[0].Name).To(gomega.Equal("wanted"))
}

func TestGivenExerciseKindsWhenGeneratingWorkloadNamesThenJobSetPodNamesFitLimits(t *testing.T) {
	// given
	kinds := []string{ExerciseCapacity, ExerciseWorkerFailure, ExerciseControllerRestart}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			assert := gomega.NewWithT(t)

			// when
			name := exerciseWorkloadName(kind, newRunID(exerciseRunPrefix(kind)))
			replicatedJobName := name + "-workers-0"

			// then
			assert.Expect(len(replicatedJobName)).To(gomega.BeNumerically("<=", 50))
		})
	}
}

func TestGivenE2EScenariosWhenGeneratingWorkloadNamesThenJobSetPodNamesFitLimits(t *testing.T) {
	tests := []struct {
		scenario       string
		workloadPrefix string
	}{
		{scenario: "success", workloadPrefix: "success-"},
		{scenario: "selected-worker-failure", workloadPrefix: "failure-"},
		{scenario: "controller-restart-cleanup", workloadPrefix: "restart-"},
	}
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			assert := gomega.NewWithT(t)
			runID := newRunID(e2eRunPrefix(test.scenario))
			replicatedJobName := test.workloadPrefix + runID + "-workers-0"

			assert.Expect(len(replicatedJobName)).To(gomega.BeNumerically("<=", 50))
		})
	}
}

func TestGivenCapacityRunIDWhenGeneratingWorkloadNameThenNameRemainsReadable(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	runID := "ex-cap-1786855813-5a8fc2fe"

	// when
	name := exerciseWorkloadName(ExerciseCapacity, runID)

	// then
	assert.Expect(name).To(gomega.Equal("cap-" + runID))
}

func readProfile(t *testing.T, path string) map[string]any {
	t.Helper()
	assert := gomega.NewWithT(t)
	data, err := os.ReadFile(path)
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	result := map[string]any{}
	assert.Expect(yaml.Unmarshal(data, &result)).To(gomega.Succeed())
	configData := result["data"].(map[string]any)["config.yaml"].(string)
	_, _, err = schedulerscheme.Codecs.UniversalDecoder().Decode(
		[]byte(configData), nil, nil,
	)
	assert.Expect(err).NotTo(gomega.HaveOccurred(), "strict Scheduler config decode failed")
	return result
}

func replaceStrategy(t *testing.T, profile map[string]any, replacement string) string {
	t.Helper()
	assert := gomega.NewWithT(t)
	data := profile["data"].(map[string]any)
	config := map[string]any{}
	assert.Expect(yaml.Unmarshal([]byte(data["config.yaml"].(string)), &config)).To(gomega.Succeed())
	profiles := config["profiles"].([]any)
	pluginConfig := profiles[0].(map[string]any)["pluginConfig"].([]any)
	args := pluginConfig[0].(map[string]any)["args"].(map[string]any)
	strategy := args["scoringStrategy"].(map[string]any)
	original := strategy["type"].(string)
	strategy["type"] = replacement
	normalized, err := yaml.Marshal(config)
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	data["config.yaml"] = string(normalized)
	return original
}

func testRunAIJob(name, runID string) *aiv1alpha1.AIJob {
	return &aiv1alpha1.AIJob{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "default",
		Labels: map[string]string{topology.RunIDLabel: runID},
	}}
}
