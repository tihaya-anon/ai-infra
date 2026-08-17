package lab

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
)

func TestGivenBenchmarkResultsWhenValidatingThenContractViolationsAreReported(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	valid := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: "run", Timestamp: time.Now(),
		Profile: "baseline", Missing: []string{"probe observation timed out"},
		Cluster:      ClusterCapacity{EligibleNodes: 3, GPUPerNode: 4, TotalGPUs: 12},
		Workloads:    []WorkloadDefinition{{Name: "holder"}},
		Measurements: Measurements{Fragmentation: Fragmentation{TargetGPUs: 4}},
	}

	// when
	validError := ValidateResult(valid)
	valid.SchemaVersion = "v2"
	valid.Missing = nil
	invalidError := ValidateResult(valid)

	// then
	assert.Expect(validError).NotTo(gomega.HaveOccurred())
	assert.Expect(invalidError).To(gomega.MatchError(gomega.And(
		gomega.ContainSubstring("schemaVersion"),
		gomega.ContainSubstring("missing observations"),
	)))
}
