package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AIJobSpec describes the worker group and its preferred GPU topology.
type AIJobSpec struct {
	// Workers is the number of indexed worker Pods that must run as one workload.
	Workers int32 `json:"workers"`
	// GPUPerWorker is the extended-resource quantity requested by each worker.
	GPUPerWorker int64 `json:"gpuPerWorker"`
	// GPUResource selects the resource advertised by the cluster's GPU driver.
	GPUResource string `json:"gpuResource,omitempty"`
	// Topology requests a supported node-fabric preference or rack constraint.
	Topology string `json:"topology,omitempty"`
	// Image is the training image executed by every worker.
	Image string `json:"image,omitempty"`
	// Args is the ordered argument list passed to every worker container.
	Args []string `json:"args,omitempty"`
}

// AIJobStatus summarizes the JobSet observed by the controller.
type AIJobStatus struct {
	// ObservedGeneration identifies the AIJob spec represented by Conditions.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions are projected from the JobSet owned by this AIJob.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
	if in.Spec.Args != nil {
		out.Spec.Args = append([]string(nil), in.Spec.Args...)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
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
