package controller

import (
	"testing"

	"github.com/onsi/gomega"
	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

func TestGivenAIJobWithSchedulingIntentWhenBuildingDesiredJobSetThenIntentIsCarried(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("nvlink")
	job.Labels = map[string]string{
		topology.QueueLabel: "training", topology.RunIDLabel: "run-1",
		topology.ExperimentLabel: "benchmark",
	}

	// when
	jobSet := desiredJobSet(job)
	worker := jobSet.Spec.ReplicatedJobs[0].Template.Spec
	pod := worker.Template
	gpuLimit := pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]

	// then
	assert.Expect(jobSet.Labels[topology.QueueLabel]).To(gomega.Equal("training"))
	assert.Expect(worker.Parallelism).NotTo(gomega.BeNil())
	assert.Expect(*worker.Parallelism).To(gomega.Equal(int32(4)))
	assert.Expect(pod.Spec.SchedulerName).To(gomega.Equal(SchedulerName))
	assert.Expect(pod.Annotations[topology.PreferenceAnnotation]).To(gomega.Equal("nvlink"))
	assert.Expect(gpuLimit.Value()).To(gomega.Equal(int64(2)))
	assert.Expect(jobSet.Labels[topology.RunIDLabel]).To(gomega.Equal("run-1"))
	assert.Expect(pod.Labels[topology.ExperimentLabel]).To(gomega.Equal("benchmark"))
}

func TestGivenAIJobArgsWhenBuildingWorkerContainerThenArgsAndCompletionIdentityAreSet(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("any")
	job.Spec.Args = []string{"--mode=complete", "--duration=2s", "--fail-indexes=1,3"}
	wantArgs := []string{"--mode=complete", "--duration=2s", "--fail-indexes=1,3"}

	// when
	container := workerContainer(job)
	fieldRef := container.Env[0].ValueFrom.FieldRef

	// then
	assert.Expect(container.Args).To(gomega.Equal(wantArgs))
	assert.Expect(container.Env).To(gomega.HaveLen(1))
	assert.Expect(container.Env[0].Name).To(gomega.Equal("JOB_COMPLETION_INDEX"))
	assert.Expect(fieldRef).NotTo(gomega.BeNil())
	assert.Expect(fieldRef.FieldPath).To(
		gomega.Equal("metadata.labels['batch.kubernetes.io/job-completion-index']"),
	)

	// when
	container.Args[0] = "changed"

	// then
	assert.Expect(job.Spec.Args[0]).To(gomega.Equal("--mode=complete"))
}

func TestGivenNoAIJobArgsWhenBuildingWorkerContainerThenArgsRemainUnset(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("any")

	// when
	container := workerContainer(job)

	// then
	assert.Expect(container.Args).To(gomega.BeNil())
	assert.Expect(job.Spec.Args).To(gomega.BeNil())
}

func TestGivenSameRackTopologyWhenBuildingDesiredJobSetThenKueueTopologyIsRequired(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("same-rack")

	// when
	jobSet := desiredJobSet(job)
	annotations := jobSet.Spec.ReplicatedJobs[0].Template.Spec.Template.Annotations

	// then
	assert.Expect(job.Spec.Topology).To(gomega.Equal("same-rack"))
	assert.Expect(annotations[topology.RequiredTopologyAnnotation]).To(gomega.Equal(topology.RackLabel))
	assert.Expect(annotations).NotTo(gomega.HaveKey(topology.PreferenceAnnotation))
}

func TestGivenJobSetOverridesWhenBuildingDesiredJobSetThenPoliciesAreCopied(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("any")
	job.Spec.JobSetOverrides = &aiv1alpha1.JobSetOverrides{
		FailurePolicy: &jobsetv1alpha2.FailurePolicy{
			MaxRestarts: 2,
			Rules: []jobsetv1alpha2.FailurePolicyRule{{
				Name:   "retry-workers",
				Action: jobsetv1alpha2.RestartJobSet,
			}},
		},
		SuccessPolicy: &jobsetv1alpha2.SuccessPolicy{
			Operator: jobsetv1alpha2.OperatorAll,
		},
	}

	// when
	jobSet := desiredJobSet(job)

	// then
	assert.Expect(jobSet.Spec.FailurePolicy).To(gomega.Equal(job.Spec.JobSetOverrides.FailurePolicy))
	assert.Expect(jobSet.Spec.SuccessPolicy).To(gomega.Equal(job.Spec.JobSetOverrides.SuccessPolicy))
}

