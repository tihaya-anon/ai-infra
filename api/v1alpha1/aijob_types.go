package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
// +kubebuilder:object:root=true
type AIJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIJobSpec   `json:"spec,omitempty"`
	Status AIJobStatus `json:"status,omitempty"`
}

// AIJobList contains a list of AIJobs.
// +kubebuilder:object:root=true
type AIJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIJob `json:"items"`
}
