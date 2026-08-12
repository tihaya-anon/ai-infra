package controller

import (
	"context"
	"fmt"
	"reflect"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const SchedulerName = "ai-scheduler"

var _ reconcile.Reconciler = &AIJobReconciler{}

// AIJobReconciler turns an AIJob into worker Pods.
type AIJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile makes the observed worker Pods match AIJob.spec.
func (r *AIJobReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	job := &aiv1alpha1.AIJob{}
	if err := r.Get(ctx, request.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(job.Namespace), client.MatchingLabels{topology.JobLabel: job.Name}); err != nil {
		return ctrl.Result{}, err
	}
	expectedNames := make(map[string]struct{}, job.Spec.Workers)
	for index := int32(0); index < job.Spec.Workers; index++ {
		name := fmt.Sprintf("%s-worker-%d", job.Name, index)
		expectedNames[name] = struct{}{}
		if containsPod(pods.Items, name) {
			continue
		}
		pod := workerPod(job, name)
		if err := ctrl.SetControllerReference(job, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
	}
	for index := range pods.Items {
		pod := &pods.Items[index]
		if _, expected := expectedNames[pod.Name]; expected || !metav1.IsControlledBy(pod, job) {
			continue
		}
		if err := r.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	status := aiv1alpha1.WorkerStatus(pods.Items)
	if reflect.DeepEqual(job.Status, status) {
		return ctrl.Result{}, nil
	}
	job.Status = status
	return ctrl.Result{}, r.Status().Update(ctx, job)
}

// SetupWithManager declares that Pod changes must reconcile their owning AIJob.
func (r *AIJobReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&aiv1alpha1.AIJob{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func workerPod(job *aiv1alpha1.AIJob, name string) *corev1.Pod {
	image := job.Spec.Image
	if image == "" {
		image = "ai-infra-lab:dev"
	}
	gpus := *resource.NewQuantity(job.Spec.GPUPerWorker, resource.DecimalSI)
	gpuResource := corev1.ResourceName(job.Spec.GPUResource)
	if gpuResource == "" {
		gpuResource = topology.GPUResource
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: job.Namespace,
			Labels:      map[string]string{topology.JobLabel: job.Name},
			Annotations: map[string]string{topology.PreferenceAnnotation: job.Spec.Topology},
		},
		Spec: corev1.PodSpec{
			SchedulerName: SchedulerName, RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name: "worker", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
				Command:   []string{"/worker"},
				Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{gpuResource: gpus}},
			}},
		},
	}
}

func containsPod(pods []corev1.Pod, name string) bool {
	return findPod(pods, name) != nil
}

func findPod(pods []corev1.Pod, name string) *corev1.Pod {
	for index := range pods {
		pod := &pods[index]
		if pod.Name == name {
			return pod
		}
	}
	return nil
}
