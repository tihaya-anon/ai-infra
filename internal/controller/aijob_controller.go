package controller

import (
	"context"
	"reflect"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

// SchedulerName selects the scheduler profile containing the GPU topology plugin.
const SchedulerName = "ai-scheduler"

type reconcileResult string

const (
	reconcileSuccess  reconcileResult = "success"
	reconcileError    reconcileResult = "error"
	reconcileNotFound reconcileResult = "not_found"
)

var _ reconcile.Reconciler = &AIJobReconciler{}

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
	done := func(result reconcileResult, err error) (ctrl.Result, error) {
		r.Metrics.observe(started, string(result))
		return ctrl.Result{}, err
	}

	job := &aiv1alpha1.AIJob{}
	if err := r.Get(ctx, request.NamespacedName, job); err != nil {
		if apierrors.IsNotFound(err) {
			return done(reconcileNotFound, nil)
		}
		r.Metrics.recordError(errorOperationGet)
		return done(reconcileError, client.IgnoreNotFound(err))
	}

	jobSet, operation, err := r.reconcileJobSet(ctx, job)
	if err != nil {
		r.Metrics.recordError(errorOperationJobSet)
		return done(reconcileError, err)
	}
	r.Metrics.recordJobSetChange(operation)
	changed, err := r.reconcileStatus(ctx, job, jobSet)
	if err != nil {
		r.Metrics.recordError(errorOperationStatus)
		return done(reconcileError, err)
	}
	r.Metrics.recordStatusChange(changed)
	return done(reconcileSuccess, nil)
}

func (r *AIJobReconciler) reconcileJobSet(
	ctx context.Context,
	job *aiv1alpha1.AIJob,
) (*jobsetv1alpha2.JobSet, string, error) {
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
		// Kueue owns spec.suspend after admission; do not overwrite it here.
		return nil
	})
	return jobSet, operationLabel(operation), err
}

func (r *AIJobReconciler) reconcileStatus(
	ctx context.Context,
	job *aiv1alpha1.AIJob,
	jobSet *jobsetv1alpha2.JobSet,
) (bool, error) {
	status := statusFromJobSet(job.Generation, jobSet)
	if reflect.DeepEqual(job.Status, status) {
		return false, nil
	}
	job.Status = status
	return true, r.Status().Update(ctx, job)
}

func operationLabel(operation controllerutil.OperationResult) string {
	switch operation {
	case controllerutil.OperationResultCreated:
		return "create"
	case controllerutil.OperationResultUpdated, controllerutil.OperationResultUpdatedStatus,
		controllerutil.OperationResultUpdatedStatusOnly:
		return "update"
	default:
		return "unchanged"
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
	if queue := job.Labels[topology.QueueLabel]; queue != "" {
		labels[topology.QueueLabel] = queue
	}

	return &jobsetv1alpha2.JobSet{
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
}

// reconcileOwnedFields initializes immutable spec fields only when creating the
// JobSet. Webhooks and Kueue may default or inject fields into that spec later.
func reconcileOwnedFields(actual, desired *jobsetv1alpha2.JobSet) {
	if actual.Labels == nil {
		actual.Labels = make(map[string]string, 2)
	}
	actual.Labels[topology.JobLabel] = desired.Labels[topology.JobLabel]
	for _, key := range []string{topology.RunIDLabel, topology.ExperimentLabel} {
		if value := desired.Labels[key]; value != "" {
			actual.Labels[key] = value
		} else {
			delete(actual.Labels, key)
		}
	}
	if queue := desired.Labels[topology.QueueLabel]; queue != "" {
		actual.Labels[topology.QueueLabel] = queue
	} else {
		delete(actual.Labels, topology.QueueLabel)
	}

	if actual.CreationTimestamp.IsZero() {
		actual.Spec.ReplicatedJobs = desired.Spec.ReplicatedJobs
		actual.Spec.Network = desired.Spec.Network
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

func schedulingAnnotations(preference string) map[string]string {
	if preference == "same-rack" {
		return map[string]string{topology.RequiredTopologyAnnotation: topology.RackLabel}
	}
	if preference == topology.FabricNVLink || preference == topology.FabricPCIe {
		return map[string]string{topology.PreferenceAnnotation: preference}
	}
	return nil
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

func boolPtr(value bool) *bool { return &value }

func ptr[T any](value T) *T { return &value }
