package lab

import (
	"context"
	"fmt"
	"strings"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

// Cluster provides typed clients and bounded waits for lab orchestration.
type Cluster struct {
	Client client.Client
	Core   kubernetes.Interface
	Config *rest.Config
}

// Snapshot is a run-scoped view of the relevant Kubernetes objects.
type Snapshot struct {
	AIJobs      []aiv1alpha1.AIJob
	JobSets     []jobsetv1alpha2.JobSet
	Workloads   []Workload
	Jobs        []batchv1.Job
	Pods        []corev1.Pod
	Nodes       []corev1.Node
	Deployments []appsv1.Deployment
	Events      []corev1.Event
}

// NewCluster loads the current kubeconfig and registers every observed API type.
func NewCluster() (*Cluster, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, batchv1.AddToScheme,
		aiv1alpha1.AddToScheme, jobsetv1alpha2.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	addKueueToScheme(scheme)
	typedClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create typed client: %w", err)
	}
	coreClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes clientset: %w", err)
	}
	return &Cluster{Client: typedClient, Core: coreClient, Config: config}, nil
}

// ValidateKindContext prevents lab mutations against an unintended cluster.
func ValidateKindContext(clusterName string) error {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := rules.Load()
	if err != nil {
		return fmt.Errorf("load kubeconfig context: %w", err)
	}
	want := "kind-" + clusterName
	return validateContext(config.CurrentContext, want)
}

func validateContext(current, want string) error {
	if current != want {
		return fmt.Errorf("refusing mutation: current context %q, want %q", current, want)
	}
	return nil
}

// Discover returns typed, run-scoped namespaced resources and shared fixtures.
func (c *Cluster) Discover(ctx context.Context, namespace, runID string) (Snapshot, error) {
	selector := client.MatchingLabels{topology.RunIDLabel: runID}
	inNamespace := client.InNamespace(namespace)
	result := Snapshot{}
	aijobs := &aiv1alpha1.AIJobList{}
	jobSets := &jobsetv1alpha2.JobSetList{}
	workloads := &WorkloadList{}
	jobs := &batchv1.JobList{}
	pods := &corev1.PodList{}
	for _, object := range []client.ObjectList{aijobs, jobSets, jobs, pods} {
		if err := c.Client.List(ctx, object, inNamespace, selector); err != nil {
			return result, fmt.Errorf("list %T: %w", object, err)
		}
	}
	if err := c.Client.List(ctx, workloads, inNamespace); err != nil {
		return result, fmt.Errorf("list %T: %w", workloads, err)
	}
	result.AIJobs = aijobs.Items
	result.JobSets = jobSets.Items
	result.Workloads = relatedWorkloads(workloads.Items, result.AIJobs, result.JobSets)
	result.Jobs = jobs.Items
	result.Pods = pods.Items

	nodes := &corev1.NodeList{}
	if err := c.Client.List(ctx, nodes); err != nil {
		return result, fmt.Errorf("list Nodes: %w", err)
	}
	result.Nodes = nodes.Items
	deployments := &appsv1.DeploymentList{}
	if err := c.Client.List(ctx, deployments, client.InNamespace("ai-infra-system")); err != nil {
		return result, fmt.Errorf("list Deployments: %w", err)
	}
	result.Deployments = deployments.Items
	events := &corev1.EventList{}
	if err := c.Client.List(ctx, events, inNamespace); err != nil {
		return result, fmt.Errorf("list Events: %w", err)
	}
	result.Events = filterEvents(events.Items, result)
	return result, nil
}

func relatedWorkloads(
	workloads []Workload,
	aijobs []aiv1alpha1.AIJob,
	jobSets []jobsetv1alpha2.JobSet,
) []Workload {
	names := make(map[string]struct{}, len(aijobs)+len(jobSets))
	for _, job := range aijobs {
		names[job.Name] = struct{}{}
	}
	for _, jobSet := range jobSets {
		names[jobSet.Name] = struct{}{}
	}
	result := make([]Workload, 0, len(workloads))
	for _, workload := range workloads {
		if _, ok := names[workload.Labels[topology.JobLabel]]; ok {
			result = append(result, workload)
			continue
		}
		for name := range names {
			if ownerNamed(workload.OwnerReferences, name) {
				result = append(result, workload)
				break
			}
		}
	}
	return result
}

// WaitForPodScheduled waits for a run-scoped workload Pod to bind to a Node.
func (c *Cluster) WaitForPodScheduled(
	ctx context.Context,
	namespace, runID, workload string,
	timeout time.Duration,
) (*corev1.Pod, error) {
	var observed *corev1.Pod
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods := &corev1.PodList{}
			labels := client.MatchingLabels{
				topology.RunIDLabel: runID, topology.JobLabel: workload,
			}
			if err := c.Client.List(ctx, pods, client.InNamespace(namespace), labels); err != nil {
				return false, err
			}
			for index := range pods.Items {
				pod := &pods.Items[index]
				if pod.Spec.NodeName != "" && conditionTrue(pod.Status.Conditions, corev1.PodScheduled) {
					observed = pod.DeepCopy()
					return true, nil
				}
			}
			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("wait for workload %s PodScheduled: %w", workload, err)
	}
	return observed, nil
}

