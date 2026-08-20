package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// SchedulerName selects the scheduler profile containing the GPU topology plugin.
const SchedulerName = "ai-scheduler"

const queueRetryDelay = 30 * time.Second

var _ reconcile.Reconciler = &AIJobReconciler{}

// +kubebuilder:rbac:groups=infra.example.io,resources=aijobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=infra.example.io,resources=aijobs/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=jobset.x-k8s.io,resources=jobsets,verbs=create;get;list;patch;update;watch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=localqueues,verbs=get;list;watch

// AIJobReconciler adapts the AIJob API to the standard JobSet API.
type AIJobReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Metrics *Metrics
}

// Reconcile converges one AIJob-owned JobSet and derives the public status.
func (r *AIJobReconciler) Reconcile(
	ctx context.Context,
	request ctrl.Request,
) (ctrl.Result, error) {
	started := time.Now()
	done := func(output ctrl.Result, result reconcileResult, err error) (ctrl.Result, error) {
		r.Metrics.observe(started, result)
		return output, err
	}

	job := &aiv1alpha1.AIJob{}
	if err := r.Get(ctx, request.NamespacedName, job); err != nil {
		if apierrors.IsNotFound(err) {
			return done(ctrl.Result{}, reconcileNotFound, nil)
		}
		r.Metrics.recordError(errorOperationGet)
		return done(ctrl.Result{}, reconcileError, client.IgnoreNotFound(err))
	}

	queueName := selectedQueueName(job.Spec.QueueName)
	queue := &kueuev1beta1.LocalQueue{}
	queueKey := types.NamespacedName{Name: queueName, Namespace: job.Namespace}
	if err := r.Get(ctx, queueKey, queue); err != nil {
		if apierrors.IsNotFound(err) {
			conditions := append([]metav1.Condition(nil), job.Status.Conditions...)
			status := statusWithQueue(
				job.Generation, aiv1alpha1.AIJobStatus{Conditions: conditions},
				job.Status.Conditions, queueName, false,
			)
			changed, statusErr := r.reconcileStatus(ctx, job, status)
			if statusErr != nil {
				r.Metrics.recordError(errorOperationStatus)
				return done(ctrl.Result{}, reconcileError, statusErr)
			}
			r.Metrics.recordStatusChange(changed)
			return done(
				ctrl.Result{RequeueAfter: queueRetryDelay}, reconcileWaiting, nil,
			)
		}
		r.Metrics.recordError(errorOperationQueue)
		return done(ctrl.Result{}, reconcileError, err)
	}

	jobSet, operation, err := r.reconcileJobSet(ctx, job)
	if err != nil {
		r.Metrics.recordError(errorOperationJobSet)
		return done(ctrl.Result{}, reconcileError, err)
	}
	r.Metrics.recordJobSetChange(operation)
	status := statusWithQueue(
		job.Generation, statusFromJobSet(job.Generation, jobSet),
		job.Status.Conditions, queueName, true,
	)
	changed, err := r.reconcileStatus(ctx, job, status)
	if err != nil {
		r.Metrics.recordError(errorOperationStatus)
		return done(ctrl.Result{}, reconcileError, err)
	}
	r.Metrics.recordStatusChange(changed)
	return done(ctrl.Result{}, reconcileSuccess, nil)
}

func (r *AIJobReconciler) reconcileJobSet(
	ctx context.Context,
	job *aiv1alpha1.AIJob,
) (*jobsetv1alpha2.JobSet, jobSetChangeOperation, error) {
	desired := desiredJobSet(job)
	jobSet := &jobsetv1alpha2.JobSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	operation, err := controllerutil.CreateOrUpdate(ctx, r.Client, jobSet, func() error {
		if err := ctrl.SetControllerReference(job, jobSet, r.Scheme); err != nil {
			return err
		}
		reconcileOwnedFields(jobSet, desired)
		return nil
	})
	return jobSet, operationLabel(operation), err
}

func (r *AIJobReconciler) reconcileStatus(
	ctx context.Context,
	job *aiv1alpha1.AIJob,
	status aiv1alpha1.AIJobStatus,
) (bool, error) {
	if reflect.DeepEqual(job.Status, status) {
		return false, nil
	}
	job.Status = status
	return true, r.Status().Update(ctx, job)
}

func operationLabel(operation controllerutil.OperationResult) jobSetChangeOperation {
	switch operation {
	case controllerutil.OperationResultCreated:
		return jobSetOperationCreate
	case controllerutil.OperationResultUpdated, controllerutil.OperationResultUpdatedStatus,
		controllerutil.OperationResultUpdatedStatusOnly:
		return jobSetOperationUpdate
	default:
		return jobSetOperationUnchanged
	}
}

// SetupWithManager maps owned JobSet changes back to their AIJob.
func (r *AIJobReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&aiv1alpha1.AIJob{}).
		Owns(&jobsetv1alpha2.JobSet{}).
		Complete(r)
}

func desiredJobSet(job *aiv1alpha1.AIJob) *jobsetv1alpha2.JobSet {
	labels := topology.LabLabels(job.Labels)
	labels[topology.JobLabel] = job.Name
	labels[topology.QueueLabel] = selectedQueueName(job.Spec.QueueName)
	jobSet := &jobsetv1alpha2.JobSet{
		ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace, Labels: labels},
		Spec: jobsetv1alpha2.JobSetSpec{
			Network: &jobsetv1alpha2.Network{EnableDNSHostnames: boolPtr(true)},
			ReplicatedJobs: []jobsetv1alpha2.ReplicatedJob{{
				Name:     "workers",
				Replicas: 1,
				Template: batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: topology.LabLabels(job.Labels)},
					Spec:       workerJobSpec(job),
				},
			}},
		},
	}
	if job.Spec.JobSetOverrides != nil {
		jobSet.Spec.FailurePolicy = job.Spec.JobSetOverrides.FailurePolicy
		jobSet.Spec.SuccessPolicy = job.Spec.JobSetOverrides.SuccessPolicy
		jobSet.Spec.Suspend = &job.Spec.JobSetOverrides.Suspend
	}
	return jobSet
}

