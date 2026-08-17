package lab

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGivenWorkerRecordsWhenBuildingReportThenRuntimeFieldsAreCollected(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	pods := []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{
		Name: "worker-0", Labels: map[string]string{
			topology.JobLabel: "training", "batch.kubernetes.io/job-completion-index": "0",
		},
	}}}
	logs := map[string][]byte{"worker-0": []byte(
		"{\"type\":\"start\",\"timestamp\":\"2026-08-17T01:00:00Z\"," +
			"\"parameters\":{\"mode\":\"complete\",\"duration\":\"1s\"," +
			"\"startupDelay\":\"0s\"}}\n" +
			"{\"type\":\"result\",\"timestamp\":\"2026-08-17T01:00:01Z\"," +
			"\"parameters\":{\"mode\":\"complete\",\"duration\":\"1s\"," +
			"\"startupDelay\":\"0s\"},\"exitReason\":\"failed\",\"exitCode\":10}\n",
	)}

	// when
	reports, err := workerRuntimeReports(pods, logs)

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(reports).To(gomega.HaveLen(1))
	assert.Expect(reports[0].StartedAt).NotTo(gomega.BeNil())
	assert.Expect(reports[0].FinishedAt).NotTo(gomega.BeNil())
	assert.Expect(reports[0].ExitReason).To(gomega.Equal("failed"))
	assert.Expect(reports[0].ExitCode).NotTo(gomega.BeNil())
	assert.Expect(*reports[0].ExitCode).To(gomega.Equal(10))
	assert.Expect(reports[0].Parameters.Mode).To(gomega.Equal("complete"))
}
