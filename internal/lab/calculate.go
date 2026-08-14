package lab

import (
	"sort"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
)

// FreeGPUs returns allocatable simulated GPUs minus bound, non-terminal Pod requests.
func FreeGPUs(nodes []corev1.Node, pods []corev1.Pod) map[string]int64 {
	free := make(map[string]int64)
	for _, node := range nodes {
		if node.Labels["infra.example.io/gpu-node"] != "true" {
			continue
		}
		free[node.Name] = quantityValue(node.Status.Allocatable, topology.GPUResource)
	}
	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded ||
			pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if _, eligible := free[pod.Spec.NodeName]; !eligible {
			continue
		}
		free[pod.Spec.NodeName] -= podGPURequest(pod)
		if free[pod.Spec.NodeName] < 0 {
			free[pod.Spec.NodeName] = 0
		}
	}
	return free
}

// CalculateFragmentation applies the target-relative metric from the specification.
func CalculateFragmentation(
	free map[string]int64,
	target int64,
	topologyDomain string,
) Fragmentation {
	result := Fragmentation{
		TargetGPUs: target, TopologyDomain: topologyDomain,
		FreeByNode: make(map[string]int64, len(free)),
	}
	for node, available := range free {
		result.EligibleNodes = append(result.EligibleNodes, node)
		result.FreeByNode[node] = available
		result.TotalFree += available
		if target > 0 {
			result.UsableFree += (available / target) * target
		}
	}
	sort.Strings(result.EligibleNodes)
	if result.TotalFree > 0 {
		result.Ratio = float64(result.TotalFree-result.UsableFree) / float64(result.TotalFree)
	}
	return result
}

// UnschedulableCount counts Pods currently exposing an Unschedulable condition.
func UnschedulableCount(pods []corev1.Pod) int {
	count := 0
	for _, pod := range pods {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse &&
				condition.Reason == corev1.PodReasonUnschedulable {
				count++
				break
			}
		}
	}
	return count
}

func podGPURequest(pod corev1.Pod) int64 {
	var regular int64
	for _, container := range pod.Spec.Containers {
		regular += quantityValue(container.Resources.Requests, topology.GPUResource)
	}
	var initMax int64
	for _, container := range pod.Spec.InitContainers {
		request := quantityValue(container.Resources.Requests, topology.GPUResource)
		if request > initMax {
			initMax = request
		}
	}
	if initMax > regular {
		regular = initMax
	}
	return regular + quantityValue(pod.Spec.Overhead, topology.GPUResource)
}

func quantityValue(resources corev1.ResourceList, name corev1.ResourceName) int64 {
	quantity := resources[name]
	return quantity.Value()
}
