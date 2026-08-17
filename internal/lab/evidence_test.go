package lab

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeEvidenceSource struct {
	discoveryError error
	metricsError   error
	snapshot       Snapshot
	workerLogs     map[string][]byte
}

func (f fakeEvidenceSource) Discover(context.Context, string, string) (Snapshot, error) {
	return f.snapshot, f.discoveryError
}

func (f fakeEvidenceSource) ComponentLogs(context.Context) (map[string][]byte, error) {
	return map[string][]byte{"controller-pod": []byte("reconciled\n")}, nil
}

func (f fakeEvidenceSource) WorkerLogs(
	context.Context,
	[]corev1.Pod,
) (map[string][]byte, error) {
	return f.workerLogs, nil
}

func TestGivenWorkerOutputWhenCollectingEvidenceThenRawAndSummaryFilesAreWritten(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	source := fakeEvidenceSource{
		snapshot: Snapshot{Pods: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{
			Name: "worker-0",
		}}}},
		workerLogs: map[string][]byte{"worker-0": []byte(
			"{\"type\":\"result\",\"timestamp\":\"2026-08-17T01:00:01Z\"," +
				"\"parameters\":{\"mode\":\"complete\"},\"exitReason\":\"succeeded\"," +
				"\"exitCode\":0}\n",
		)},
	}
	collector, err := NewEvidenceCollector(source, EvidenceOptions{
		RunID: "run-workers", Experiment: "worker-failure", OutputDir: t.TempDir(),
	})
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	root, err := collector.Collect(context.Background())
	workers, readErr := os.ReadFile(filepath.Join(root, "resources", "workers.json"))
	_, rawErr := os.Stat(filepath.Join(root, "logs", "workers", "worker-0.ndjson"))

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(readErr).NotTo(gomega.HaveOccurred())
	assert.Expect(rawErr).NotTo(gomega.HaveOccurred())
	assert.Expect(string(workers)).To(gomega.ContainSubstring(`"exitReason": "succeeded"`))
}

func (f fakeEvidenceSource) MetricsSnapshot(context.Context, string, int) ([]byte, error) {
	if f.metricsError != nil {
		return nil, f.metricsError
	}
	return []byte("metric_total 1\n"), nil
}

func TestGivenCompleteObservationsWhenCollectingEvidenceThenManifestIsComplete(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	collector, err := NewEvidenceCollector(fakeEvidenceSource{}, EvidenceOptions{
		RunID: "run-success", Experiment: "worker-failure", OutputDir: t.TempDir(),
		Expected: []string{"AIJob Failed"}, Observed: []string{"AIJob Failed"},
	})
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	root, err := collector.Collect(context.Background())
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(string(data)).To(gomega.ContainSubstring(`"complete": true`))
}

func TestGivenDiscoveryTimeoutWhenCollectingEvidenceThenManifestIsIncomplete(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	collector, err := NewEvidenceCollector(fakeEvidenceSource{
		discoveryError: context.DeadlineExceeded,
	}, EvidenceOptions{
		RunID: "run-timeout", Experiment: "capacity", OutputDir: t.TempDir(),
		Expected: []string{"Pod Unschedulable"},
	})
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	root, err := collector.Collect(context.Background())
	data, readErr := os.ReadFile(filepath.Join(root, "manifest.json"))

	// then
	assert.Expect(err).To(gomega.HaveOccurred())
	assert.Expect(readErr).NotTo(gomega.HaveOccurred())
	assert.Expect(string(data)).To(gomega.And(
		gomega.ContainSubstring(`"complete": false`),
		gomega.ContainSubstring("context deadline exceeded"),
	))
}

func TestGivenMetricsErrorWhenCollectingEvidenceThenWarningIsRecorded(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	collector, err := NewEvidenceCollector(fakeEvidenceSource{
		metricsError: context.DeadlineExceeded,
	}, EvidenceOptions{
		RunID: "run-metrics", Experiment: "capacity", OutputDir: t.TempDir(),
		Expected: []string{"Capacity block diagnosed"},
		Observed: []string{"Capacity block diagnosed"},
	})
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	root, err := collector.Collect(context.Background())
	data, readErr := os.ReadFile(filepath.Join(root, "manifest.json"))

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(readErr).NotTo(gomega.HaveOccurred())
	assert.Expect(string(data)).To(gomega.And(
		gomega.ContainSubstring(`"complete": true`),
		gomega.ContainSubstring(`"warnings"`),
	))
}

func TestGivenMissingRunScopeWhenCreatingEvidenceCollectorThenValidationFails(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	options := EvidenceOptions{}

	// when
	_, err := NewEvidenceCollector(fakeEvidenceSource{}, options)

	// then
	assert.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("run ID")))
}
