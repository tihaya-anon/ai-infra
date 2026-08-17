package lab

import (
	"context"
	"errors"
	"fmt"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

// E2EOptions controls deployed lifecycle verification.
type E2EOptions struct {
	Namespace string
	OutputDir string
	Timeout   time.Duration
}

// E2ERunner verifies the deployed success, failure, restart, and GC paths.
type E2ERunner struct {
	cluster *Cluster
	options E2EOptions
}

// NewE2ERunner validates E2E options.
func NewE2ERunner(cluster *Cluster, options E2EOptions) (*E2ERunner, error) {
	if options.Namespace == "" {
		options.Namespace = "default"
	}
	if options.OutputDir == "" {
		options.OutputDir = "out/e2e"
	}
	if options.Timeout <= 0 {
		return nil, errors.New("E2E timeout must be positive")
	}
	return &E2ERunner{cluster: cluster, options: options}, nil
}

// Run executes all E2E scenarios sequentially.
func (r *E2ERunner) Run(ctx context.Context) error {
	scenarios := []struct {
		name string
		run  func(context.Context, string) ([]string, error)
	}{
		{name: "success", run: r.runSuccess},
		{name: "selected-worker-failure", run: r.runFailure},
		{name: "controller-restart-cleanup", run: r.runRestartAndCleanup},
	}
	var scenarioErrors []error
	for _, scenario := range scenarios {
		runID := newRunID(e2eRunPrefix(scenario.name))
		observed, err := scenario.run(ctx, runID)
		if err != nil {
			collector, collectorErr := NewEvidenceCollector(r.cluster, EvidenceOptions{
				Namespace: r.options.Namespace, RunID: runID,
				Experiment: scenario.name, OutputDir: r.options.OutputDir,
				Expected: []string{"scenario complete"}, Observed: observed,
			})
			if collectorErr == nil {
				evidenceCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
				_, collectorErr = collector.Collect(evidenceCtx)
				cancel()
			}
			scenarioErrors = append(scenarioErrors, errors.Join(err, collectorErr))
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
		cleanup := &BenchmarkRunner{cluster: r.cluster, options: BenchmarkOptions{
			Namespace: r.options.Namespace, Timeout: r.options.Timeout,
		}}
		if err := cleanup.cleanup(cleanupCtx, runID); err != nil {
			scenarioErrors = append(scenarioErrors, err)
		}
		cancel()
	}
	return errors.Join(scenarioErrors...)
}

func e2eRunPrefix(scenario string) string {
	switch scenario {
	case "success":
		return "e2e-ok"
	case "selected-worker-failure":
		return "e2e-fail"
	case "controller-restart-cleanup":
		return "e2e-restart"
	default:
		return "e2e"
	}
}

func (r *E2ERunner) runSuccess(ctx context.Context, runID string) ([]string, error) {
	benchmark := r.benchmarkHelper()
	definition := WorkloadDefinition{
		Name: "success-" + runID, Workers: 2, GPUPerWorker: 1,
		Args: []string{"--mode=complete", "--duration=1s"},
	}
	if err := benchmark.createAIJob(ctx, runID, "e2e-success", definition); err != nil {
		return nil, err
	}
	if _, err := r.cluster.WaitForAIJobCondition(ctx, types.NamespacedName{
		Namespace: r.options.Namespace, Name: definition.Name,
	}, "Completed", r.options.Timeout); err != nil {
		return nil, err
	}
	snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
	if err != nil {
		return nil, err
	}
	if err := verifyLifecycle(snapshot, false); err != nil {
		return nil, err
	}
	return []string{"scenario complete"}, nil
}

func (r *E2ERunner) runFailure(ctx context.Context, runID string) ([]string, error) {
	benchmark := r.benchmarkHelper()
	definition := WorkloadDefinition{
		Name: "failure-" + runID, Workers: 2, GPUPerWorker: 1,
		Args: []string{"--mode=complete", "--duration=1s", "--fail-indexes=1"},
	}
	if err := benchmark.createAIJob(ctx, runID, "e2e-failure", definition); err != nil {
		return nil, err
	}
	if _, err := r.cluster.WaitForAIJobCondition(ctx, types.NamespacedName{
		Namespace: r.options.Namespace, Name: definition.Name,
	}, "Failed", r.options.Timeout); err != nil {
		return nil, err
	}
	snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
	if err != nil {
		return nil, err
	}
	if err := verifyLifecycle(snapshot, true); err != nil {
		return nil, err
	}
	return []string{"scenario complete"}, nil
}

func (r *E2ERunner) runRestartAndCleanup(ctx context.Context, runID string) ([]string, error) {
	benchmark := r.benchmarkHelper()
	definition := WorkloadDefinition{
		Name: "restart-" + runID, Workers: 1, GPUPerWorker: 1,
		Args: []string{"--mode=wait"},
	}
	if err := benchmark.createAIJob(ctx, runID, "e2e-restart", definition); err != nil {
		return nil, err
	}
	if _, err := r.cluster.WaitForPodScheduled(
		ctx, r.options.Namespace, runID, definition.Name, r.options.Timeout,
	); err != nil {
		return nil, err
	}
	exercise := &ExerciseRunner{cluster: r.cluster, options: ExerciseOptions{
		Namespace: r.options.Namespace, Timeout: r.options.Timeout,
	}}
	if _, err := exercise.replaceController(ctx, runID); err != nil {
		return nil, err
	}
	job := &aiv1alpha1.AIJob{}
	key := types.NamespacedName{Namespace: r.options.Namespace, Name: definition.Name}
	if err := r.cluster.Client.Get(ctx, key, job); err != nil {
		return nil, err
	}
	if err := r.cluster.Client.Delete(ctx, job); err != nil {
		return nil, err
	}
	if err := r.waitForDescendantsGone(ctx, runID); err != nil {
		return nil, err
	}
	return []string{"scenario complete"}, nil
}

func (r *E2ERunner) waitForDescendantsGone(ctx context.Context, runID string) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, r.options.Timeout, true,
		func(ctx context.Context) (bool, error) {
			snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
			if err != nil {
				return false, err
			}
			return len(snapshot.AIJobs) == 0 && len(snapshot.JobSets) == 0 &&
				len(snapshot.Jobs) == 0 && len(snapshot.Pods) == 0, nil
		})
}

