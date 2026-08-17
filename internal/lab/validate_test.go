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
	valid.SchemaVersion = "v999"
	valid.Missing = nil
	invalidError := ValidateResult(valid)

	// then
	assert.Expect(validError).NotTo(gomega.HaveOccurred())
	assert.Expect(invalidError).To(gomega.MatchError(gomega.And(
		gomega.ContainSubstring("schemaVersion"),
		gomega.ContainSubstring("missing observations"),
	)))
}

func TestGivenCompleteBaselineWithoutRecoveryWhenValidatingThenRecoveryIsRequired(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	result := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: "run", Timestamp: time.Now(),
		Profile: "baseline", Complete: true,
		Cluster: ClusterCapacity{EligibleNodes: 3, GPUPerNode: 4, TotalGPUs: 12},
	}

	// when
	err := ValidateResult(result)

	// then
	assert.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("release recovery")))
}
