## 1. AIJob Workload Contract

- [x] 1.1 Add optional ordered `args` to the AIJob Go type and CRD schema, implement slice-safe deep copy, and add API serialization tests.
- [x] 1.2 Propagate AIJob arguments and the indexed completion identity environment variable into the generated Worker container without changing omitted-argument behavior.
- [x] 1.3 Extend Controller transformation tests for argument ordering, omitted arguments, completion identity injection, and preservation of externally owned fields.

## 2. Synthetic Worker

- [x] 2.1 Extract a testable Worker package with validated complete/wait modes, duration, startup delay, failure-index selection, and direct-execution index handling.
- [x] 2.2 Emit newline-delimited JSON start/result records and implement bounded SIGINT/SIGTERM handling with stable success, failure, validation, and termination exit behavior.
- [x] 2.3 Add unit tests for success, selected-index failure, wait cancellation, delayed execution, invalid flags, structured output, and missing completion index.
- [x] 2.4 Add bounded-success and selected-failure AIJob examples while retaining an explicit long-running holder example for benchmarks.

## 3. Component Observability

- [x] 3.1 Register low-cardinality Controller reconciliation, error, JobSet-change, status-change, and duration metrics, then test operation/result classification and registration.
- [x] 3.2 Register low-cardinality Scheduler plugin score, topology/fabric, error, and duration metrics, then test bounded label normalization and error accounting.
- [x] 3.3 Expose Controller and authenticated Scheduler metrics through Kubernetes Services and verify that both endpoints can be retrieved without installing Prometheus.

## 4. API-Level And Kind Verification

- [x] 4.1 Add pinned envtest setup and a separate `make test-api` target that does not affect `make verify` or require Docker.
- [x] 4.2 Add envtest reconciliation cases for one owned JobSet, desired Worker fields and arguments, status projection, repeated reconciliation, and preservation of defaulted fields.
- [x] 4.3 Add a context-guarded, timeout-bounded `make test-e2e` entry point with run labels, scoped cleanup, and automatic diagnostics on failure.
- [x] 4.4 Verify the deployed successful AIJob lifecycle from AIJob through Workload, Job, and Pod to the terminal condition.
- [x] 4.5 Verify selected Worker failure propagation through Pod, Job, JobSet, and AIJob conditions.
- [x] 4.6 Verify Controller Pod replacement converges to exactly one owned JobSet and AIJob deletion garbage-collects descendant resources.

## 5. Scheduling Benchmark

- [x] 5.1 Add baseline LeastAllocated and optimized MostAllocated Scheduler configurations that differ only in simulated-GPU scoring and retain the topology plugin and default filters.
- [x] 5.2 Add versioned benchmark result types and unit-tested calculations for free simulated GPUs, eligible Nodes, lifecycle durations, unschedulable observations, and target-relative fragmentation.
- [x] 5.3 Implement typed Kubernetes discovery and condition waiting for AIJobs, JobSets, Kueue Workloads, Jobs, Pods, Nodes, Deployments, events, logs, and metrics snapshots.
- [x] 5.4 Implement the sequential three-holder and four-GPU-probe benchmark with configurable profiles, repetitions, and timeouts.
- [x] 5.5 Make the benchmark validate cluster fixtures, switch and restore Scheduler configuration, isolate resources by run ID, collect partial results, and clean only its own resources on every exit path.
- [x] 5.6 Add Make targets and ignored output paths for executing the benchmark and writing one schema-versioned JSON result per profile and repetition.
- [x] 5.7 Add calculation fixtures proving fragmentation ratios of one for `[2,2,2]` and one third for `[0,2,4]` with a four-GPU target.

## 6. Failure Exercises And Evidence

- [x] 6.1 Add isolated insufficient-quota/capacity, selected-Worker-failure, and Controller-restart exercise commands with explicit expected conditions and finite timeouts.
- [x] 6.2 Write run-scoped evidence bundles containing a manifest, relevant resource YAML, events, component logs, metrics snapshots, completeness state, and expected-versus-observed summary.
- [x] 6.3 Test successful and timed-out evidence collection, including non-zero incomplete outcomes and cleanup that cannot select unrelated resources.

## 7. Teaching Documentation

- [x] 7.1 Correct the unsupported `preScore` tutorial example and remove or replace broken learning-note links.
- [x] 7.2 Document Worker modes, argument pass-through, metrics retrieval, the three verification layers, and each failure-diagnosis path.
- [x] 7.3 Add a scheduling experiment guide and report template covering commands, environment, metric definitions, raw evidence references, limitations, and measured-versus-expected results.
- [x] 7.4 Update README, tutorial, source-reading guide, and learning roadmap so readers can follow the code path and execute unit, API, E2E, benchmark, and failure labs independently.

## 8. Final Validation

- [x] 8.1 Run formatting, line-length checks, `go vet`, and all cluster-independent unit tests with `make verify`.
- [x] 8.2 Run `make test-api` from a clean envtest asset state and confirm the pinned setup is reproducible.
- [ ] 8.3 Run kind E2E and all failure exercises, retain diagnostics for any failed attempt, and confirm scoped cleanup and Scheduler configuration restoration.
- [ ] 8.4 Run repeated baseline and optimized benchmarks, publish only genuinely measured results with environment metadata, and verify all raw JSON against the versioned result contract.
- [ ] 8.5 Validate the completed OpenSpec change strictly and reconcile any implementation or documentation drift against all four capability specs.
