package aijob

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GVR = schema.GroupVersionResource{
	Group: "infra.example.io", Version: "v1alpha1", Resource: "aijobs",
}

type Spec struct {
	Workers      int64
	GPUPerWorker int64
	Topology     string
	Image        string
}

func Parse(object *unstructured.Unstructured) (Spec, error) {
	workers, found, err := unstructured.NestedInt64(object.Object, "spec", "workers")
	if err != nil || !found || workers < 1 {
		return Spec{}, fmt.Errorf("spec.workers must be a positive integer")
	}
	gpus, found, err := unstructured.NestedInt64(object.Object, "spec", "gpuPerWorker")
	if err != nil || !found || gpus < 1 {
		return Spec{}, fmt.Errorf("spec.gpuPerWorker must be a positive integer")
	}
	topology, _, _ := unstructured.NestedString(object.Object, "spec", "topology")
	image, _, _ := unstructured.NestedString(object.Object, "spec", "image")
	if image == "" {
		image = "ai-infra-lab:dev"
	}
	return Spec{Workers: workers, GPUPerWorker: gpus, Topology: topology, Image: image}, nil
}

func OwnerReference(object *unstructured.Unstructured) metav1.OwnerReference {
	controller := true
	blockDeletion := true
	return metav1.OwnerReference{
		APIVersion: object.GetAPIVersion(), Kind: object.GetKind(), Name: object.GetName(),
		UID: object.GetUID(), Controller: &controller, BlockOwnerDeletion: &blockDeletion,
	}
}
