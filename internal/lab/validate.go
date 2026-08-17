package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// ValidateResultFile strictly decodes and validates one versioned benchmark result.
func ValidateResultFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	result := BenchmarkResult{}
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode %s: trailing JSON content", path)
	}
	return ValidateResult(result)
}

// ValidateResult checks the current semantic contract, including partial-run consistency.
func ValidateResult(result BenchmarkResult) error {
	var problems []error
	if result.SchemaVersion != ResultSchemaVersion {
		problems = append(problems, fmt.Errorf("unsupported schemaVersion %q", result.SchemaVersion))
	}
	if result.RunID == "" || result.Timestamp.IsZero() {
		problems = append(problems, errors.New("runId and timestamp are required"))
	}
	if result.Profile != "baseline" && result.Profile != "optimized" {
		problems = append(problems, fmt.Errorf("unsupported profile %q", result.Profile))
	}
	if result.Cluster.EligibleNodes != 3 || result.Cluster.GPUPerNode != 4 ||
		result.Cluster.TotalGPUs != 12 {
		problems = append(problems, errors.New("unexpected simulated cluster capacity"))
	}
	if result.Complete && len(result.Missing) != 0 {
		problems = append(problems, errors.New("complete result must not contain missing observations"))
	}
	if !result.Complete && len(result.Missing) == 0 {
		problems = append(problems, errors.New("incomplete result must identify missing observations"))
	}
	fragmentation := result.Measurements.Fragmentation
	if len(result.Workloads) > 0 && fragmentation.TargetGPUs != 4 {
		problems = append(problems, errors.New("benchmark fragmentation target must be four GPUs"))
	}
	if fragmentation.Ratio < 0 || fragmentation.Ratio > 1 {
		problems = append(problems, errors.New("fragmentation ratio must be within [0,1]"))
	}
	if result.Complete && result.Profile == "baseline" {
		recovery := result.Measurements.Recovery
		if !recovery.Attempted || !recovery.InitiallyUnschedulable || !recovery.Recovered ||
			recovery.ReleasedWorkload == "" || recovery.ReleasedAt == nil ||
			recovery.RecoveredAt == nil || recovery.LatencySeconds == nil {
			problems = append(problems, errors.New(
				"complete baseline result must include successful release recovery",
			))
		} else if *recovery.LatencySeconds < 0 {
			problems = append(problems, errors.New("recovery latency must not be negative"))
		}
	}
	if result.Complete && result.Profile == "optimized" && result.Measurements.Recovery.Attempted {
		problems = append(problems, errors.New("optimized result must not attempt release recovery"))
	}
	return errors.Join(problems...)
}
