package lab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const ToolVersion = "dev"

// Profile identifies a Scheduler ConfigMap manifest and expected probe behavior.
type Profile struct {
	Name           string
	ConfigPath     string
	ProbeShouldFit bool
}

// BenchmarkOptions controls isolated sequential experiment runs.
type BenchmarkOptions struct {
	Namespace   string
	OutputDir   string
	Timeout     time.Duration
	Repetitions int
	Profiles    []Profile
}

// BenchmarkRunner owns Scheduler switching, workload lifecycle, results, and cleanup.
type BenchmarkRunner struct {
	cluster *Cluster
	options BenchmarkOptions
}

// NewBenchmarkRunner validates options and constructs an orchestrator.
func NewBenchmarkRunner(cluster *Cluster, options BenchmarkOptions) (*BenchmarkRunner, error) {
	if options.Namespace == "" {
		options.Namespace = "default"
	}
	if options.OutputDir == "" {
		options.OutputDir = "out/benchmark"
	}
	if options.Timeout <= 0 {
		return nil, errors.New("benchmark timeout must be positive")
	}
	if options.Repetitions < 1 {
		return nil, errors.New("benchmark repetitions must be at least one")
	}
	if len(options.Profiles) == 0 {
		return nil, errors.New("at least one Scheduler profile is required")
	}
	return &BenchmarkRunner{cluster: cluster, options: options}, nil
}

// Run executes every profile and repetition while restoring the initial Scheduler config.
func (r *BenchmarkRunner) Run(ctx context.Context) (runErr error) {
	capacity, err := r.validateFixtures(ctx)
	if err != nil {
		return err
	}
	restore, err := r.captureScheduler(ctx)
	if err != nil {
		return err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
		defer cancel()
		runErr = errors.Join(runErr, restore(restoreCtx))
	}()

	var runErrors []error
	for _, profile := range r.options.Profiles {
		for repetition := 1; repetition <= r.options.Repetitions; repetition++ {
			if err := r.runOne(ctx, profile, repetition, capacity); err != nil {
				runErrors = append(runErrors, err)
			}
		}
	}
	return errors.Join(runErrors...)
}

func (r *BenchmarkRunner) runOne(
	ctx context.Context,
	profile Profile,
	repetition int,
	capacity ClusterCapacity,
) (runErr error) {
	runID := newRunID("bench-" + profile.Name)
	result := BenchmarkResult{
		SchemaVersion: ResultSchemaVersion, RunID: runID, Timestamp: time.Now().UTC(),
		Profile: profile.Name, Cluster: capacity, Outcomes: make(map[string]string),
		Environment: Environment{ToolVersion: ToolVersion, Components: map[string]string{
			"schedulerProfile": filepath.Base(profile.ConfigPath),
		}},
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), r.options.Timeout)
		defer cancel()
		if runErr != nil {
			result.Missing = append(result.Missing, runErr.Error())
		}
		if err := r.collectResult(cleanupCtx, &result); err != nil {
			result.Missing = append(result.Missing, "final snapshot: "+err.Error())
		}
		result.Complete = runErr == nil && len(result.Missing) == 0
		if err := writeResult(r.options.OutputDir, profile.Name, repetition, result); err != nil {
			runErr = errors.Join(runErr, err)
		}
		if err := r.cleanup(cleanupCtx, runID); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()

	if err := r.switchProfile(ctx, profile.ConfigPath); err != nil {
		runErr = fmt.Errorf("switch profile %s: %w", profile.Name, err)
		return runErr
	}
	start := time.Now().UTC()
	for index := 1; index <= 3; index++ {
		name := fmt.Sprintf("holder-%d-%s", index, runID)
		definition := WorkloadDefinition{
			Name: name, Workers: 1, GPUPerWorker: 2, Args: []string{"--mode=wait"},
		}
		result.Workloads = append(result.Workloads, definition)
		if err := r.createAIJob(ctx, runID, "benchmark", definition); err != nil {
			runErr = err
			return runErr
		}
		if _, err := r.cluster.WaitForPodScheduled(
			ctx, r.options.Namespace, runID, name, r.options.Timeout,
		); err != nil {
			runErr = err
			return runErr
		}
	}

	snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
	if err != nil {
		runErr = err
		return runErr
	}
	result.Measurements.Fragmentation = CalculateFragmentation(
		FreeGPUs(snapshot.Nodes, snapshot.Pods), 4, "all-simulated-gpu-nodes",
	)
	probe := WorkloadDefinition{
		Name: "probe-" + runID, Workers: 1, GPUPerWorker: 4,
		Args: []string{"--mode=complete", "--duration=1s"},
	}
	result.Workloads = append(result.Workloads, probe)
	if err := r.createAIJob(ctx, runID, "benchmark", probe); err != nil {
		runErr = err
		return runErr
	}
	if profile.ProbeShouldFit {
		if _, err := r.cluster.WaitForPodScheduled(
			ctx, r.options.Namespace, runID, probe.Name, r.options.Timeout,
		); err != nil {
			runErr = err
			return runErr
		}
		if _, err := r.cluster.WaitForAIJobCondition(ctx, types.NamespacedName{
			Namespace: r.options.Namespace, Name: probe.Name,
		}, "Completed", r.options.Timeout); err != nil {
			runErr = err
			return runErr
		}
		result.Outcomes[probe.Name] = "completed"
	} else {
		if _, err := r.cluster.WaitForPodUnschedulable(
			ctx, r.options.Namespace, runID, probe.Name, r.options.Timeout,
		); err != nil {
			runErr = err
			return runErr
		}
		result.Outcomes[probe.Name] = "unschedulable"
	}
	makespan := time.Since(start).Seconds()
	result.Measurements.MakespanSeconds = &makespan
	return nil
}

