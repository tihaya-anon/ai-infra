## Purpose

Provide a repeatable experiment that demonstrates how node-scoring policy affects simulated GPU
fragmentation, workload waiting time, and the schedulability of a larger follow-up workload.

## ADDED Requirements

### Requirement: Comparable scheduling profiles
The lab SHALL provide baseline and optimized scheduling profiles that differ only in their
fragmentation-related node-scoring behavior. Both profiles MUST retain the same resource filters,
Kueue admission behavior, topology constraints, cluster inventory, and workload sequence.

#### Scenario: Compare profiles under equal conditions
- **WHEN** a learner runs the baseline and optimized experiments with the same benchmark settings
- **THEN** each run uses the same simulated GPU capacity, workload requests, submission order, and measurement definitions

#### Scenario: Preserve standard component boundaries
- **WHEN** the optimized profile evaluates a feasible Node
- **THEN** Kueue still owns workload admission and quota, kube-scheduler still owns Node selection, and no result claims to select a concrete GPU device

### Requirement: Deterministic fragmentation scenario
The benchmark SHALL include a mixed-size workload scenario on three Worker Nodes with four
simulated GPUs per Node. It MUST keep the small workloads active while submitting a four-GPU probe
workload so that spreading and packing behavior are observable.

#### Scenario: Baseline can create fragmentation
- **WHEN** the baseline profile spreads three two-GPU workloads across the three empty Nodes
- **THEN** the benchmark records that six GPUs are free in total while no Node can fit the four-GPU probe workload

#### Scenario: Optimized profile preserves a full Node
- **WHEN** the optimized profile packs the same three two-GPU workloads
- **THEN** the benchmark records whether at least one Node retains four free GPUs and whether the probe workload becomes schedulable

### Requirement: Defined fragmentation metric
For a target Pod requesting `G` GPUs, the benchmark SHALL calculate target-relative GPU
fragmentation as `(totalFree - usableFree) / totalFree`, where `usableFree` is
`sum(floor(nodeFree / G) * G)` over eligible Nodes. The reported value SHALL be zero when
`totalFree` is zero and SHALL identify `G` and the eligible topology domain used in the calculation.

#### Scenario: Fragmented six-GPU remainder
- **WHEN** three eligible Nodes each have two free GPUs and the target request is four GPUs
- **THEN** the benchmark reports six total free GPUs, zero usable free GPUs, and a fragmentation ratio of one

#### Scenario: One full Node remains
- **WHEN** eligible Nodes have zero, two, and four free GPUs and the target request is four GPUs
- **THEN** the benchmark reports six total free GPUs, four usable free GPUs, and a fragmentation ratio of one third within numeric output tolerance

### Requirement: Machine-readable benchmark results
Every benchmark run SHALL emit a machine-readable result containing the run identifier, timestamp,
software versions, scheduler profile, cluster capacity, workload definitions, relevant lifecycle
timestamps, Pod placements, outcomes, and aggregate measurements. Aggregate measurements MUST
include admission wait, scheduling latency, makespan, unschedulable count, and target-relative GPU
fragmentation.

#### Scenario: Successful result collection
- **WHEN** all benchmark workloads reach their expected terminal or waiting condition before timeout
- **THEN** the collector writes a complete result document and exits successfully

#### Scenario: Incomplete experiment
- **WHEN** a required observation is missing or a workload does not reach its expected condition before timeout
- **THEN** the collector marks the run incomplete, records the missing observation, and exits non-zero

### Requirement: Reproducible execution and reporting
The benchmark SHALL support configurable timeout and repetition count, SHALL clean only resources
created by its own run, and SHALL provide a report template that separates measured results from
expected behavior and interpretation.

#### Scenario: Repeat an experiment
- **WHEN** a learner requests multiple repetitions
- **THEN** the benchmark produces one independently identified result per profile and repetition

#### Scenario: Generate a teaching report
- **WHEN** benchmark results are summarized in the repository report
- **THEN** the report identifies the command, environment, raw result files, limitations, and measured comparison without presenting simulated GPUs as real hardware results
