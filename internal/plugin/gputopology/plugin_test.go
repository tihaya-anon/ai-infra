package gputopology

import (
	"testing"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	fwk "k8s.io/kube-scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestTopologyScore(t *testing.T) {
	tests := []struct {
		name       string
		preference string
		labels     map[string]string
		want       int64
	}{
		{
			name:       "prefer NVLink",
			preference: "nvlink",
			labels:     map[string]string{topology.FabricLabel: "nvlink"},
			want:       framework.MaxNodeScore,
		},
		{
			name:       "PCIe is fallback for NVLink",
			preference: "nvlink",
			labels:     map[string]string{topology.FabricLabel: "pcie"},
			want:       50,
		},
		{
			name:       "NVLink satisfies PCIe",
			preference: "pcie",
			labels:     map[string]string{topology.FabricLabel: "nvlink"},
			want:       framework.MaxNodeScore,
		},
		{
			name:       "same rack belongs to Kueue",
			preference: "same-rack",
			labels:     map[string]string{topology.RackLabel: "rack-a"},
			want:       0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := topologyPreferenceScore(test.preference, test.labels); got != test.want {
				t.Fatalf("got score %d, want %d", got, test.want)
			}
		})
	}
}

func TestTopologyRequirementStatus(t *testing.T) {
	tests := []struct {
		name     string
		required string
		labels   map[string]string
		wantCode fwk.Code
	}{
		{
			name:     "no requirement",
			required: "",
			labels:   map[string]string{topology.FabricLabel: "pcie"},
			wantCode: fwk.Success,
		},
		{
			name:     "NVLink requires NVLink fabric",
			required: "nvlink",
			labels:   map[string]string{topology.FabricLabel: "nvlink"},
			wantCode: fwk.Success,
		},
		{
			name:     "NVLink rejects PCIe fabric",
			required: "nvlink",
			labels:   map[string]string{topology.FabricLabel: "pcie"},
			wantCode: fwk.Unschedulable,
		},
		{
			name:     "PCIe accepts NVLink fabric",
			required: "pcie",
			labels:   map[string]string{topology.FabricLabel: "nvlink"},
			wantCode: fwk.Success,
		},
		{
			name:     "PCIe accepts PCIe fabric",
			required: "pcie",
			labels:   map[string]string{topology.FabricLabel: "pcie"},
			wantCode: fwk.Success,
		},
		{
			name:     "PCIe rejects missing fabric",
			required: "pcie",
			labels:   map[string]string{},
			wantCode: fwk.Unschedulable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := topologyRequirementStatus(test.required, test.labels)
			if test.wantCode == fwk.Success && status != nil {
				t.Fatalf("got status %v, want success", status)
			}
			if test.wantCode != fwk.Success && (status == nil || status.Code() != test.wantCode) {
				t.Fatalf("got status %v, want code %v", status, test.wantCode)
			}
		})
	}
}
