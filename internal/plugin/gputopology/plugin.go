package gputopology

import (
	"context"
	"fmt"
	"time"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fwk "k8s.io/kube-scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	schedulerruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

// Name is the identifier used in the scheduler registry and profile config.
const Name = "GPUTopology"

var _ framework.ScorePlugin = &Plugin{}
var _ framework.FilterPlugin = &Plugin{}
var _ framework.PreFilterPlugin = &Plugin{}
var _ framework.PreScorePlugin = &Plugin{}

const topologyStateKey fwk.StateKey = Name

// Plugin adds cluster-specific GPU topology-class preferences to node scoring.
type Plugin struct {
	metrics *Metrics
}

type topologyState struct {
	preference string
	required   string
}

func (s *topologyState) Clone() fwk.StateData {
	return &topologyState{preference: s.preference, required: s.required}
}

// New is the factory registered in the kube-scheduler plugin registry.
func New(_ context.Context, _ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &Plugin{}, nil
}

// NewFactory injects registered metrics into each plugin instance.
func NewFactory(metrics *Metrics) schedulerruntime.PluginFactory {
	return func(
		_ context.Context,
		_ runtime.Object,
		_ framework.Handle,
	) (framework.Plugin, error) {
		return &Plugin{metrics: metrics}, nil
	}
}

// Name implements framework.Plugin.
func (p *Plugin) Name() string { return Name }

// PreFilter parses topology annotations once before node filtering.
func (p *Plugin) PreFilter(
	_ context.Context,
	state fwk.CycleState,
	pod *corev1.Pod,
	_ []fwk.NodeInfo,
) (*framework.PreFilterResult, *fwk.Status) {
	state.Write(topologyStateKey, topologyStateFromPod(pod))
	return nil, nil
}

// PreFilterExtensions reports that this plugin does not maintain incremental state.
func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions { return nil }

// PreScore refreshes topology annotations before node scoring.
func (p *Plugin) PreScore(
	_ context.Context,
	state fwk.CycleState,
	pod *corev1.Pod,
	_ []fwk.NodeInfo,
) *fwk.Status {
	state.Write(topologyStateKey, topologyStateFromPod(pod))
	return nil
}

func (p *Plugin) observeFilterAndScore(
	intent *topologyState,
	nodeInfo fwk.NodeInfo,
) (*corev1.Node, *fwk.Status) {
	started := time.Now()
	node := nodeInfo.Node()
	if node == nil {
		p.metrics.recordError(errorReasonNodeMissing)
		p.metrics.observeScore(started, scoreError, intent.preference, "")
		return nil, fwk.NewStatus(
			fwk.Error,
			fmt.Sprintf("node not found in %s", nodeInfo.String()),
		)
	}
	p.metrics.observeScore(
		started, scoreSuccess, intent.preference, nodeTopologyClass(node.Labels),
	)
	return node, nil
}

// Filter rejects nodes that do not satisfy a required node GPU topology class.
func (p *Plugin) Filter(
	_ context.Context,
	state fwk.CycleState,
	_ *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) *fwk.Status {
	intent, status := topologyStateFromCycle(state)
	if status != nil {
		return status
	}
	node, status := p.observeFilterAndScore(intent, nodeInfo)
	if status != nil {
		return status
	}
	return topologyRequirementStatus(intent.required, node.Labels)
}

// Score ranks a feasible Node without replacing kube-scheduler's default checks.
func (p *Plugin) Score(
	_ context.Context,
	state fwk.CycleState,
	_ *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) (int64, *fwk.Status) {
	intent, status := topologyStateFromCycle(state)
	if status != nil {
		return 0, status
	}
	node, status := p.observeFilterAndScore(intent, nodeInfo)
	if status != nil {
		return 0, status
	}
	return topologyPreferenceScore(intent.preference, node.Labels), nil
}

// ScoreExtensions reports that this plugin does not normalize scores.
func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

func topologyStateFromPod(pod *corev1.Pod) *topologyState {
	return &topologyState{
		preference: pod.Annotations[topology.PreferenceAnnotation],
		required:   pod.Annotations[topology.RequiredTopologyClassAnnotation],
	}
}

func topologyStateFromCycle(state fwk.CycleState) (*topologyState, *fwk.Status) {
	data, err := state.Read(topologyStateKey)
	if err != nil {
		return nil, fwk.NewStatus(fwk.Error, "GPU topology cycle state not found")
	}
	intent, ok := data.(*topologyState)
	if !ok {
		return nil, fwk.NewStatus(fwk.Error, "GPU topology cycle state has unexpected type")
	}
	return intent, nil
}

func topologyRequirementStatus(want string, labels map[string]string) *fwk.Status {
	switch want {
	case "", "any":
		return nil
	case topology.TopologyNVLink:
		if nodeTopologyClass(labels) == topology.TopologyClassNVLinkCapable {
			return nil
		}
		return fwk.NewStatus(
			fwk.Unschedulable,
			fmt.Sprintf("requires GPU topology compatible with %s", topology.TopologyNVLink),
		)
	case topology.TopologyPCIe:
		class := nodeTopologyClass(labels)
		if class == topology.TopologyClassNVLinkCapable ||
			class == topology.TopologyClassPCIeOnly {
			return nil
		}
		return fwk.NewStatus(
			fwk.Unschedulable,
			fmt.Sprintf("requires GPU topology compatible with %s", topology.TopologyPCIe),
		)
	default:
		return nil
	}
}

func topologyPreferenceScore(want string, labels map[string]string) int64 {
	class := nodeTopologyClass(labels)
	switch want {
	case topology.TopologyNVLink:
		if class == topology.TopologyClassNVLinkCapable {
			return framework.MaxNodeScore
		}
		if class == topology.TopologyClassPCIeOnly {
			return 50
		}
	case topology.TopologyPCIe:
		if class == topology.TopologyClassNVLinkCapable {
			return framework.MaxNodeScore
		}
		if class == topology.TopologyClassPCIeOnly {
			return 80
		}
	}
	return 0
}

func nodeTopologyClass(labels map[string]string) string {
	return labels[topology.GPUTopologyClassLabel]
}
