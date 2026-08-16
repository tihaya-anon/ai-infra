package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

const EvidenceSchemaVersion = "v1"

// EvidenceSource supplies typed state and component observations.
type EvidenceSource interface {
	Discover(context.Context, string, string) (Snapshot, error)
	ComponentLogs(context.Context) (map[string][]byte, error)
	MetricsSnapshot(context.Context, string, int) ([]byte, error)
}

// EvidenceOptions identifies one run and its expected teaching outcome.
type EvidenceOptions struct {
	Namespace  string
	RunID      string
	Experiment string
	OutputDir  string
	Expected   []string
	Observed   []string
}

// EvidenceManifest is the index and completeness contract for one bundle.
type EvidenceManifest struct {
	SchemaVersion string            `json:"schemaVersion"`
	RunID         string            `json:"runId"`
	Experiment    string            `json:"experiment"`
	CollectedAt   time.Time         `json:"collectedAt"`
	Complete      bool              `json:"complete"`
	Expected      []string          `json:"expected"`
	Observed      []string          `json:"observed"`
	Missing       []string          `json:"missing,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
	Files         map[string]string `json:"files"`
}

// EvidenceCollector writes a run-scoped bundle and returns non-zero completeness errors.
type EvidenceCollector struct {
	source  EvidenceSource
	options EvidenceOptions
}

// NewEvidenceCollector validates and constructs a collector.
func NewEvidenceCollector(
	source EvidenceSource,
	options EvidenceOptions,
) (*EvidenceCollector, error) {
	if options.Namespace == "" {
		options.Namespace = "default"
	}
	if options.RunID == "" || options.Experiment == "" {
		return nil, errors.New("evidence run ID and experiment are required")
	}
	if options.OutputDir == "" {
		options.OutputDir = "out/evidence"
	}
	return &EvidenceCollector{source: source, options: options}, nil
}

// Collect writes every available artifact and reports an incomplete bundle as an error.
func (c *EvidenceCollector) Collect(ctx context.Context) (string, error) {
	root := filepath.Join(c.options.OutputDir, c.options.RunID)
	manifest := EvidenceManifest{
		SchemaVersion: EvidenceSchemaVersion, RunID: c.options.RunID,
		Experiment: c.options.Experiment, CollectedAt: time.Now().UTC(),
		Expected: append([]string(nil), c.options.Expected...),
		Observed: append([]string(nil), c.options.Observed...),
		Files:    make(map[string]string),
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return root, err
	}

	snapshot, err := c.source.Discover(ctx, c.options.Namespace, c.options.RunID)
	if err != nil {
		manifest.Missing = append(manifest.Missing, "resources: "+err.Error())
	} else {
		resources := map[string]any{
			"aijobs": snapshot.AIJobs, "jobsets": snapshot.JobSets,
			"workloads": snapshot.Workloads, "jobs": snapshot.Jobs,
			"pods": snapshot.Pods, "nodes": snapshot.Nodes,
			"deployments": snapshot.Deployments, "events": snapshot.Events,
		}
		for name, value := range resources {
			path := filepath.Join("resources", name+".yaml")
			if err := writeYAML(filepath.Join(root, path), value); err != nil {
				manifest.Missing = append(manifest.Missing, path+": "+err.Error())
			} else {
				manifest.Files[name] = path
			}
		}
	}

	logs, err := c.source.ComponentLogs(ctx)
	if err != nil {
		manifest.Missing = append(manifest.Missing, "component logs: "+err.Error())
	} else {
		for name, data := range logs {
			path := filepath.Join("logs", safeName(name)+".log")
			if err := writeFile(filepath.Join(root, path), data); err != nil {
				manifest.Missing = append(manifest.Missing, path+": "+err.Error())
			} else {
				manifest.Files["log-"+name] = path
			}
		}
	}

	services := []struct {
		name string
		port int
	}{
		{name: "aijob-controller-metrics", port: 8080},
		{name: "ai-scheduler-metrics", port: 10259},
	}
	for _, service := range services {
		data, err := c.source.MetricsSnapshot(ctx, service.name, service.port)
		path := filepath.Join("metrics", safeName(service.name)+".prom")
		if err != nil {
			manifest.Warnings = append(manifest.Warnings, service.name+": "+err.Error())
			continue
		}
		if err := writeFile(filepath.Join(root, path), data); err != nil {
			manifest.Warnings = append(manifest.Warnings, path+": "+err.Error())
		} else {
			manifest.Files[service.name] = path
		}
	}

	sort.Strings(manifest.Missing)
	sort.Strings(manifest.Warnings)
	conditionsMet := expectedObserved(manifest.Expected, manifest.Observed)
	manifest.Complete = len(manifest.Missing) == 0 && conditionsMet
	if !conditionsMet {
		manifest.Missing = append(manifest.Missing, "expected conditions were not all observed")
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return root, err
	}
	if !manifest.Complete {
		return root, fmt.Errorf("evidence bundle %s is incomplete", root)
	}
	return root, nil
}

func writeYAML(path string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(data, '\n'))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func expectedObserved(expected, observed []string) bool {
	seen := make(map[string]struct{}, len(observed))
	for _, item := range observed {
		seen[item] = struct{}{}
	}
	for _, item := range expected {
		if _, exists := seen[item]; !exists {
			return false
		}
	}
	return true
}

func safeName(value string) string {
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "..", "-")
	return value
}