func (r *BenchmarkRunner) createAIJob(
	ctx context.Context,
	runID, experiment string,
	definition WorkloadDefinition,
) error {
	job := &aiv1alpha1.AIJob{
		TypeMeta: metav1.TypeMeta{APIVersion: aiv1alpha1.GroupVersion.String(), Kind: "AIJob"},
		ObjectMeta: metav1.ObjectMeta{
			Name: definition.Name, Namespace: r.options.Namespace,
			Labels: map[string]string{
				topology.RunIDLabel: runID, topology.ExperimentLabel: experiment,
			},
		},
		Spec: aiv1alpha1.AIJobSpec{
			Workers: definition.Workers, GPUPerWorker: definition.GPUPerWorker,
			GPUResource: string(topology.GPUResource),
			QueueName:   aiv1alpha1.DefaultQueueName,
			Topology:    aiv1alpha1.Topology{Preference: "any"},
			Image:       "ai-infra-lab:dev", Args: append([]string(nil), definition.Args...),
		},
	}
	if err := r.cluster.Client.Create(ctx, job); err != nil {
		return fmt.Errorf("create AIJob %s: %w", definition.Name, err)
	}
	return nil
}

func (r *BenchmarkRunner) validateFixtures(ctx context.Context) (ClusterCapacity, error) {
	nodes := &corev1.NodeList{}
	if err := r.cluster.Client.List(ctx, nodes, client.MatchingLabels{
		"infra.example.io/gpu-node": "true",
	}); err != nil {
		return ClusterCapacity{}, err
	}
	if len(nodes.Items) != 3 {
		return ClusterCapacity{}, fmt.Errorf("benchmark requires 3 GPU Nodes, found %d", len(nodes.Items))
	}
	for _, node := range nodes.Items {
		if value := quantityValue(node.Status.Allocatable, topology.GPUResource); value != 4 {
			return ClusterCapacity{}, fmt.Errorf("Node %s has %d simulated GPUs, want 4", node.Name, value)
		}
	}
	queue := &ClusterQueue{}
	if err := r.cluster.Client.Get(ctx, types.NamespacedName{Name: "training"}, queue); err != nil {
		return ClusterCapacity{}, fmt.Errorf("get ClusterQueue training: %w", err)
	}
	if quota := simulatedGPUQuota(queue); quota < 10 {
		return ClusterCapacity{}, fmt.Errorf(
			"ClusterQueue training has %d simulated GPUs, benchmark requires at least 10", quota,
		)
	}
	return ClusterCapacity{EligibleNodes: 3, GPUPerNode: 4, TotalGPUs: 12}, nil
}

