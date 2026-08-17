package manifests

import (
	"bytes"
	"os"
	"testing"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

func TestKueueResourcesContainEveryConfiguredQueue(t *testing.T) {
	resources := kueueResources()
	clusterQueues := make(map[string]bool)
	localQueues := make(map[string]bool)
	for _, object := range resources {
		switch resource := object.(type) {
		case *kueuev1beta1.ClusterQueue:
			clusterQueues[resource.Name] = true
		case *kueuev1beta1.LocalQueue:
			localQueues[resource.Name] = true
		}
	}
	for _, config := range queueConfigs {
		if !clusterQueues[config.name] || !localQueues[config.name] {
			t.Fatalf("queue %q does not have both ClusterQueue and LocalQueue resources", config.name)
		}
	}
}

func TestGeneratedKueueManifestIsCurrent(t *testing.T) {
	want, err := RenderKueueResources()
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	got, err := os.ReadFile("../../deploy/kueue-resources.yaml")
	if err != nil {
		t.Fatalf("read generated manifest: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("deploy/kueue-resources.yaml is stale; run make generate")
	}
}
