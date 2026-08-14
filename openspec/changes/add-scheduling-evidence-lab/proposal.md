## Why

The repository explains the AIJob-to-Pod control flow and GPU topology boundaries, but its
current demo only proves that Pods can be admitted and placed. It does not produce repeatable
evidence that a scheduling policy changes fragmentation, waiting time, or workload outcomes,
which limits both its teaching value and its relevance to the target AI Infra role.

## What Changes

- Add repeatable baseline and optimized scheduling scenarios for mixed-size simulated GPU
  workloads, with machine-readable observations and a checked-in experiment report template.
- Add a fragmentation-aware scheduling policy that preserves the existing division of
  responsibility between Kueue admission, kube-scheduler node selection, and device allocation.
- Extend the AIJob workload contract with optional container arguments and replace the blocking
  placeholder Worker with a deterministic synthetic Worker that can succeed, fail, delay, and
  expose indexed-worker context.
- Expose lab-relevant Controller and Scheduler metrics and collect end-to-end admission,
  scheduling, completion, and fragmentation measurements without requiring a real GPU.
- Add automated API-level and kind end-to-end verification for reconciliation, admission,
  placement, completion, failure, restart convergence, and cleanup.
- Add scripted failure exercises and a results/runbook structure that teaches diagnosis from
  Kubernetes objects, events, logs, and metrics.
- Correct existing documentation inconsistencies and remove or replace broken internal links.

## Capabilities

### New Capabilities

- `scheduling-benchmark`: Reproducible baseline and optimized simulated-GPU scheduling scenarios,
  measurement output, comparison rules, and experiment reporting.
- `synthetic-worker`: A deterministic indexed Worker and AIJob argument pass-through for success,
  failure, delay, and lifecycle exercises.
- `lab-observability`: Controller and Scheduler metrics plus scripted failure diagnosis using
  objects, events, logs, and collected measurements.
- `end-to-end-verification`: Automated API-level and kind verification of the complete
  AIJob-to-Worker lifecycle and controller restart convergence.

### Modified Capabilities

None. This repository does not yet contain main OpenSpec capability specifications.

## Impact

- Affected APIs: the `AIJob` CRD and Go type gain optional container arguments; existing manifests
  remain valid.
- Affected code: AIJob Controller translation, Scheduler Framework plugin and configuration,
  synthetic Worker, metrics registration, tests, and experiment tooling.
- Affected deployment: Controller/Scheduler metrics endpoints, optional monitoring resources,
  benchmark scenarios, and kind E2E targets.
- Affected documentation: README, tutorial, source-reading guide, learning roadmap, benchmark
  report, and failure runbooks.
- New test tooling may add controller-runtime `envtest` assets and shell-based kind E2E scripts;
  the normal unit-test path must remain usable without Docker or a running cluster.
- Cold-start caching, CRI/containerd profiling, Checkpoint I/O, real GPU device allocation, and
  RDMA/NCCL benchmarking remain separate future changes.