func simulatedGPUQuota(queue *ClusterQueue) int64 {
	var total int64
	for _, group := range queue.Spec.ResourceGroups {
		for _, flavor := range group.Flavors {
			for _, quota := range flavor.Resources {
				if quota.Name == topology.GPUResource {
					total += quota.NominalQuota.Value()
				}
			}
		}
	}
	return total
}

func (r *BenchmarkRunner) switchProfile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	desired := &corev1.ConfigMap{}
	if err := yaml.Unmarshal(data, desired); err != nil {
		return fmt.Errorf("parse Scheduler profile: %w", err)
	}
	key := types.NamespacedName{Name: "ai-scheduler-config", Namespace: "ai-infra-system"}
	current := &corev1.ConfigMap{}
	if err := r.cluster.Client.Get(ctx, key, current); err != nil {
		return err
	}
	current.Data = desired.Data
	if err := r.cluster.Client.Update(ctx, current); err != nil {
		return err
	}
	deploymentKey := types.NamespacedName{Name: "ai-scheduler", Namespace: "ai-infra-system"}
	deployment := &appsv1.Deployment{}
	if err := r.cluster.Client.Get(ctx, deploymentKey, deployment); err != nil {
		return err
	}
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["infra.example.io/config-restarted-at"] =
		time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.cluster.Client.Update(ctx, deployment); err != nil {
		return err
	}
	return r.cluster.WaitForDeployment(ctx, deploymentKey, deployment.Generation, r.options.Timeout)
}

func (r *BenchmarkRunner) captureScheduler(
	ctx context.Context,
) (func(context.Context) error, error) {
	configKey := types.NamespacedName{Name: "ai-scheduler-config", Namespace: "ai-infra-system"}
	deploymentKey := types.NamespacedName{Name: "ai-scheduler", Namespace: "ai-infra-system"}
	config := &corev1.ConfigMap{}
	deployment := &appsv1.Deployment{}
	if err := r.cluster.Client.Get(ctx, configKey, config); err != nil {
		return nil, err
	}
	if err := r.cluster.Client.Get(ctx, deploymentKey, deployment); err != nil {
		return nil, err
	}
	originalData := cloneMap(config.Data)
	originalAnnotations := cloneMap(deployment.Spec.Template.Annotations)
	return func(ctx context.Context) error {
		currentConfig := &corev1.ConfigMap{}
		if err := r.cluster.Client.Get(ctx, configKey, currentConfig); err != nil {
			return err
		}
		currentConfig.Data = cloneMap(originalData)
		if err := r.cluster.Client.Update(ctx, currentConfig); err != nil {
			return err
		}
		currentDeployment := &appsv1.Deployment{}
		if err := r.cluster.Client.Get(ctx, deploymentKey, currentDeployment); err != nil {
			return err
		}
		currentDeployment.Spec.Template.Annotations = cloneMap(originalAnnotations)
		if err := r.cluster.Client.Update(ctx, currentDeployment); err != nil {
			return err
		}
		return r.cluster.WaitForDeployment(
			ctx, deploymentKey, currentDeployment.Generation, r.options.Timeout,
		)
	}, nil
}

func (r *BenchmarkRunner) cleanup(ctx context.Context, runID string) error {
	jobs := &aiv1alpha1.AIJobList{}
	if err := r.cluster.Client.List(ctx, jobs, client.InNamespace(r.options.Namespace),
		client.MatchingLabels{topology.RunIDLabel: runID}); err != nil {
		return err
	}
	var cleanupErrors []error
	for index := range jobs.Items {
		err := r.cluster.Client.Delete(ctx, &jobs.Items[index])
		if err != nil && !apierrors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if err := errors.Join(cleanupErrors...); err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, r.options.Timeout, true,
		func(ctx context.Context) (bool, error) {
			snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, runID)
			if err != nil {
				return false, err
			}
			return len(snapshot.AIJobs) == 0 && len(snapshot.JobSets) == 0 &&
				len(snapshot.Workloads) == 0 && len(snapshot.Jobs) == 0 &&
				len(snapshot.Pods) == 0, nil
		})
}