// WaitForPodUnschedulable waits until kube-scheduler exposes a fit failure.
func (c *Cluster) WaitForPodUnschedulable(
	ctx context.Context,
	namespace, runID, workload string,
	timeout time.Duration,
) (*corev1.Pod, error) {
	var observed *corev1.Pod
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods := &corev1.PodList{}
			labels := client.MatchingLabels{
				topology.RunIDLabel: runID, topology.JobLabel: workload,
			}
			if err := c.Client.List(ctx, pods, client.InNamespace(namespace), labels); err != nil {
				return false, err
			}
			for index := range pods.Items {
				pod := &pods.Items[index]
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodScheduled &&
						condition.Status == corev1.ConditionFalse &&
						condition.Reason == corev1.PodReasonUnschedulable {
						observed = pod.DeepCopy()
						return true, nil
					}
				}
			}
			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("wait for workload %s Unschedulable: %w", workload, err)
	}
	return observed, nil
}

// WaitForAIJobCondition waits for a projected terminal condition.
func (c *Cluster) WaitForAIJobCondition(
	ctx context.Context,
	key types.NamespacedName,
	condition string,
	timeout time.Duration,
) (*aiv1alpha1.AIJob, error) {
	var observed *aiv1alpha1.AIJob
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, timeout, true,
		func(ctx context.Context) (bool, error) {
			job := &aiv1alpha1.AIJob{}
			if err := c.Client.Get(ctx, key, job); err != nil {
				return false, client.IgnoreNotFound(err)
			}
			if metaConditionTrue(job.Status.Conditions, condition) {
				observed = job
				return true, nil
			}
			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("wait for AIJob %s condition %s: %w", key, condition, err)
	}
	return observed, nil
}

// WaitForDeployment waits for a specific generation to be fully available.
func (c *Cluster) WaitForDeployment(
	ctx context.Context,
	key types.NamespacedName,
	generation int64,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deployment := &appsv1.Deployment{}
			if err := c.Client.Get(ctx, key, deployment); err != nil {
				return false, err
			}
			want := int32(1)
			if deployment.Spec.Replicas != nil {
				want = *deployment.Spec.Replicas
			}
			return deployment.Status.ObservedGeneration >= generation &&
				deployment.Status.UpdatedReplicas == want &&
				deployment.Status.AvailableReplicas == want, nil
		})
}

// ComponentLogs retrieves current Controller and Scheduler Pod logs.
func (c *Cluster) ComponentLogs(ctx context.Context) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for _, component := range []string{"aijob-controller", "ai-scheduler"} {
		pods, err := c.Core.CoreV1().Pods("ai-infra-system").List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + component,
		})
		if err != nil {
			return result, err
		}
		for _, pod := range pods.Items {
			data, err := c.Core.CoreV1().Pods(pod.Namespace).GetLogs(
				pod.Name, &corev1.PodLogOptions{},
			).DoRaw(ctx)
			if err != nil {
				return result, err
			}
			result[component+"-"+pod.Name] = data
		}
	}
	return result, nil
}

// MetricsSnapshot retrieves a Service metrics path through the Kubernetes API proxy.
func (c *Cluster) MetricsSnapshot(
	ctx context.Context,
	service string,
	port int,
) ([]byte, error) {
	name := fmt.Sprintf("https:%s:%d", service, port)
	if service == "aijob-controller-metrics" {
		name = fmt.Sprintf("http:%s:%d", service, port)
	}
	return c.Core.CoreV1().RESTClient().Get().
		Namespace("ai-infra-system").
		Resource("services").Name(name).
		SubResource("proxy").Suffix("metrics").
		DoRaw(ctx)
}

func conditionTrue(conditions []corev1.PodCondition, condition corev1.PodConditionType) bool {
	for _, item := range conditions {
		if item.Type == condition && item.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func metaConditionTrue(conditions []metav1.Condition, condition string) bool {
	for _, item := range conditions {
		if strings.EqualFold(item.Type, condition) && item.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func filterEvents(events []corev1.Event, snapshot Snapshot) []corev1.Event {
	names := make(map[string]struct{})
	for _, job := range snapshot.AIJobs {
		names[job.Name] = struct{}{}
	}
	for _, jobSet := range snapshot.JobSets {
		names[jobSet.Name] = struct{}{}
	}
	for _, workload := range snapshot.Workloads {
		names[workload.Name] = struct{}{}
	}
	for _, job := range snapshot.Jobs {
		names[job.Name] = struct{}{}
	}
	for _, pod := range snapshot.Pods {
		names[pod.Name] = struct{}{}
	}
	result := make([]corev1.Event, 0, len(events))
	for _, event := range events {
		if _, relevant := names[event.InvolvedObject.Name]; relevant {
			result = append(result, event)
		}
	}
	return result
}
