package gputopology

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassNVLinkCapable,
			},
			want: framework.MaxNodeScore,
		},
		{
			name:       "PCIe is fallback for NVLink",
			preference: "nvlink",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassPCIeOnly,
			},
			want: 50,
		},
		{
			name:       "NVLink satisfies PCIe",
			preference: "pcie",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassNVLinkCapable,
			},
			want: framework.MaxNodeScore,
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

func TestGivenPreFilterStateWhenFilteringThenStoredRequiredTopologyIsUsed(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	plugin := &Plugin{}
	state := framework.NewCycleState()
	pod := podWithTopology("pcie", "nvlink")
	node := nodeInfoWithTopologyClass(topology.TopologyClassPCIeOnly)

	// when
	_, preFilterStatus := plugin.PreFilter(context.Background(), state, pod, nil)
	pod.Annotations[topology.RequiredTopologyClassAnnotation] = "pcie"
	filterStatus := plugin.Filter(context.Background(), state, pod, node)

	// then
	assert.Expect(preFilterStatus).To(gomega.BeNil())
	assert.Expect(filterStatus).NotTo(gomega.BeNil())
	assert.Expect(filterStatus.Code()).To(gomega.Equal(fwk.Unschedulable))
}

func TestGivenPreScoreStateWhenScoringThenStoredPreferenceIsUsed(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	plugin := &Plugin{}
	state := framework.NewCycleState()
	pod := podWithTopology("nvlink", "")
	node := nodeInfoWithTopologyClass(topology.TopologyClassPCIeOnly)

	// when
	preScoreStatus := plugin.PreScore(context.Background(), state, pod, nil)
	pod.Annotations[topology.PreferenceAnnotation] = "pcie"
	score, scoreStatus := plugin.Score(context.Background(), state, pod, node)

	// then
	assert.Expect(preScoreStatus).To(gomega.BeNil())
	assert.Expect(scoreStatus).To(gomega.BeNil())
	assert.Expect(score).To(gomega.Equal(int64(50)))
}

func TestGivenMissingCycleStateWhenFilteringOrScoringThenErrorIsReturned(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	plugin := &Plugin{}
	state := framework.NewCycleState()
	pod := podWithTopology("nvlink", "nvlink")
	node := nodeInfoWithTopologyClass(topology.TopologyClassNVLinkCapable)

	// when
	filterStatus := plugin.Filter(context.Background(), state, pod, node)
	score, scoreStatus := plugin.Score(context.Background(), state, pod, node)

	// then
	assert.Expect(filterStatus).NotTo(gomega.BeNil())
	assert.Expect(filterStatus.Code()).To(gomega.Equal(fwk.Error))
	assert.Expect(score).To(gomega.Equal(int64(0)))
	assert.Expect(scoreStatus).NotTo(gomega.BeNil())
	assert.Expect(scoreStatus.Code()).To(gomega.Equal(fwk.Error))
}

func podWithTopology(preference, required string) *corev1.Pod {
	annotations := map[string]string{}
	if preference != "" {
		annotations[topology.PreferenceAnnotation] = preference
	}
	if required != "" {
		annotations[topology.RequiredTopologyClassAnnotation] = required
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
}

func nodeInfoWithTopologyClass(class string) fwk.NodeInfo {
	nodeInfo := framework.NewNodeInfo()
	nodeInfo.SetNode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-a",
		Labels: map[string]string{
			topology.GPUTopologyClassLabel: class,
		},
	}})
	return nodeInfo
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
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassPCIeOnly,
			},
			wantCode: fwk.Success,
		},
		{
			name:     "NVLink requires NVLink-capable node class",
			required: "nvlink",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassNVLinkCapable,
			},
			wantCode: fwk.Success,
		},
		{
			name:     "NVLink rejects PCIe-only node class",
			required: "nvlink",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassPCIeOnly,
			},
			wantCode: fwk.Unschedulable,
		},
		{
			name:     "PCIe accepts NVLink-capable node class",
			required: "pcie",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassNVLinkCapable,
			},
			wantCode: fwk.Success,
		},
		{
			name:     "PCIe accepts PCIe-only node class",
			required: "pcie",
			labels: map[string]string{
				topology.GPUTopologyClassLabel: topology.TopologyClassPCIeOnly,
			},
			wantCode: fwk.Success,
		},
		{
			name:     "PCIe rejects missing node class",
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
