package topology

import corev1 "k8s.io/api/core/v1"

const (
	JobLabel             = "infra.example.io/aijob"
	PreferenceAnnotation = "infra.example.io/gpu-topology"
	FabricLabel          = "infra.example.io/gpu-fabric"
	RackLabel            = "infra.example.io/rack"
	FabricNVLink         = "nvlink"
	FabricPCIe           = "pcie"
)

// GPUResource is simulated by the kind lab. Production AIJobs should select
// the extended resource advertised by their GPU device plugin.
const GPUResource corev1.ResourceName = "example.com/gpu"
