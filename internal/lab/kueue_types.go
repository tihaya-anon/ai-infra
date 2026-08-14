package lab

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var kueueGroupVersion = schema.GroupVersion{Group: "kueue.x-k8s.io", Version: "v1beta1"}

// Workload is the typed subset of the Kueue API observed by this lab.
type Workload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            WorkloadStatus `json:"status,omitempty"`
}

// WorkloadStatus contains the admission conditions needed for lifecycle timing.
type WorkloadStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// WorkloadList contains Kueue Workloads.
type WorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workload `json:"items"`
}

// ClusterQueue is the typed quota subset validated before a benchmark.
type ClusterQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterQueueSpec `json:"spec,omitempty"`
}

// ClusterQueueSpec contains resource groups and flavor quotas.
type ClusterQueueSpec struct {
	ResourceGroups []ResourceGroup `json:"resourceGroups,omitempty"`
}

// ResourceGroup lists covered resources and flavor quota entries.
type ResourceGroup struct {
	CoveredResources []corev1.ResourceName `json:"coveredResources,omitempty"`
	Flavors          []FlavorQuotas        `json:"flavors,omitempty"`
}

// FlavorQuotas contains nominal quota values for one ResourceFlavor.
type FlavorQuotas struct {
	Name      string          `json:"name,omitempty"`
	Resources []ResourceQuota `json:"resources,omitempty"`
}

// ResourceQuota is the typed subset of one flavor resource quota.
type ResourceQuota struct {
	Name         corev1.ResourceName `json:"name"`
	NominalQuota resource.Quantity   `json:"nominalQuota"`
}

func (in *Workload) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Workload)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Status.Conditions != nil {
		out.Status.Conditions = append([]metav1.Condition(nil), in.Status.Conditions...)
	}
	return out
}

func (in *WorkloadList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(WorkloadList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]Workload, len(in.Items))
		for index := range in.Items {
			out.Items[index] = *(in.Items[index].DeepCopyObject().(*Workload))
		}
	}
	return out
}

func (in *ClusterQueue) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ClusterQueue)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec.ResourceGroups = make([]ResourceGroup, len(in.Spec.ResourceGroups))
	for groupIndex := range in.Spec.ResourceGroups {
		group := in.Spec.ResourceGroups[groupIndex]
		out.Spec.ResourceGroups[groupIndex].CoveredResources =
			append([]corev1.ResourceName(nil), group.CoveredResources...)
		out.Spec.ResourceGroups[groupIndex].Flavors = make([]FlavorQuotas, len(group.Flavors))
		for flavorIndex := range group.Flavors {
			flavor := group.Flavors[flavorIndex]
			out.Spec.ResourceGroups[groupIndex].Flavors[flavorIndex] = FlavorQuotas{
				Name: flavor.Name, Resources: append([]ResourceQuota(nil), flavor.Resources...),
			}
		}
	}
	return out
}

func addKueueToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(
		kueueGroupVersion, &Workload{}, &WorkloadList{}, &ClusterQueue{},
	)
	metav1.AddToGroupVersion(scheme, kueueGroupVersion)
}