func TestGivenNoJobSetOverridesWhenBuildingDesiredJobSetThenPoliciesRemainUnset(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	job := testAIJob("any")

	// when
	jobSet := desiredJobSet(job)

	// then
	assert.Expect(jobSet.Spec.FailurePolicy).To(gomega.BeNil())
	assert.Expect(jobSet.Spec.SuccessPolicy).To(gomega.BeNil())
}

func TestGivenDesiredPoliciesWhenCreatingOwnedJobSetThenImmutablePoliciesAreInitialized(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	desired := desiredJobSet(testAIJob("any"))
	desired.Spec.FailurePolicy = &jobsetv1alpha2.FailurePolicy{MaxRestarts: 1}
	desired.Spec.SuccessPolicy = &jobsetv1alpha2.SuccessPolicy{Operator: jobsetv1alpha2.OperatorAll}
	actual := &jobsetv1alpha2.JobSet{}

	// when
	reconcileOwnedFields(actual, desired)

	// then
	assert.Expect(actual.Spec.FailurePolicy).To(gomega.Equal(desired.Spec.FailurePolicy))
	assert.Expect(actual.Spec.SuccessPolicy).To(gomega.Equal(desired.Spec.SuccessPolicy))
}

func TestGivenExistingLabelsWhenReconcilingOwnedFieldsThenExternalLabelsArePreserved(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	actual := &jobsetv1alpha2.JobSet{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		"jobset.x-k8s.io/internal": "keep",
		topology.QueueLabel:        "old-queue",
	}}}
	desired := desiredJobSet(testAIJob("any"))

	// when
	reconcileOwnedFields(actual, desired)

	// then
	assert.Expect(actual.Labels["jobset.x-k8s.io/internal"]).To(gomega.Equal("keep"))
	assert.Expect(actual.Labels).NotTo(gomega.HaveKey(topology.QueueLabel))
}

func TestGivenExistingJobSetWhenReconcilingOwnedFieldsThenDefaultedSpecIsPreserved(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	created := metav1.Now()
	actual := &jobsetv1alpha2.JobSet{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: created},
		Spec: jobsetv1alpha2.JobSetSpec{
			Network: &jobsetv1alpha2.Network{
				EnableDNSHostnames:       boolPtr(true),
				PublishNotReadyAddresses: boolPtr(true),
			},
		},
	}
	desired := desiredJobSet(testAIJob("nvlink"))

	// when
	reconcileOwnedFields(actual, desired)

	// then
	assert.Expect(actual.Spec.Network.PublishNotReadyAddresses).NotTo(gomega.BeNil())
	assert.Expect(*actual.Spec.Network.PublishNotReadyAddresses).To(gomega.BeTrue())
	assert.Expect(actual.Spec.ReplicatedJobs).To(gomega.BeNil())
	assert.Expect(actual.Spec.FailurePolicy).To(gomega.BeNil())
	assert.Expect(actual.Spec.SuccessPolicy).To(gomega.BeNil())
}

func TestGivenJobSetConditionsWhenProjectingStatusThenConditionsUseAIJobGeneration(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	jobSet := &jobsetv1alpha2.JobSet{Status: jobsetv1alpha2.JobSetStatus{
		ReplicatedJobsStatus: []jobsetv1alpha2.ReplicatedJobStatus{{Ready: 2, Active: 2, Succeeded: 1}},
		Conditions:           []metav1.Condition{{Type: "Completed", Status: metav1.ConditionFalse}},
	}}

	// when
	status := statusFromJobSet(7, jobSet)

	// then
	assert.Expect(status.ObservedGeneration).To(gomega.Equal(int64(7)))
	assert.Expect(status.Conditions).To(gomega.HaveLen(1))
	assert.Expect(status.Conditions[0].ObservedGeneration).To(gomega.Equal(int64(7)))
}

func TestGivenJobSetWithoutConditionsWhenProjectingStatusThenConditionsRemainUnset(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	jobSet := &jobsetv1alpha2.JobSet{}

	// when
	status := statusFromJobSet(3, jobSet)

	// then
	assert.Expect(jobSet.Status.Conditions).To(gomega.BeNil())
	assert.Expect(status.Conditions).To(gomega.BeNil())
}

func testAIJob(preference string) *aiv1alpha1.AIJob {
	return &aiv1alpha1.AIJob{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "default"},
		Spec: aiv1alpha1.AIJobSpec{
			Workers: 4, GPUPerWorker: 2, GPUResource: "nvidia.com/gpu",
			Topology: preference, Image: "training:v1",
		},
	}
}
