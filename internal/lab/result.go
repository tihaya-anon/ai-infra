// Package lab implements typed observation and orchestration for the teaching lab.
package lab

import "time"

const ResultSchemaVersion = "v2"

// BenchmarkResult is one profile and repetition observation document.
type BenchmarkResult struct {
	SchemaVersion string               `json:"schemaVersion"`
	RunID         string               `json:"runId"`
	Timestamp     time.Time            `json:"timestamp"`
	Complete      bool                 `json:"complete"`
	Missing       []string             `json:"missing,omitempty"`
	Environment   Environment          `json:"environment"`
	Profile       string               `json:"profile"`
	Cluster       ClusterCapacity      `json:"cluster"`
	Workloads     []WorkloadDefinition `json:"workloads"`
	Lifecycle     []Lifecycle          `json:"lifecycle"`
	Placements    []PodPlacement       `json:"placements"`
	Workers       []WorkerRuntime      `json:"workers"`
	Outcomes      map[string]string    `json:"outcomes"`
	Measurements  Measurements         `json:"measurements"`
	Evidence      []string             `json:"evidence,omitempty"`
}

// Environment identifies the software and cluster used for a measured run.
type Environment struct {
	KubernetesVersion string            `json:"kubernetesVersion"`
	ToolVersion       string            `json:"toolVersion"`
	Components        map[string]string `json:"components"`
}

// ClusterCapacity records only the simulated resource fixture used by this lab.
type ClusterCapacity struct {
	EligibleNodes int   `json:"eligibleNodes"`
	GPUPerNode    int64 `json:"gpuPerNode"`
	TotalGPUs     int64 `json:"totalSimulatedGpus"`
}

// WorkloadDefinition makes the submitted sequence inspectable.
type WorkloadDefinition struct {
	Name         string   `json:"name"`
	Workers      int32    `json:"workers"`
	GPUPerWorker int64    `json:"gpuPerWorker"`
	Args         []string `json:"args"`
}

// Lifecycle records API timestamps and their derived durations.
type Lifecycle struct {
	Name                     string     `json:"name"`
	SubmittedAt              time.Time  `json:"submittedAt"`
	WorkloadCreatedAt        *time.Time `json:"workloadCreatedAt,omitempty"`
	PodCreatedAt             *time.Time `json:"podCreatedAt,omitempty"`
	AdmittedAt               *time.Time `json:"admittedAt,omitempty"`
	ScheduledAt              *time.Time `json:"scheduledAt,omitempty"`
	TerminalAt               *time.Time `json:"terminalAt,omitempty"`
	AdmissionWaitSeconds     *float64   `json:"admissionWaitSeconds,omitempty"`
	SchedulingLatencySeconds *float64   `json:"schedulingLatencySeconds,omitempty"`
}

// PodPlacement connects a worker index to its selected Node and outcome.
type PodPlacement struct {
	Pod             string `json:"pod"`
	Workload        string `json:"workload"`
	CompletionIndex string `json:"completionIndex,omitempty"`
	Node            string `json:"node,omitempty"`
	Phase           string `json:"phase"`
}

// WorkerRuntime summarizes one worker process from its Pod state and NDJSON records.
type WorkerRuntime struct {
	Pod             string           `json:"pod"`
	Workload        string           `json:"workload"`
	CompletionIndex string           `json:"completionIndex,omitempty"`
	StartedAt       *time.Time       `json:"startedAt,omitempty"`
	FinishedAt      *time.Time       `json:"finishedAt,omitempty"`
	ExitReason      string           `json:"exitReason,omitempty"`
	ExitCode        *int             `json:"exitCode,omitempty"`
	Parameters      WorkerParameters `json:"parameters"`
}

// WorkerParameters is the normalized argument summary emitted by the worker.
type WorkerParameters struct {
	Mode         string `json:"mode,omitempty"`
	Duration     string `json:"duration,omitempty"`
	StartupDelay string `json:"startupDelay,omitempty"`
	FailIndexes  []int  `json:"failIndexes,omitempty"`
}

// Measurements contains the aggregate quantities compared across profiles.
type Measurements struct {
	MakespanSeconds    *float64      `json:"makespanSeconds,omitempty"`
	UnschedulableCount int           `json:"unschedulableCount"`
	Fragmentation      Fragmentation `json:"fragmentation"`
	Recovery           Recovery      `json:"recovery"`
}

// Recovery measures whether releasing one holder unblocked the fragmented probe.
type Recovery struct {
	Attempted              bool       `json:"attempted"`
	InitiallyUnschedulable bool       `json:"initiallyUnschedulable"`
	ReleasedWorkload       string     `json:"releasedWorkload,omitempty"`
	UnschedulableAt        *time.Time `json:"unschedulableAt,omitempty"`
	ReleasedAt             *time.Time `json:"releasedAt,omitempty"`
	RecoveredAt            *time.Time `json:"recoveredAt,omitempty"`
	LatencySeconds         *float64   `json:"latencySeconds,omitempty"`
	Recovered              bool       `json:"recovered"`
}

// Fragmentation is target-relative simulated-GPU capacity evidence.
type Fragmentation struct {
	TargetGPUs     int64            `json:"targetGpus"`
	TopologyDomain string           `json:"topologyDomain"`
	EligibleNodes  []string         `json:"eligibleNodes"`
	FreeByNode     map[string]int64 `json:"freeByNode"`
	TotalFree      int64            `json:"totalFree"`
	UsableFree     int64            `json:"usableFree"`
	Ratio          float64          `json:"ratio"`
}
