package gputopology

import (
	"testing"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestTopologyScore(t *testing.T) {
	tests := []struct {
		name          string
		preference    string
		labels        map[string]string
		preferredRack string
		want          int64
	}{
		{name: "prefer NVLink", preference: "nvlink", labels: map[string]string{topology.FabricLabel: "nvlink"}, want: framework.MaxNodeScore},
		{name: "PCIe is fallback for NVLink", preference: "nvlink", labels: map[string]string{topology.FabricLabel: "pcie"}, want: 50},
		{name: "NVLink satisfies PCIe", preference: "pcie", labels: map[string]string{topology.FabricLabel: "nvlink"}, want: framework.MaxNodeScore},
		{name: "same rack", preference: "same-rack", labels: map[string]string{topology.RackLabel: "rack-a"}, preferredRack: "rack-a", want: framework.MaxNodeScore},
		{name: "different rack", preference: "same-rack", labels: map[string]string{topology.RackLabel: "rack-b"}, preferredRack: "rack-a", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := topologyScore(test.preference, test.labels, test.preferredRack); got != test.want {
				t.Fatalf("got score %d, want %d", got, test.want)
			}
		})
	}
}
