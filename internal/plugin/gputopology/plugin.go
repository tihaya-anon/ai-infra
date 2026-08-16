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

// Plugin adds cluster-specific GPU fabric preferences to node scoring.
type Plugin struct {
	metrics *Metrics
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

func (p *Plugin) observeFilterAndScore(pod *corev1.Pod,
	nodeInfo fwk.NodeInfo) *fwk.Status {
	started := time.Now()
	node := nodeInfo.Node()
	if node == nil {
		p.metrics.recordError(errorReasonNodeMissing)
		p.metrics.observeScore(started, scoreError, preference(pod), "")
		return fwk.NewStatus(fwk.Error, fmt.Sprintf("node not found in %s", nodeInfo.String()))
	}
	p.metrics.observeScore(
		started, scoreSuccess, preference(pod), node.Labels[topology.FabricLabel],
	)
	return nil
}

// Filter rejects nodes that do not satisfy a required GPU fabric.
func (p *Plugin) Filter(
	_ context.Context,
	_ fwk.CycleState,
	pod *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) *fwk.Status {
	if status := p.observeFilterAndScore(pod, nodeInfo); status != nil {
		return status
	}
	node := nodeInfo.Node()
	return topologyRequirementStatus(requiredFabric(pod), node.Labels)
}

// Score ranks a feasible Node without replacing kube-scheduler's default checks.
func (p *Plugin) Score(
	_ context.Context,
	_ fwk.CycleState,
	pod *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) (int64, *fwk.Status) {
	if status := p.observeFilterAndScore(pod, nodeInfo); status != nil {
		return 0, status
	}
	node := nodeInfo.Node()
	return topologyPreferenceScore(preference(pod), node.Labels), nil
}

// ScoreExtensions reports that this plugin does not normalize scores.
func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

func preference(pod *corev1.Pod) string {
	return pod.Annotations[topology.PreferenceAnnotation]
}

func requiredFabric(pod *corev1.Pod) string {
	return pod.Annotations[topology.RequiredFabricAnnotation]
}

func topologyRequirementStatus(want string, labels map[string]string) *fwk.Status {
	switch want {
	case "", "any":
		return nil
	case topology.FabricNVLink:
		if labels[topology.FabricLabel] == topology.FabricNVLink {
			return nil
		}
		return fwk.NewStatus(
			fwk.Unschedulable,
			fmt.Sprintf("requires GPU fabric %s", topology.FabricNVLink),
		)
	case topology.FabricPCIe:
		if labels[topology.FabricLabel] == topology.FabricNVLink ||
			labels[topology.FabricLabel] == topology.FabricPCIe {
			return nil
		}
		return fwk.NewStatus(
			fwk.Unschedulable,
			fmt.Sprintf("requires GPU fabric %s", topology.FabricPCIe),
		)
	default:
		return nil
	}
}

func topologyPreferenceScore(want string, labels map[string]string) int64 {
	switch want {
	case topology.FabricNVLink:
		if labels[topology.FabricLabel] == topology.FabricNVLink {
			return framework.MaxNodeScore
		}
		if labels[topology.FabricLabel] == topology.FabricPCIe {
			return 50
		}
	case topology.FabricPCIe:
		if labels[topology.FabricLabel] == topology.FabricNVLink {
			return framework.MaxNodeScore
		}
		if labels[topology.FabricLabel] == topology.FabricPCIe {
			return 80
		}
	}
	return 0
}
