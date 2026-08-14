package controller

import (
	"testing"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

func TestDesiredJobSetCarriesSchedulingIntent(t *testing.T) {
	job := testAIJob("nvlink")
	job.Labels = map[string]string{topology.QueueLabel: "training"}

	jobSet := desiredJobSet(job)
	worker := jobSet.Spec.ReplicatedJobs[0].Template.Spec
	pod := worker.Template

	if got := jobSet.Labels[topology.QueueLabel]; got != "training" {
		t.Fatalf("got queue %q, want training", got)
	}
	if worker.Parallelism == nil || *worker.Parallelism != 4 {
		t.Fatalf("got parallelism %v, want 4", worker.Parallelism)
	}
	if pod.Spec.SchedulerName != SchedulerName {
		t.Fatalf("got scheduler %q, want %q", pod.Spec.SchedulerName, SchedulerName)
	}
	if got := pod.Annotations[topology.PreferenceAnnotation]; got != "nvlink" {
		t.Fatalf("got topology %q, want nvlink", got)
	}
	if got := pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]; got.Value() != 2 {
		t.Fatalf("got GPU limit %s, want 2", got.String())
	}
}

func TestSameRackUsesKueueTopology(t *testing.T) {
	jobSet := desiredJobSet(testAIJob("same-rack"))
	annotations := jobSet.Spec.ReplicatedJobs[0].Template.Spec.Template.Annotations

	if got := annotations[topology.RequiredTopologyAnnotation]; got != topology.RackLabel {
		t.Fatalf("got required topology %q, want %q", got, topology.RackLabel)
	}
	if _, exists := annotations[topology.PreferenceAnnotation]; exists {
		t.Fatal("same-rack must be handled by Kueue, not the node score plugin")
	}
}

func TestReconcileOwnedFieldsPreservesOtherControllersFields(t *testing.T) {
	actual := &jobsetv1alpha2.JobSet{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		"jobset.x-k8s.io/internal": "keep",
		topology.QueueLabel:        "old-queue",
	}}}
	desired := desiredJobSet(testAIJob("any"))

	reconcileOwnedFields(actual, desired)

	if got := actual.Labels["jobset.x-k8s.io/internal"]; got != "keep" {
		t.Fatalf("got external label %q, want keep", got)
	}
	if _, exists := actual.Labels[topology.QueueLabel]; exists {
		t.Fatal("queue label should be removed when AIJob no longer selects a queue")
	}
}

func TestReconcileOwnedFieldsDoesNotOverwriteDefaultedSpec(t *testing.T) {
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

	reconcileOwnedFields(actual, desired)

	if actual.Spec.Network.PublishNotReadyAddresses == nil ||
		!*actual.Spec.Network.PublishNotReadyAddresses {
		t.Fatal("webhook-defaulted network fields must be preserved after creation")
	}
	if actual.Spec.ReplicatedJobs != nil {
		t.Fatal("existing spec must not be replaced during reconciliation")
	}
}

func TestStatusFromJobSet(t *testing.T) {
	jobSet := &jobsetv1alpha2.JobSet{Status: jobsetv1alpha2.JobSetStatus{
		ReplicatedJobsStatus: []jobsetv1alpha2.ReplicatedJobStatus{{Ready: 2, Active: 2, Succeeded: 1}},
		Conditions:           []metav1.Condition{{Type: "Completed", Status: metav1.ConditionFalse}},
	}}

	status := statusFromJobSet(7, jobSet)
	if status.ObservedGeneration != 7 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if got := status.Conditions[0].ObservedGeneration; got != 7 {
		t.Fatalf("got condition generation %d, want 7", got)
	}
}

func TestStatusFromJobSetKeepsEmptyConditionsNil(t *testing.T) {
	status := statusFromJobSet(3, &jobsetv1alpha2.JobSet{})
	if status.Conditions != nil {
		t.Fatalf("got conditions %#v, want nil", status.Conditions)
	}
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
