package topology

import corev1 "k8s.io/api/core/v1"

const (
	// JobLabel connects the JobSet, Jobs, and Pods back to their AIJob.
	JobLabel = "infra.example.io/aijob"
	// QueueLabel selects the Kueue LocalQueue used for admission.
	QueueLabel = "kueue.x-k8s.io/queue-name"
	// RunIDLabel scopes generated lab resources, evidence, and cleanup to one run.
	RunIDLabel = "infra.example.io/run-id"
	// ExperimentLabel is a bounded category such as benchmark or worker-failure.
	ExperimentLabel = "infra.example.io/experiment"
	// PreferenceAnnotation carries a GPU topology preference to the scheduler plugin.
	PreferenceAnnotation = "infra.example.io/gpu-topology/preference"
	// RequiredTopologyClassAnnotation carries a node-level topology-class constraint.
	RequiredTopologyClassAnnotation = "infra.example.io/gpu-topology/required-class"
	// RequiredTopologyAnnotation asks Kueue TAS to co-locate the whole PodSet.
	RequiredTopologyAnnotation = "kueue.x-k8s.io/podset-required-topology"
	GPUTopologyClassLabel      = "infra.example.io/gpu-topology-class"
	RackLabel                  = "infra.example.io/rack"
	TopologyNVLink             = "nvlink"
	TopologyPCIe               = "pcie"
	TopologyClassNVLinkCapable = "nvlink-capable"
	TopologyClassPCIeOnly      = "pcie-only"
)

// GPUResource is simulated by the kind lab. Production AIJobs should select
// the extended resource advertised by their GPU device plugin.
const GPUResource corev1.ResourceName = "example.com/gpu"

// LabLabels returns only the bounded set of labels propagated through descendants.
func LabLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, 2)
	for _, key := range []string{RunIDLabel, ExperimentLabel} {
		if value := labels[key]; value != "" {
			result[key] = value
		}
	}
	return result
}
