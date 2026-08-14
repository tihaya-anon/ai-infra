## Context

See `proposal.md` for motivation and the four capability specs for observable behavior. The current
lab has one `ai-scheduler` profile, a topology-only `ScorePlugin`, a thin AIJob-to-JobSet Controller,
and a Worker that blocks until termination. kind provides three four-GPU Worker Nodes by patching a
simulated extended resource; there is no Device Plugin, real GPU, Prometheus installation, or CI
environment guaranteed.

The change crosses the API, Controller, Scheduler, Worker, deployment, experiment tooling, tests,
and documentation. The design therefore keeps the production-shaped control path small and puts
measurement and failure orchestration in a separate lab command.

## Goals / Non-Goals

**Goals:**

- Produce a deterministic teaching experiment in which spreading creates target-relative GPU
  fragmentation and packing preserves a Node for a larger Pod.
- Make every claimed outcome derivable from Kubernetes API state and versioned JSON results.
- Exercise success, failure, long-running occupancy, reconciliation, restart, and cleanup without
  introducing a real training framework.
- Keep low-cardinality operational metrics inspectable without installing a monitoring stack.
- Preserve a fast default developer loop while adding explicit API-level and kind E2E layers.

**Non-Goals:**

- Selecting individual GPU devices, simulating GPU execution, or claiming hardware utilization,
  NVLink bandwidth, or production-scale performance.
- Reimplementing Kueue gang admission, kube-scheduler resource filtering, or Device Plugin/DRA
  responsibilities.
- Adding cold-start caching, Checkpoint storage, autoscaling, RDMA/NCCL, CRI profiling, or an
  observability dashboard in this change.
- Turning the synthetic Worker flags into a production training API. AIJob only passes opaque
  ordered arguments to the selected image.

## Decisions

### Use standard NodeResourcesFit strategies for the fragmentation comparison

The baseline scheduler configuration will explicitly score `example.com/gpu` with
`LeastAllocated`; the optimized configuration will score the same resource with `MostAllocated`.
Both configurations keep the existing topology plugin, default filters, scheduler name, and all
other settings equal. Benchmark workloads use `topology: any`, so topology scores do not mask the
resource-allocation comparison.

This makes the lesson about choosing and measuring the correct extension point: standard
NodeResourcesFit already implements resource packing, while the custom plugin remains responsible
only for the cluster-specific fabric preference. A new fragmentation plugin was rejected because
it would duplicate standard behavior and weaken the repository's component-boundary lesson.

Profiles run sequentially under the same `ai-scheduler` name. The runner applies one pinned
configuration, waits for the Scheduler rollout, verifies an empty run scope, executes the scenario,
collects evidence, cleans up, and then applies the other configuration. Adding a public
`schedulingPolicy` field to AIJob was rejected because baseline-versus-optimized is a lab concern,
not workload intent.

### Make the fragmentation scenario sequential and target-relative

Each profile run submits three one-Worker holder AIJobs requesting two GPUs each. The runner waits
for each holder Pod to become scheduled before submitting the next, which lets LeastAllocated
spread and MostAllocated pack predictably across three four-GPU Nodes. Holders run until terminated.
After their placements are stable, the runner snapshots free capacity and submits one four-GPU
probe AIJob.

Fragmentation is evaluated relative to the probe request using the formula in the scheduling spec.
This is preferable to a generic percentage because a remainder is only fragmented relative to a
particular demand shape. Admission quota is kept high enough for all holders and the probe, so an
unscheduled probe in this scenario identifies Node-level fit rather than Kueue quota exhaustion.

### Add only opaque container arguments to AIJob

`AIJobSpec` gains `Args []string` with the corresponding CRD array schema. The Controller copies
the slice to `Container.Args` and continues to invoke `/worker` for the repository's default image.
Omitting arguments retains the current long-running Worker behavior, so existing manifests stay
valid. A generic PodTemplate was rejected because it would substantially expand API ownership and
validation beyond the teaching objective.

The Worker uses standard flag parsing with `--mode=complete|wait`, `--duration`,
`--startup-delay`, and `--fail-indexes`. Default mode is `wait`. A selected failure index overrides
the successful result of bounded completion. The Pod template injects `JOB_COMPLETION_INDEX` from
the Indexed Job Pod label through the downward API. The Worker emits newline-delimited JSON start
and terminal records and handles SIGINT/SIGTERM through a cancellation context.

### Implement orchestration and collection as a Go lab command

A repository command, provisionally `cmd/labctl`, owns benchmark and failure-exercise orchestration.
It uses typed Kubernetes clients and structured JSON encoding for object inspection, waiting,
calculation, and evidence collection. Shell remains limited to the existing Make targets and
cluster bootstrap.

