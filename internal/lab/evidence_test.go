package lab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeEvidenceSource struct {
	discoveryError error
	metricsError   error
}

func (f fakeEvidenceSource) Discover(context.Context, string, string) (Snapshot, error) {
	return Snapshot{}, f.discoveryError
}

func (f fakeEvidenceSource) ComponentLogs(context.Context) (map[string][]byte, error) {
	return map[string][]byte{"controller-pod": []byte("reconciled\n")}, nil
}

func (f fakeEvidenceSource) MetricsSnapshot(context.Context, string, int) ([]byte, error) {
	if f.metricsError != nil {
		return nil, f.metricsError
	}
	return []byte("metric_total 1\n"), nil
}

func TestEvidenceCollectionComplete(t *testing.T) {
	collector, err := NewEvidenceCollector(fakeEvidenceSource{}, EvidenceOptions{
		RunID: "run-success", Experiment: "worker-failure", OutputDir: t.TempDir(),
		Expected: []string{"AIJob Failed"}, Observed: []string{"AIJob Failed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"complete": true`) {
		t.Fatalf("unexpected manifest: %s", data)
	}
}

func TestTimedOutEvidenceIsWrittenAndReturnsError(t *testing.T) {
	collector, err := NewEvidenceCollector(fakeEvidenceSource{
		discoveryError: context.DeadlineExceeded,
	}, EvidenceOptions{
		RunID: "run-timeout", Experiment: "capacity", OutputDir: t.TempDir(),
		Expected: []string{"Pod Unschedulable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := collector.Collect(context.Background())
	if err == nil {
		t.Fatal("incomplete evidence must return non-zero")
	}
	data, readErr := os.ReadFile(filepath.Join(root, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"complete": false`) ||
		!strings.Contains(string(data), "context deadline exceeded") {
		t.Fatalf("timeout was not preserved in manifest: %s", data)
	}
}

func TestMetricsErrorsAreWarnings(t *testing.T) {
	collector, err := NewEvidenceCollector(fakeEvidenceSource{
		metricsError: context.DeadlineExceeded,
	}, EvidenceOptions{
		RunID: "run-metrics", Experiment: "capacity", OutputDir: t.TempDir(),
		Expected: []string{"Capacity block diagnosed"},
		Observed: []string{"Capacity block diagnosed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := os.ReadFile(filepath.Join(root, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"complete": true`) ||
		!strings.Contains(string(data), `"warnings"`) {
		t.Fatalf("metrics warning should not make evidence incomplete: %s", data)
	}
}

func TestEvidenceOptionsRequireRunScope(t *testing.T) {
	_, err := NewEvidenceCollector(fakeEvidenceSource{}, EvidenceOptions{})
	if err == nil || !strings.Contains(err.Error(), "run ID") {
		t.Fatal("expected validation error")
	}
}
