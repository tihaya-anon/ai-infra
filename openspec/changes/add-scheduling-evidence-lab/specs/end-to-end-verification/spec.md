## Purpose

Provide layered automated verification that proves API reconciliation and the deployed
AIJob-to-Kueue-to-Scheduler-to-Worker flow while keeping fast local checks independent of Docker.

## ADDED Requirements

### Requirement: Fast verification remains cluster-independent
The default unit and static verification path SHALL run without Docker, kind, kubectl, or a live
Kubernetes cluster. Cluster-dependent verification MUST use separate explicit targets.

#### Scenario: Run normal verification
- **WHEN** a developer runs the repository's default verification target with Go dependencies available
- **THEN** formatting, static analysis, and unit tests complete without downloading Kubernetes test binaries or creating a kind cluster

### Requirement: API-level reconciliation verification
An explicit API-level test target SHALL verify AIJob creation, JobSet ownership and desired fields,
argument propagation, status projection, and idempotent repeated reconciliation against a
Kubernetes API implementation with the required CRDs installed. It MUST NOT require Docker or a
kind cluster and SHALL manage its required test control-plane assets separately from the default
verification target.

#### Scenario: AIJob creates one owned JobSet
- **WHEN** an AIJob is created and reconciled through the API-level test environment
- **THEN** exactly one JobSet exists with the expected OwnerReference, Worker template, resources, topology intent, and arguments

#### Scenario: JobSet status changes
- **WHEN** the owned JobSet receives a terminal condition
- **THEN** the AIJob exposes the corresponding condition with its current observed generation

#### Scenario: Reconciliation repeats
- **WHEN** the same AIJob is reconciled repeatedly without desired-state changes
- **THEN** no duplicate JobSet is created and externally owned or defaulted fields remain intact

### Requirement: Kind lifecycle verification
The repository SHALL provide an explicit kind E2E target that installs pinned dependencies and
verifies AIJob creation, Kueue admission, expected Pod placement, successful completion, selected
Worker failure propagation, Controller restart convergence, and owner-based cleanup using the
deployed controllers and scheduler.

#### Scenario: Successful full lifecycle
- **WHEN** the kind E2E target submits a bounded-success AIJob
- **THEN** the test observes the AIJob-to-JobSet-to-Workload-to-Job-to-Pod chain and a successful terminal condition before timeout

#### Scenario: Failed full lifecycle
- **WHEN** the kind E2E target submits an AIJob whose selected Worker fails
- **THEN** the test observes the non-zero Pod outcome and the corresponding terminal failure through Job, JobSet, and AIJob status before timeout

#### Scenario: Controller restart convergence
- **WHEN** the E2E target replaces the Controller Pod after the owned JobSet exists
- **THEN** the replacement Controller becomes ready and the AIJob still converges with exactly one owned JobSet

#### Scenario: AIJob is deleted
- **WHEN** the E2E target deletes an AIJob after its owned resources exist
- **THEN** the JobSet and its descendant Jobs and Pods are removed before timeout

### Requirement: Safe and diagnosable E2E execution
The E2E target SHALL verify that the current Kubernetes context is the explicitly named lab kind
cluster before mutation, SHALL apply finite timeouts to every wait, and SHALL collect diagnostic
evidence before returning a failure. Cleanup MUST be restricted to the named lab cluster or
run-labeled resources.

#### Scenario: Kubernetes context does not match
- **WHEN** the current context is not the explicitly named lab kind cluster
- **THEN** the E2E target exits before applying, deleting, or patching Kubernetes resources

#### Scenario: E2E assertion times out
- **WHEN** an expected lifecycle condition is not reached before timeout
- **THEN** the target captures relevant objects, events, and component logs and exits non-zero