func (r *BenchmarkRunner) collectResult(ctx context.Context, result *BenchmarkResult) error {
	snapshot, err := r.cluster.Discover(ctx, r.options.Namespace, result.RunID)
	if err != nil {
		return err
	}
	result.Measurements.UnschedulableCount = UnschedulableCount(snapshot.Pods)
	result.Placements = make([]PodPlacement, 0, len(snapshot.Pods))
	for _, pod := range snapshot.Pods {
		result.Placements = append(result.Placements, PodPlacement{
			Pod: pod.Name, Workload: pod.Labels[topology.JobLabel],
			CompletionIndex: pod.Labels["batch.kubernetes.io/job-completion-index"],
			Node:            pod.Spec.NodeName, Phase: string(pod.Status.Phase),
		})
	}
	for _, definition := range result.Workloads {
		if _, exists := result.Outcomes[definition.Name]; !exists {
			result.Outcomes[definition.Name] = "running"
		}
	}
	sort.Slice(result.Placements, func(i, j int) bool {
		return result.Placements[i].Pod < result.Placements[j].Pod
	})
	version, err := r.cluster.Core.Discovery().ServerVersion()
	if err == nil {
		result.Environment.KubernetesVersion = version.GitVersion
	}
	for _, deployment := range snapshot.Deployments {
		if len(deployment.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		result.Environment.Components[deployment.Name] =
			deployment.Spec.Template.Spec.Containers[0].Image
	}
	result.Lifecycle = lifecycleFromSnapshot(result.Workloads, snapshot)
	return nil
}

func lifecycleFromSnapshot(definitions []WorkloadDefinition, snapshot Snapshot) []Lifecycle {
	result := make([]Lifecycle, 0, len(definitions))
	for _, definition := range definitions {
		item := Lifecycle{Name: definition.Name}
		for _, job := range snapshot.AIJobs {
			if job.Name == definition.Name {
				item.SubmittedAt = job.CreationTimestamp.Time
				for _, condition := range job.Status.Conditions {
					if (condition.Type == "Completed" || condition.Type == "Failed") &&
						condition.Status == metav1.ConditionTrue {
						value := condition.LastTransitionTime.Time
						item.TerminalAt = &value
					}
				}
			}
		}
		for _, workload := range snapshot.Workloads {
			if workload.Labels[topology.JobLabel] != definition.Name &&
				!ownerNamed(workload.OwnerReferences, definition.Name) {
				continue
			}
			created := workload.CreationTimestamp.Time
			item.WorkloadCreatedAt = &created
			for _, condition := range workload.Status.Conditions {
				if condition.Type == "Admitted" && condition.Status == metav1.ConditionTrue {
					value := condition.LastTransitionTime.Time
					item.AdmittedAt = &value
				}
			}
		}
		for _, pod := range snapshot.Pods {
			if pod.Labels[topology.JobLabel] != definition.Name {
				continue
			}
			created := pod.CreationTimestamp.Time
			if item.PodCreatedAt == nil || created.Before(*item.PodCreatedAt) {
				item.PodCreatedAt = &created
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionTrue {
					value := condition.LastTransitionTime.Time
					if item.ScheduledAt == nil || value.Before(*item.ScheduledAt) {
						item.ScheduledAt = &value
					}
				}
			}
		}
		if item.AdmittedAt != nil && item.WorkloadCreatedAt != nil {
			seconds := item.AdmittedAt.Sub(*item.WorkloadCreatedAt).Seconds()
			item.AdmissionWaitSeconds = &seconds
		}
		if item.ScheduledAt != nil && item.PodCreatedAt != nil {
			seconds := item.ScheduledAt.Sub(*item.PodCreatedAt).Seconds()
			item.SchedulingLatencySeconds = &seconds
		}
		result = append(result, item)
	}
	return result
}

func writeResult(outputDir, profile string, repetition int, result BenchmarkResult) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%02d-%s.json", profile, repetition, result.RunID)
	path := filepath.Join(outputDir, name)
	temporary := path + ".tmp"
	file, err := os.Create(temporary)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(result)
	closeErr := file.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func newRunID(prefix string) string {
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().Unix(), hex.EncodeToString(random))
}

func cloneMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func ownerNamed(owners []metav1.OwnerReference, name string) bool {
	for _, owner := range owners {
		if owner.Name == name {
			return true
		}
	}
	return false
}
