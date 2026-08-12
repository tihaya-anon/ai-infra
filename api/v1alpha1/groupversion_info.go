package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion identifies the version served by the AIJob CRD.
	GroupVersion = schema.GroupVersion{Group: "infra.example.io", Version: "v1alpha1"}
	// SchemeBuilder registers AIJob types with controller-runtime.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	// AddToScheme adds all AIJob API types to a runtime scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&AIJob{}, &AIJobList{})
}
