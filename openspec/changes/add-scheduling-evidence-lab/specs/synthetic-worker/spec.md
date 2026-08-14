## Purpose

Provide a deterministic, inspectable workload process for demonstrating indexed AI job success,
failure, delay, long-running occupancy, termination, and status propagation without a real model.

## ADDED Requirements

### Requirement: AIJob argument pass-through
The AIJob API SHALL accept an optional ordered list of container arguments and the Controller SHALL
preserve that order in the generated Worker Pod template. Existing AIJobs that omit the field MUST
remain valid and continue to invoke the repository Worker using its compatible default behavior.

#### Scenario: Arguments are supplied
- **WHEN** an AIJob specifies Worker arguments
- **THEN** every generated Worker container receives exactly those arguments in the declared order

#### Scenario: Arguments are omitted
- **WHEN** an existing AIJob omits Worker arguments
- **THEN** admission and JobSet generation succeed without requiring a manifest migration

### Requirement: Deterministic Worker modes
The synthetic Worker SHALL support bounded success, bounded failure, delayed execution, and
run-until-terminated modes. It SHALL allow failure to be selected by indexed Worker identity so a
multi-Worker JobSet can demonstrate partial execution followed by JobSet failure handling.

#### Scenario: Successful Worker
- **WHEN** a Worker is configured for bounded success
- **THEN** it waits for the configured duration, records a successful result, and exits with code zero

#### Scenario: Selected Worker fails
- **WHEN** the Worker's completion index is included in the configured failure indexes
- **THEN** that Worker records a failed result and exits with a stable non-zero code after the configured duration

#### Scenario: Worker occupies resources
- **WHEN** a Worker is configured to run until terminated
- **THEN** it remains active for the scheduling benchmark until it receives a termination signal

### Requirement: Indexed execution context
The Worker SHALL discover its indexed Job completion identity when Kubernetes provides one and
SHALL expose the discovered value in its lifecycle output. Absence of an index MUST be handled
without crashing so the binary remains directly runnable.

#### Scenario: Indexed Job execution
- **WHEN** Kubernetes starts the Worker for an Indexed Job
- **THEN** the Worker's start and result records contain the completion index

#### Scenario: Direct execution
- **WHEN** the Worker runs outside an Indexed Job
- **THEN** it reports that no index is available and continues according to its configured mode

### Requirement: Structured lifecycle output
The Worker SHALL emit machine-readable start and result records containing timestamp, mode,
completion index when available, configured duration, and outcome. Invalid arguments MUST produce
a diagnostic and non-zero exit without entering the workload mode.

#### Scenario: Parse successful lifecycle
- **WHEN** a bounded Worker completes
- **THEN** its output contains one start record and one terminal result record that can be associated with the same execution

#### Scenario: Reject invalid mode
- **WHEN** the Worker receives an unsupported mode
- **THEN** it emits an actionable validation error and exits non-zero before reporting a start record

### Requirement: Graceful termination
The Worker SHALL handle normal Kubernetes termination signals, emit a terminal result identifying
termination, and exit within a bounded grace period.

#### Scenario: Long-running Worker is deleted
- **WHEN** Kubernetes sends a termination signal to a run-until-terminated Worker
- **THEN** the Worker records the termination outcome and exits before its Pod termination grace period expires
