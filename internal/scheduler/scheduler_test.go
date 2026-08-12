package scheduler

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestChooseNodeBinPacksWorkers(t *testing.T) {
	nodes := []*corev1.Node{
		node("node-a", "rack-a", "4"),
		node("node-b", "rack-b", "4"),
	}
	pods := []*corev1.Pod{boundPod("existing", "node-b", "2", "other")}
	wanted := pendingPod("worker", "2", "job", "")

	got, _, err := chooseNode(wanted, nodes, pods)
	if err != nil {
		t.Fatal(err)
	}
	if got != "node-b" {
		t.Fatalf("expected node-b to be filled first, got %s", got)
	}
}

func TestChooseNodePrefersSameRack(t *testing.T) {
	nodes := []*corev1.Node{
		node("node-a", "rack-a", "4"),
		node("node-b", "rack-b", "4"),
	}
	pods := []*corev1.Pod{boundPod("peer", "node-b", "1", "training")}
	wanted := pendingPod("worker", "1", "training", "same-rack")

	got, _, err := chooseNode(wanted, nodes, pods)
	if err != nil {
		t.Fatal(err)
	}
	if got != "node-b" {
		t.Fatalf("expected same-rack node-b, got %s", got)
	}
}

func TestChooseNodeRejectsInsufficientCapacity(t *testing.T) {
	nodes := []*corev1.Node{node("node-a", "rack-a", "2")}
	pods := []*corev1.Pod{boundPod("existing", "node-a", "2", "other")}

	_, _, err := chooseNode(pendingPod("worker", "1", "job", ""), nodes, pods)
	if err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func node(name, rack, capacity string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{rackLabel: rack, gpuCapacityLabel: capacity}}}
}

func pendingPod(name, request, job, topology string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{gpuRequestLabel: request, "infra.example.io/aijob": job, "infra.example.io/topology": topology}}}
}

func boundPod(name, nodeName, request, job string) *corev1.Pod {
	pod := pendingPod(name, request, job, "")
	pod.Spec.NodeName = nodeName
	return pod
}
