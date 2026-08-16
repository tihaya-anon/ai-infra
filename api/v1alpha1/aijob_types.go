package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AIJobSpec describes the worker group and its preferred GPU topology.
type AIJobSpec struct {
	// Workers is the number of indexed worker Pods that must run as one workload.
	// +kubebuilder:validation:Minimum=1
	Workers int32 `json:"workers"`
	// GPUPerWorker is the extended-resource quantity requested by each worker.
	// +kubebuilder:validation:Minimum=1
	GPUPerWorker int64 `json:"gpuPerWorker"`
	// GPUResource selects the resource advertised by the cluster's GPU driver.
	// +kubebuilder:default=example.com/gpu
	GPUResource string `json:"gpuResource,omitempty"`
	// Topology requests a supported node-fabric preference or rack constraint.
	// +kubebuilder:validation:Enum=any;nvlink;pcie;same-rack
	// +kubebuilder:default=any
	Topology string `json:"topology,omitempty"`
	// Image is the training image executed by every worker.
	// +kubebuilder:default="ai-infra-lab:dev"
	Image string `json:"image,omitempty"`
	// Args is the ordered argument list passed to every worker container.
	Args []string `json:"args,omitempty"`
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

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="AIJob spec is immutable; recreate the AIJob to change the workload"
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
