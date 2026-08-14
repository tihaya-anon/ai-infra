package lab

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	ExerciseCapacity          = "capacity"
	ExerciseWorkerFailure     = "worker-failure"
	ExerciseControllerRestart = "controller-restart"
)

// ExerciseOptions controls one isolated diagnostic exercise.
type ExerciseOptions struct {
	Kind      string
	Namespace string
	OutputDir string
	Timeout   time.Duration
}

// ExerciseRunner injects one failure or restart and always collects evidence.
type ExerciseRunner struct {
	cluster *Cluster
	options ExerciseOptions
}

// NewExerciseRunner validates a failure exercise.
func NewExerciseRunner(cluster *Cluster, options ExerciseOptions) (*ExerciseRunner, error) {
	if options.Namespace == "" {
		options.Namespace = "default"
	}
	if options.OutputDir == "" {
		options.OutputDir = "out/evidence"
	}
	if options.Timeout <= 0 {
		return nil, errors.New("exercise timeout must be positive")
	}
	switch options.Kind {
	case ExerciseCapacity, ExerciseWorkerFailure, ExerciseControllerRestart:
	default:
		return nil, fmt.Errorf("unsupported exercise %q", options.Kind)
	}
	return &ExerciseRunner{cluster: cluster, options: options}, nil
}

// Run executes the selected exercise, writes evidence, and cleans only its run.
func (r *ExerciseRunner) Run(ctx context.Context) (runErr error) {
	runID := newRunID("exercise-" + r.options.Kind)
	benchmark := &BenchmarkRunner{cluster: r.cluster, options: BenchmarkOptions{
		Namespace: r.options.Namespace, Timeout: r.options.Timeout,
	}}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
		defer cancel()
		runErr = errors.Join(runErr, benchmark.cleanup(cleanupCtx, runID))
	}()

	expected, observed, scenarioErr := r.execute(ctx, benchmark, runID)
	collector, err := NewEvidenceCollector(r.cluster, EvidenceOptions{
		Namespace: r.options.Namespace, RunID: runID,
		Experiment: r.options.Kind, OutputDir: r.options.OutputDir,
		Expected: expected, Observed: observed,
	})
	if err != nil {
		return errors.Join(scenarioErr, err)
	}
	evidenceCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
	defer cancel()
	_, evidenceErr := collector.Collect(evidenceCtx)
	return errors.Join(scenarioErr, evidenceErr)
}

func (r *ExerciseRunner) execute(
	ctx context.Context,
	benchmark *BenchmarkRunner,
	runID string,
) ([]string, []string, error) {
	switch r.options.Kind {
	case ExerciseCapacity:
		definition := WorkloadDefinition{
			Name: "capacity-" + runID, Workers: 1, GPUPerWorker: 13,
			Args: []string{"--mode=complete", "--duration=1s"},
		}
		if err := benchmark.createAIJob(ctx, runID, r.options.Kind, definition); err != nil {
			return []string{"Capacity block diagnosed"}, nil, err
		}
		stage, err := r.waitCapacityStage(ctx, runID, definition.Name)
		observed := []string{}
		if stage != "" {
			observed = append(observed, "Capacity block diagnosed", stage)
		}
		return []string{"Capacity block diagnosed"}, observed, err
	case ExerciseWorkerFailure:
		definition := WorkloadDefinition{
			Name: "failure-" + runID, Workers: 2, GPUPerWorker: 1,
			Args: []string{"--mode=complete", "--duration=1s", "--fail-indexes=1"},
		}
		if err := benchmark.createAIJob(ctx, runID, r.options.Kind, definition); err != nil {
			return []string{"AIJob Failed"}, nil, err
		}
		_, err := r.cluster.WaitForAIJobCondition(ctx, types.NamespacedName{
			Namespace: r.options.Namespace, Name: definition.Name,
		}, "Failed", r.options.Timeout)
		observed := []string{}
		if err == nil {
			observed = append(observed, "AIJob Failed")
		}
		return []string{"AIJob Failed"}, observed, err
	case ExerciseControllerRestart:
		definition := WorkloadDefinition{
			Name: "restart-" + runID, Workers: 1, GPUPerWorker: 1,
			Args: []string{"--mode=wait"},
		}
		if err := benchmark.createAIJob(ctx, runID, r.options.Kind, definition); err != nil {
			return []string{"Controller replaced", "one owned JobSet"}, nil, err
		}
		if _, err := r.cluster.WaitForPodScheduled(
			ctx, r.options.Namespace, runID, definition.Name, r.options.Timeout,
		); err != nil {
			return []string{"Controller replaced", "one owned JobSet"}, nil, err
		}
		observed, err := r.replaceController(ctx, runID)
		return []string{"Controller replaced", "one owned JobSet"}, observed, err
	default:
		panic("validated exercise kind is not implemented")
	}
}

func (r *ExerciseRunner) waitCapacityStage(
	ctx context.Context,
	runID, workloadName string,
) (string, error) {
	var workloadSeenAt time.Time
	var stage string
	err := wait.PollUntilContextTimeout(ctx, time.Second, r.options.Timeout, true,
		func(ctx context.Context) (bool, error) {
			snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
			if err != nil {
				return false, err
			}
			for _, pod := range snapshot.Pods {
				if pod.Labels[topology.JobLabel] == workloadName &&
					UnschedulableCount([]corev1.Pod{pod}) == 1 {
					stage = "Pod Unschedulable at kube-scheduler"
					return true, nil
				}
			}
			if len(snapshot.Workloads) > 0 {
				if workloadSeenAt.IsZero() {
					workloadSeenAt = time.Now()
				}
				if time.Since(workloadSeenAt) >= 3*time.Second && len(snapshot.Pods) == 0 {
					stage = "Workload waiting at Kueue admission"
					return true, nil
				}
			}
			return false, nil
		})
	return stage, err
}

func (r *ExerciseRunner) replaceController(ctx context.Context, runID string) ([]string, error) {
	pods, err := r.cluster.Core.CoreV1().Pods("ai-infra-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app=aijob-controller",
	})
	if err != nil || len(pods.Items) != 1 {
		return nil, fmt.Errorf("expected one Controller Pod before restart: count=%d err=%w",
			len(pods.Items), err)
	}
	oldUID := pods.Items[0].UID
	if err := r.cluster.Core.CoreV1().Pods("ai-infra-system").Delete(
		ctx, pods.Items[0].Name, metav1.DeleteOptions{},
	); err != nil {
		return nil, err
	}
	observed := []string{}
	err = wait.PollUntilContextTimeout(ctx, time.Second, r.options.Timeout, true,
		func(ctx context.Context) (bool, error) {
			current, err := r.cluster.Core.CoreV1().Pods("ai-infra-system").List(
				ctx, metav1.ListOptions{LabelSelector: "app=aijob-controller"},
			)
			if err != nil {
				return false, err
			}
			for _, pod := range current.Items {
				if pod.UID != oldUID && conditionTrue(pod.Status.Conditions, corev1.PodReady) {
					return true, nil
				}
			}
			return false, nil
		})
	if err != nil {
		return observed, err
	}
	observed = append(observed, "Controller replaced")
	snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
	if err != nil {
		return observed, err
	}
	if len(snapshot.JobSets) != 1 {
		return observed, fmt.Errorf("got %d run-scoped JobSets after restart, want one",
			len(snapshot.JobSets))
	}
	return append(observed, "one owned JobSet"), nil
}
