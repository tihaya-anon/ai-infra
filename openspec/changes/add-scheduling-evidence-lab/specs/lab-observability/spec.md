## Purpose

Make control-loop behavior, scheduling decisions, benchmark outcomes, and injected failures
observable through bounded metrics and a repeatable evidence-based diagnostic workflow.

## ADDED Requirements

### Requirement: Controller metrics
The AIJob Controller SHALL expose counters and duration measurements for reconciliation attempts,
errors, JobSet changes, and status changes. Metric labels MUST use bounded categories and MUST NOT
contain AIJob names, Pod names, namespaces, or other unbounded resource identifiers.

#### Scenario: Successful reconciliation is observed
- **WHEN** an AIJob reconciliation creates or updates its desired JobSet and status
- **THEN** the Controller metrics endpoint reflects the attempt, duration, and performed change categories

#### Scenario: Reconciliation fails
- **WHEN** the Controller returns an error from a reconciliation attempt
- **THEN** the error counter increases under a bounded error category

### Requirement: Scheduler policy metrics
The custom Scheduler plugin SHALL expose bounded counters and duration measurements for score
evaluations, requested topology categories, observed fabric categories, and plugin errors. Metrics
MUST NOT identify individual Pods, AIJobs, or Nodes.

#### Scenario: Topology preference is scored
- **WHEN** the plugin scores eligible Nodes for a supported topology preference
- **THEN** the metrics endpoint records the preference and bounded observed-fabric categories

#### Scenario: Plugin input is invalid
- **WHEN** the plugin cannot score because required framework state is unavailable
- **THEN** a bounded plugin error metric is recorded and the scheduling error remains visible to kube-scheduler

### Requirement: Metrics are usable without a monitoring stack
The lab SHALL make Controller and Scheduler metrics retrievable from inside the kind cluster using
standard Kubernetes access. Installing Prometheus or Grafana MUST NOT be required to execute or
verify the lab.

#### Scenario: Learner inspects metrics
- **WHEN** the lab components are ready
- **THEN** the documented command retrieves their Prometheus text endpoints through Kubernetes

### Requirement: Scripted failure exercises
The lab SHALL provide isolated exercises for insufficient quota or capacity, selected Worker
failure, and Controller restart. Each exercise MUST define the expected Kubernetes conditions,
events, logs, and metrics used to distinguish the failure stage.

#### Scenario: Capacity failure is diagnosed
- **WHEN** a workload cannot be admitted or scheduled because simulated GPU capacity is insufficient
- **THEN** the exercise identifies whether it is waiting at Kueue admission or kube-scheduler placement using observable evidence

#### Scenario: Worker failure is diagnosed
- **WHEN** a selected indexed Worker exits non-zero
- **THEN** the exercise traces the failure from Pod and Job status through JobSet and AIJob conditions

#### Scenario: Controller restarts
- **WHEN** the AIJob Controller Pod is replaced while an AIJob exists
- **THEN** the exercise shows recovery and reconciliation without duplicate owned JobSets

### Requirement: Scoped evidence bundles
Each failure exercise SHALL capture a run-scoped evidence bundle containing relevant resource
descriptions, events, component logs, metrics snapshots, and a summary of expected versus observed
conditions. Collection and cleanup MUST target only resources labeled for that exercise run.

#### Scenario: Exercise completes
- **WHEN** a failure exercise reaches its expected condition
- **THEN** its evidence bundle is written before the run-scoped resources are cleaned up

#### Scenario: Exercise times out
- **WHEN** an expected condition is not reached before timeout
- **THEN** the available evidence is still collected, the missing condition is recorded, and the exercise exits non-zero
