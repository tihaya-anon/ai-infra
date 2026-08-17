package lab

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGivenFreeGPUCapacityWhenCalculatingFragmentationThenCapacityIsClassified(t *testing.T) {
	tests := []struct {
		name       string
		free       map[string]int64
		wantTotal  int64
		wantUsable int64
		wantRatio  float64
	}{
		{name: "spread 2 2 2", free: gpuMap(2, 2, 2), wantTotal: 6, wantUsable: 0, wantRatio: 1},
		{name: "packed 0 2 4", free: gpuMap(0, 2, 4), wantTotal: 6, wantUsable: 4, wantRatio: 1.0 / 3.0},
		{name: "no free capacity", free: gpuMap(0, 0, 0), wantTotal: 0, wantUsable: 0, wantRatio: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := gomega.NewWithT(t)

			// given
			free := test.free

			// when
			got := CalculateFragmentation(free, 4, "all-gpu-nodes")

			// then
			assert.Expect(got.TotalFree).To(gomega.Equal(test.wantTotal))
			assert.Expect(got.UsableFree).To(gomega.Equal(test.wantUsable))
			assert.Expect(got.Ratio).To(gomega.BeNumerically("~", test.wantRatio, 1e-9))
		})
	}
}

func TestGivenBoundPodsWhenCalculatingFreeGPUsThenOnlyActiveRequestsAreCounted(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	nodes := []corev1.Node{gpuNode("node-a", 4), gpuNode("node-b", 4)}
	pods := []corev1.Pod{
		gpuPod("active", "node-a", corev1.PodRunning, 2),
		gpuPod("done", "node-a", corev1.PodSucceeded, 2),
		gpuPod("pending", "", corev1.PodPending, 4),
	}

	// when
	free := FreeGPUs(nodes, pods)

	// then
	assert.Expect(free).To(gomega.Equal(map[string]int64{"node-a": 2, "node-b": 4}))
}

func TestGivenUnschedulablePodsWhenCountingThenMatchingConditionsAreCounted(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	pods := []corev1.Pod{{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
		Reason: corev1.PodReasonUnschedulable,
	}}}}}

	// when
	count := UnschedulableCount(pods)

	// then
	assert.Expect(count).To(gomega.Equal(1))
}

func gpuMap(values ...int64) map[string]int64 {
	result := make(map[string]int64, len(values))
	for index, value := range values {
		result[string(rune('a'+index))] = value
	}
	return result
}

func gpuNode(name string, capacity int64) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Labels: map[string]string{"infra.example.io/gpu-node": "true"},
		},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			topology.GPUResource: *resource.NewQuantity(capacity, resource.DecimalSI),
		}},
	}
}

func gpuPod(name, node string, phase corev1.PodPhase, request int64) corev1.Pod {
	quantity := *resource.NewQuantity(request, resource.DecimalSI)
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{topology.GPUResource: quantity},
			}}},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}
