package gputopology

import (
	"context"
	"fmt"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	Name     = "GPUTopology"
	stateKey = framework.StateKey(Name + "State")
)

var (
	_ framework.PreScorePlugin = &Plugin{}
	_ framework.ScorePlugin    = &Plugin{}
)

// Plugin adds AI-specific topology preferences to kube-scheduler scoring.
type Plugin struct {
	handle framework.Handle
}

type cycleData struct {
	preferredRack string
}

func (d *cycleData) Clone() framework.StateData {
	copy := *d
	return &copy
}

// New is the factory registered in the kube-scheduler plugin registry.
func New(_ context.Context, _ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Plugin{handle: handle}, nil
}

func (p *Plugin) Name() string { return Name }

// PreScore derives job-level locality once for the current scheduling cycle.
func (p *Plugin) PreScore(_ context.Context, state *framework.CycleState, pod *corev1.Pod, _ []*framework.NodeInfo) *framework.Status {
	want := preference(pod)
	if want == "" || want == "any" {
		return framework.NewStatus(framework.Skip)
	}

	data := &cycleData{}
	if want == "same-rack" {
		data.preferredRack = p.findPeerRack(pod)
	}
	state.Write(stateKey, data)
	return nil
}

// Score ranks a feasible Node without replacing kube-scheduler's default checks.
func (p *Plugin) Score(_ context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("node %s not found", nodeName))
	}

	data, err := readCycleData(state)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	return topologyScore(preference(pod), node.Labels, data.preferredRack), nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

func (p *Plugin) findPeerRack(pod *corev1.Pod) string {
	jobName := pod.Labels[topology.JobLabel]
	if jobName == "" {
		return ""
	}
	nodes, err := p.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return ""
	}
	for _, nodeInfo := range nodes {
		if nodeInfo.Node() == nil {
			continue
		}
		for _, peer := range nodeInfo.Pods {
			if peer.Pod.Labels[topology.JobLabel] == jobName {
				return nodeInfo.Node().Labels[topology.RackLabel]
			}
		}
	}
	return ""
}

func readCycleData(state *framework.CycleState) (*cycleData, error) {
	data, err := state.Read(stateKey)
	if err != nil {
		return nil, err
	}
	result, ok := data.(*cycleData)
	if !ok {
		return nil, fmt.Errorf("invalid %s cycle state", Name)
	}
	return result, nil
}

func preference(pod *corev1.Pod) string {
	return pod.Annotations[topology.PreferenceAnnotation]
}

func topologyScore(want string, labels map[string]string, preferredRack string) int64 {
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
	case "same-rack":
		if preferredRack != "" && labels[topology.RackLabel] == preferredRack {
			return framework.MaxNodeScore
		}
	}
	return 0
}