func (r *E2ERunner) benchmarkHelper() *BenchmarkRunner {
	return &BenchmarkRunner{cluster: r.cluster, options: BenchmarkOptions{
		Namespace: r.options.Namespace, Timeout: r.options.Timeout,
	}}
}

func verifyLifecycle(snapshot Snapshot, failed bool) error {
	if len(snapshot.AIJobs) != 1 || len(snapshot.JobSets) != 1 ||
		len(snapshot.Workloads) != 1 || len(snapshot.Jobs) != 1 || len(snapshot.Pods) == 0 {
		return fmt.Errorf(
			"incomplete chain: AIJobs=%d JobSets=%d Workloads=%d Jobs=%d Pods=%d",
			len(snapshot.AIJobs), len(snapshot.JobSets), len(snapshot.Workloads),
			len(snapshot.Jobs), len(snapshot.Pods),
		)
	}
	if failed {
		if !metaConditionTrue(snapshot.AIJobs[0].Status.Conditions, "Failed") ||
			!jobSetConditionTrue(snapshot.JobSets[0].Status.Conditions, "Failed") ||
			!jobConditionTrue(snapshot.Jobs[0].Status.Conditions, batchv1.JobFailed) ||
			!hasPodPhase(snapshot.Pods, corev1.PodFailed) {
			return errors.New("failure did not propagate through Pod, Job, JobSet, and AIJob")
		}
		return nil
	}
	if !metaConditionTrue(snapshot.AIJobs[0].Status.Conditions, "Completed") ||
		!jobSetConditionTrue(snapshot.JobSets[0].Status.Conditions, "Completed") ||
		!jobConditionTrue(snapshot.Jobs[0].Status.Conditions, batchv1.JobComplete) {
		return errors.New("success did not propagate through Job, JobSet, and AIJob")
	}
	return nil
}

func jobSetConditionTrue(conditions []metav1.Condition, condition string) bool {
	return metaConditionTrue(conditions, condition)
}

func jobConditionTrue(conditions []batchv1.JobCondition, condition batchv1.JobConditionType) bool {
	for _, item := range conditions {
		if item.Type == condition && item.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func hasPodPhase(pods []corev1.Pod, phase corev1.PodPhase) bool {
	for _, pod := range pods {
		if pod.Status.Phase == phase {
			return true
		}
	}
	return false
}
