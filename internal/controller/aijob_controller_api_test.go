//go:build api_test

package controller

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

func TestAPIReconciliation(t *testing.T) {
	jobSetCRD := os.Getenv("JOBSET_CRD_PATH")
	if jobSetCRD == "" {
		t.Fatal("JOBSET_CRD_PATH is required; run make test-api")
	}
	environment := &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../deploy/crd.yaml", jobSetCRD},
		ErrorIfCRDPathMissing: true,
	}
	config, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	mustTest(t, clientgoscheme.AddToScheme(scheme))
	mustTest(t, aiv1alpha1.AddToScheme(scheme))
	mustTest(t, jobsetv1alpha2.AddToScheme(scheme))
	apiClient, err := client.New(config, client.Options{Scheme: scheme})
	mustTest(t, err)
	reconciler := &AIJobReconciler{Client: apiClient, Scheme: scheme}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := types.NamespacedName{Name: "api-test", Namespace: "default"}
	job := apiTestAIJob(key)
	mustTest(t, apiClient.Create(ctx, job))
	reconcileAPI(t, ctx, reconciler, key)
	assertDesiredJobSet(t, ctx, apiClient, job, key)

	jobSet := &jobsetv1alpha2.JobSet{}
	mustTest(t, apiClient.Get(ctx, key, jobSet))
	defaultedSpec := jobSet.Spec.DeepCopy()
	jobSet.Labels["jobset.x-k8s.io/defaulted"] = "keep"
	mustTest(t, apiClient.Update(ctx, jobSet))
	reconcileAPI(t, ctx, reconciler, key)
	assertPreservedAndUnique(t, ctx, apiClient, key, defaultedSpec)

	mustTest(t, apiClient.Get(ctx, key, jobSet))
	jobSet.Status.Conditions = []metav1.Condition{{
		Type: "Completed", Status: metav1.ConditionTrue,
		Reason: "AllJobsCompleted", Message: "all jobs completed",
		LastTransitionTime: metav1.Now(),
	}}
	mustTest(t, apiClient.Status().Update(ctx, jobSet))
	reconcileAPI(t, ctx, reconciler, key)
	assertProjectedStatus(t, ctx, apiClient, key)
}

func apiTestAIJob(key types.NamespacedName) *aiv1alpha1.AIJob {
	return &aiv1alpha1.AIJob{
		TypeMeta: metav1.TypeMeta{APIVersion: aiv1alpha1.GroupVersion.String(), Kind: "AIJob"},
		ObjectMeta: metav1.ObjectMeta{
			Name: key.Name, Namespace: key.Namespace,
			Labels: map[string]string{topology.QueueLabel: "training"},
		},
		Spec: aiv1alpha1.AIJobSpec{
			Workers: 2, GPUPerWorker: 1, Topology: "nvlink",
			Image: "worker:test", Args: []string{"--mode=complete", "--duration=1s"},
		},
	}
}

func reconcileAPI(
	t *testing.T,
	ctx context.Context,
	reconciler *AIJobReconciler,
	key types.NamespacedName,
) {
	t.Helper()
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	mustTest(t, err)
}

func assertDesiredJobSet(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	job *aiv1alpha1.AIJob,
	key types.NamespacedName,
) {
	t.Helper()
	jobSet := &jobsetv1alpha2.JobSet{}
	mustTest(t, apiClient.Get(ctx, key, jobSet))
	if !metav1.IsControlledBy(jobSet, job) {
		t.Fatalf("JobSet is not controlled by AIJob: %#v", jobSet.OwnerReferences)
	}
	worker := jobSet.Spec.ReplicatedJobs[0].Template.Spec
	container := worker.Template.Spec.Containers[0]
	if worker.CompletionMode == nil || *worker.CompletionMode != batchv1.IndexedCompletion {
		t.Fatalf("unexpected completion mode: %#v", worker.CompletionMode)
	}
	if !reflect.DeepEqual(container.Args, job.Spec.Args) {
		t.Fatalf("got args %#v, want %#v", container.Args, job.Spec.Args)
	}
	if container.Env[0].ValueFrom.FieldRef.FieldPath !=
		"metadata.labels['batch.kubernetes.io/job-completion-index']" {
		t.Fatalf("unexpected completion index source: %#v", container.Env)
	}
	if worker.Template.Spec.SchedulerName != SchedulerName ||
		worker.Template.Annotations[topology.PreferenceAnnotation] != "nvlink" {
		t.Fatalf("scheduling intent was not propagated: %#v", worker.Template)
	}
	if got := container.Resources.Requests[corev1.ResourceName("example.com/gpu")]; got.Value() != 1 {
		t.Fatalf("got GPU request %s", got.String())
	}
}

func assertPreservedAndUnique(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	key types.NamespacedName,
	defaultedSpec *jobsetv1alpha2.JobSetSpec,
) {
	t.Helper()
	list := &jobsetv1alpha2.JobSetList{}
	mustTest(t, apiClient.List(ctx, list, client.InNamespace(key.Namespace)))
	if len(list.Items) != 1 {
		t.Fatalf("got %d JobSets, want one", len(list.Items))
	}
	jobSet := list.Items[0]
	if jobSet.Labels["jobset.x-k8s.io/defaulted"] != "keep" ||
		!reflect.DeepEqual(jobSet.Spec, *defaultedSpec) {
		t.Fatalf("reconciliation removed external fields: %#v", jobSet)
	}
}

func assertProjectedStatus(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	key types.NamespacedName,
) {
	t.Helper()
	job := &aiv1alpha1.AIJob{}
	mustTest(t, apiClient.Get(ctx, key, job))
	if len(job.Status.Conditions) != 1 || job.Status.Conditions[0].Type != "Completed" {
		t.Fatalf("unexpected AIJob status: %#v", job.Status)
	}
	if job.Status.ObservedGeneration != job.Generation ||
		job.Status.Conditions[0].ObservedGeneration != job.Generation {
		t.Fatalf("status generation is stale: %#v", job.Status)
	}
}

func mustTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
