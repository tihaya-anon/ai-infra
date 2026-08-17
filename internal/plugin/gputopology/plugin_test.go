package gputopology

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	fwk "k8s.io/kube-scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestGivenTopologyPreferencesWhenScoringNodesThenExpectedScoresAreReturned(t *testing.T) {
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
			assert := gomega.NewWithT(t)

			// given
			preference := test.preference
			labels := test.labels

			// when
			score := topologyPreferenceScore(preference, labels)

			// then
			assert.Expect(score).To(gomega.Equal(test.want))
		})
	}
}

func TestGivenTopologyRequirementsWhenFilteringNodesThenExpectedStatusIsReturned(t *testing.T) {
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
			assert := gomega.NewWithT(t)

			// given
			required := test.required
			labels := test.labels

			// when
			status := topologyRequirementStatus(required, labels)

			// then
			if test.wantCode == fwk.Success {
				assert.Expect(status).To(gomega.BeNil())
			} else {
				assert.Expect(status).NotTo(gomega.BeNil())
				assert.Expect(status.Code()).To(gomega.Equal(test.wantCode))
			}
		})
	}
}
