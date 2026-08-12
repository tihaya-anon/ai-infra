package topology

import corev1 "k8s.io/api/core/v1"

const (
	// JobLabel connects the JobSet, Jobs, and Pods back to their AIJob.
	JobLabel = "infra.example.io/aijob"
	// QueueLabel selects the Kueue LocalQueue used for admission.
	QueueLabel = "kueue.x-k8s.io/queue-name"
	// PreferenceAnnotation carries node-level fabric intent to the scheduler plugin.
	PreferenceAnnotation = "infra.example.io/gpu-topology"
	// RequiredTopologyAnnotation asks Kueue TAS to co-locate the whole PodSet.
	RequiredTopologyAnnotation = "kueue.x-k8s.io/podset-required-topology"
	FabricLabel                = "infra.example.io/gpu-fabric"
	RackLabel                  = "infra.example.io/rack"
	FabricNVLink               = "nvlink"
	FabricPCIe                 = "pcie"
)

// GPUResource is simulated by the kind lab. Production AIJobs should select
// the extended resource advertised by their GPU device plugin.
const GPUResource corev1.ResourceName = "example.com/gpu"
