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

// Score ranks a feasible Node without replacing kube-scheduler's default checks.
func (p *Plugin) Score(
	_ context.Context,
	_ fwk.CycleState,
	pod *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) (int64, *fwk.Status) {
	started := time.Now()
	node := nodeInfo.Node()
	if node == nil {
		p.metrics.recordError("node_missing")
		p.metrics.observeScore(started, "error", preference(pod), "")
		return 0, fwk.NewStatus(fwk.Error, fmt.Sprintf("node not found in %s", nodeInfo.String()))
	}
	p.metrics.observeScore(
		started, "success", preference(pod), node.Labels[topology.FabricLabel],
	)
	return topologyScore(preference(pod), node.Labels), nil
}

// ScoreExtensions reports that this plugin does not normalize scores.
func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

func preference(pod *corev1.Pod) string {
	return pod.Annotations[topology.PreferenceAnnotation]
}

func topologyScore(want string, labels map[string]string) int64 {
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