This was chosen over a Bash/`kubectl`/`jq` pipeline because lifecycle timestamps, quantities,
conditions, repeated runs, partial-result handling, and schema evolution are structured-data
problems. The command writes schema-versioned results under a configurable output directory,
defaulting to an ignored `out/` tree. A checked-in report template references raw results when a
measured run is intentionally published; no expected or fabricated values are recorded as
measurements.

The collector derives:

- admission wait from Workload creation to its Admitted condition transition;
- scheduling latency from Pod creation to its PodScheduled condition transition;
- makespan from scenario submission to the defined expected probe condition;
- unschedulable count from Pods with unschedulable scheduling conditions during the observation;
- free simulated GPUs from Node allocatable capacity minus bound non-terminal Pod requests;
- target-relative fragmentation from the formula and eligible Node set in the spec.

### Register bounded metrics in existing component registries

Controller metrics register with controller-runtime's metrics registry. Scheduler plugin metrics
register with the Kubernetes component metrics registry already served by kube-scheduler. Labels
are enums such as operation, result, preference, and observed fabric; resource names are never
metric labels.

The Controller deployment exposes its metrics port through a ClusterIP Service. Scheduler metrics
remain on kube-scheduler's authenticated HTTPS endpoint and are exposed by a Service. The tutorial
documents token-authenticated port-forward retrieval. A ServiceMonitor and full Prometheus stack
were rejected because scrape discovery is orthogonal to demonstrating metric production.

### Use three verification layers

Pure transformation, Worker flag/lifecycle, metric-label, and fragmentation calculations remain
ordinary Go unit tests under `make verify`. API-level reconciliation tests run through a separate
`make test-api` target backed by controller-runtime envtest with pinned Kubernetes test assets and
the AIJob and JobSet CRDs. Garbage collection is not asserted in envtest because its control plane
does not run the Kubernetes garbage-collector controller.

`make test-e2e` targets an explicitly named kind cluster and tests the deployed JobSet, Kueue,
Controller, Scheduler, Worker, and garbage collection path. All waits have timeouts. On failure,
labctl collects run-labeled objects, events, logs, and metrics before cleanup. The target refuses to
mutate a context other than `kind-<explicit-cluster-name>`.

### Keep benchmark and failure evidence run-scoped

All generated AIJobs and descendant resources carry a bounded experiment kind plus a unique run ID
label. Queries and deletion use that run ID. Node capacity and labels are shared lab fixtures and
are validated, not deleted, by a run. Scheduler configuration changes are restored to the normal
lab configuration after benchmark completion, including failure paths.

Evidence bundles contain a manifest with tool and component versions, expected conditions, actual
conditions, and completeness state. Resource YAML, namespace-scoped events, selected component
logs, metrics snapshots, and benchmark JSON are stored in subdirectories so a failed run remains
diagnosable.

## Risks / Trade-offs

- [Scheduler tie-breaking or asynchronous Pod accounting makes placement flaky] -> Submit holders
  sequentially, wait for binding before continuing, explicitly weight only the simulated GPU
  resource for the comparison, and support repeated runs.
- [Patched extended resources do not reproduce Device Plugin allocation] -> Keep the Worker
  hardware-independent, label every result as simulated, and make no device-level claims.
- [Kueue admission obscures scheduler fragmentation] -> Set quota above combined benchmark demand
  and record Workload admission separately from Pod scheduling.
- [Metrics endpoint authentication complicates a beginner lab] -> Provide exact token and
  port-forward commands while keeping Scheduler metrics on its standard authenticated endpoint.
- [envtest asset setup slows contributors] -> Keep it behind `make test-api`, pin the asset version,
  cache it under the existing ignored tooling directory, and leave `make verify` independent.
- [A failed runner leaves holders or an alternate Scheduler configuration behind] -> Use run labels,
  deferred cleanup, configuration restoration, and a documented recovery target.
- [Synthetic Worker options leak into the domain API] -> AIJob carries only opaque image arguments;
  mode semantics remain local to the repository Worker.

## Migration Plan

1. Add the optional AIJob argument field to Go types and CRD; verify existing examples still apply.
2. Introduce the new Worker behavior with the current wait behavior as its default.
3. Add metrics and Services without changing the existing scheduling profile behavior.
4. Add envtest and kind lifecycle verification, then make the normal demo use bounded completion.
5. Add baseline/optimized Scheduler configurations, labctl, scenarios, and failure exercises.
6. Run the benchmark, retain raw evidence outside version control by default, and publish only
   explicitly measured results with environment metadata.

Rollback removes the new scenarios, Services, metrics, and optional `args` usage. Existing AIJobs
that omit `args` remain compatible throughout; CRD storage does not require conversion because the
new field is optional in the existing `v1alpha1` version.