// reconcileOwnedFields initializes immutable spec fields only when creating the
// JobSet. Webhooks and Kueue may default or inject fields into that spec later.
func reconcileOwnedFields(actual, desired *jobsetv1alpha2.JobSet) {
	if actual.Labels == nil {
		actual.Labels = make(map[string]string, 3)
	}
	actual.Labels[topology.JobLabel] = desired.Labels[topology.JobLabel]
	for _, key := range []string{topology.RunIDLabel, topology.ExperimentLabel, topology.QueueLabel} {
		if value := desired.Labels[key]; value != "" {
			actual.Labels[key] = value
		} else {
			delete(actual.Labels, key)
		}
	}

	if desired.Spec.Suspend != nil {
		actual.Spec.Suspend = desired.Spec.Suspend
	}
	if actual.CreationTimestamp.IsZero() {
		actual.Spec.ReplicatedJobs = desired.Spec.ReplicatedJobs
		actual.Spec.Network = desired.Spec.Network
		actual.Spec.FailurePolicy = desired.Spec.FailurePolicy
		actual.Spec.SuccessPolicy = desired.Spec.SuccessPolicy
	}
}

func workerJobSpec(job *aiv1alpha1.AIJob) batchv1.JobSpec {
	workers := job.Spec.Workers
	backoffLimit := int32(0)
	labels := topology.LabLabels(job.Labels)
	labels[topology.JobLabel] = job.Name
	return batchv1.JobSpec{
		Parallelism:    &workers,
		Completions:    &workers,
		CompletionMode: ptr(batchv1.IndexedCompletion),
		BackoffLimit:   &backoffLimit,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: schedulingAnnotations(job.Spec.Topology),
			},
			Spec: corev1.PodSpec{
				SchedulerName: SchedulerName,
				RestartPolicy: corev1.RestartPolicyNever,
				Containers:    []corev1.Container{workerContainer(job)},
			},
		},
	}
}

func workerContainer(job *aiv1alpha1.AIJob) corev1.Container {
	image := job.Spec.Image
	if image == "" {
		image = "ai-infra-lab:dev"
	}
	gpuResource := corev1.ResourceName(job.Spec.GPUResource)
	if gpuResource == "" {
		gpuResource = topology.GPUResource
	}
	gpus := *resource.NewQuantity(job.Spec.GPUPerWorker, resource.DecimalSI)
	resources := corev1.ResourceList{gpuResource: gpus}
	return corev1.Container{
		Name:            "worker",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/worker"},
		Args:            append([]string(nil), job.Spec.Args...),
		Env: []corev1.EnvVar{{
			Name: "JOB_COMPLETION_INDEX",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.labels['batch.kubernetes.io/job-completion-index']",
			}},
		}},
		Resources: corev1.ResourceRequirements{Requests: resources, Limits: resources},
	}
}

func schedulingAnnotations(topologyConfig aiv1alpha1.Topology) map[string]string {
	annotationMap := map[string]string{}

	if topologyConfig.Preference == topology.TopologyNVLink ||
		topologyConfig.Preference == topology.TopologyPCIe {
		annotationMap[topology.PreferenceAnnotation] = topologyConfig.Preference
	}

	switch topologyConfig.Required {
	case "same-rack":
		annotationMap[topology.RequiredTopologyAnnotation] = topology.RackLabel
	case topology.TopologyNVLink, topology.TopologyPCIe:
		annotationMap[topology.RequiredTopologyClassAnnotation] = topologyConfig.Required
	}

	if len(annotationMap) == 0 {
		return nil
	}
	return annotationMap
}

// statusFromJobSet exposes the standard JobSet conditions through the AIJob API.
func statusFromJobSet(generation int64, jobSet *jobsetv1alpha2.JobSet) aiv1alpha1.AIJobStatus {
	status := aiv1alpha1.AIJobStatus{ObservedGeneration: generation}
	if len(jobSet.Status.Conditions) == 0 {
		return status
	}
	status.Conditions = make([]metav1.Condition, len(jobSet.Status.Conditions))
	copy(status.Conditions, jobSet.Status.Conditions)
	for index := range status.Conditions {
		status.Conditions[index].ObservedGeneration = generation
	}
	return status
}

func statusWithQueue(
	generation int64,
	status aiv1alpha1.AIJobStatus,
	previous []metav1.Condition,
	queueName string,
	ready bool,
) aiv1alpha1.AIJobStatus {
	status.ObservedGeneration = generation
	existing := apiMeta.FindStatusCondition(previous, aiv1alpha1.ConditionQueueReady)
	current := apiMeta.FindStatusCondition(status.Conditions, aiv1alpha1.ConditionQueueReady)
	if existing != nil && current == nil {
		status.Conditions = append(status.Conditions, *existing)
	}
	condition := metav1.Condition{
		Type:               aiv1alpha1.ConditionQueueReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		Reason:             "QueueNotFound",
		Message:            fmt.Sprintf("LocalQueue %q does not exist", queueName),
	}
	if ready {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "QueueFound"
		condition.Message = fmt.Sprintf("LocalQueue %q is available", queueName)
	}
	apiMeta.SetStatusCondition(&status.Conditions, condition)
	return status
}

func selectedQueueName(queueName string) string {
	if queueName == "" {
		return aiv1alpha1.DefaultQueueName
	}
	return queueName
}

func boolPtr(value bool) *bool { return &value }

func ptr[T any](value T) *T { return &value }
