package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

// AIJobSpec describes the worker group and its preferred GPU topology.
type AIJobSpec struct {
	// Workers is the number of indexed worker Pods that must run as one workload.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="workers is immutable"
	Workers int32 `json:"workers"`
	// GPUPerWorker is the extended-resource quantity requested by each worker.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpuPerWorker is immutable"
	GPUPerWorker int64 `json:"gpuPerWorker"`
	// GPUResource selects the resource advertised by the cluster's GPU driver.
	// +kubebuilder:default=example.com/gpu
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpuResource is immutable"
	GPUResource string `json:"gpuResource,omitempty"`
	// Topology requests a supported node-fabric preference or rack constraint.
	// +kubebuilder:validation:Enum=any;nvlink;pcie;same-rack
	// +kubebuilder:default=any
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="topology is immutable"
	Topology string `json:"topology,omitempty"`
	// Image is the training image executed by every worker.
	// +kubebuilder:default="ai-infra-lab:dev"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="image is immutable"
	Image string `json:"image,omitempty"`
	// Args is the ordered argument list passed to every worker container.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="args is immutable"
	Args []string `json:"args,omitempty"`

	JobSetOverrides *JobSetOverrides `json:"jobSetOverrides,omitempty"`
}

type JobSetOverrides struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="failurePolicy is immutable"
	FailurePolicy *jobsetv1alpha2.FailurePolicy `json:"failurePolicy,omitempty"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="successPolicy is immutable"
	SuccessPolicy *jobsetv1alpha2.SuccessPolicy `json:"successPolicy,omitempty"`
	Suspend       bool                          `json:"suspend,omitempty"`
}

// AIJobStatus summarizes the JobSet observed by the controller.
type AIJobStatus struct {
	// ObservedGeneration identifies the AIJob spec represented by Conditions.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions are projected from the JobSet owned by this AIJob.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AIJob is the declarative API consumed by the training-job controller.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=aijobs,singular=aijob,scope=Namespaced,shortName=aijob
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Workers",type=integer,JSONPath=".spec.workers"
// +kubebuilder:printcolumn:name="Completed",type=string,JSONPath=".status.conditions[?(@.type==\"Completed\")].status"
// +kubebuilder:printcolumn:name="Failed",type=string,JSONPath=".status.conditions[?(@.type==\"Failed\")].status"
type AIJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIJobSpec   `json:"spec"`
	Status AIJobStatus `json:"status,omitempty"`
}

// AIJobList contains a list of AIJobs.
// +kubebuilder:object:root=true
type AIJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIJob `json:"items"`
}
