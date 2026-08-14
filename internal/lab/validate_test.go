package lab

import (
	"strings"
	"testing"
	"time"
)

func TestValidateResultContract(t *testing.T) {
	valid := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: "run", Timestamp: time.Now(),
		Profile: "baseline", Missing: []string{"probe observation timed out"},
		Cluster:      ClusterCapacity{EligibleNodes: 3, GPUPerNode: 4, TotalGPUs: 12},
		Workloads:    []WorkloadDefinition{{Name: "holder"}},
		Measurements: Measurements{Fragmentation: Fragmentation{TargetGPUs: 4}},
	}
	if err := ValidateResult(valid); err != nil {
		t.Fatal(err)
	}
	valid.SchemaVersion = "v2"
	valid.Missing = nil
	err := ValidateResult(valid)
	if err == nil || !strings.Contains(err.Error(), "schemaVersion") ||
		!strings.Contains(err.Error(), "missing observations") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
