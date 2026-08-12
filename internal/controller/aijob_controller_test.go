package controller

import (
	"testing"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkerPodCarriesSchedulingIntent(t *testing.T) {
	job := &aiv1alpha1.AIJob{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "default"},
		Spec: aiv1alpha1.AIJobSpec{
			GPUPerWorker: 2,
			GPUResource:  "nvidia.com/gpu",
			Topology:     "nvlink",
			Image:        "training:v1",
		},
	}

	pod := workerPod(job, "training-worker-0")
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
