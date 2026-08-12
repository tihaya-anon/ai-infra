package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AIJobSpec describes the worker group and its preferred GPU topology.
type AIJobSpec struct {
	Workers      int32  `json:"workers"`
	GPUPerWorker int64  `json:"gpuPerWorker"`
	GPUResource  string `json:"gpuResource,omitempty"`
	Topology     string `json:"topology,omitempty"`
	Image        string `json:"image,omitempty"`
}

// AIJobStatus summarizes the current worker Pod phases.
type AIJobStatus struct {
	Pending   int32 `json:"pending,omitempty"`
	Running   int32 `json:"running,omitempty"`
	Succeeded int32 `json:"succeeded,omitempty"`
	Failed    int32 `json:"failed,omitempty"`
}

// AIJob is the declarative API consumed by the training-job controller.
type AIJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIJobSpec   `json:"spec,omitempty"`
	Status AIJobStatus `json:"status,omitempty"`
}

// AIJobList contains a list of AIJobs.
type AIJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIJob `json:"items"`
}

func (in *AIJob) DeepCopyInto(out *AIJob) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
}

func (in *AIJob) DeepCopy() *AIJob {
	if in == nil {
		return nil
	}
	out := new(AIJob)
	in.DeepCopyInto(out)
	return out
}

func (in *AIJob) DeepCopyObject() runtime.Object { return in.DeepCopy() }

func (in *AIJobList) DeepCopyInto(out *AIJobList) {
	*out = *in
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AIJob, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AIJobList) DeepCopy() *AIJobList {
	if in == nil {
		return nil
	}
	out := new(AIJobList)
	in.DeepCopyInto(out)
	return out
}

func (in *AIJobList) DeepCopyObject() runtime.Object { return in.DeepCopy() }

// WorkerStatus reduces Pod phases to the API status exposed to users.
func WorkerStatus(pods []corev1.Pod) AIJobStatus {
	status := AIJobStatus{}
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			status.Running++
		case corev1.PodSucceeded:
			status.Succeeded++
		case corev1.PodFailed:
			status.Failed++
		default:
			status.Pending++
		}
	}
	return status
}
