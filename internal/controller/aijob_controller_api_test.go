//go:build api_test

package controller

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/onsi/gomega"
	aiv1alpha1 "github.com/tihaya-anon/ai-infra-lab/api/v1alpha1"
	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	jobsetv1alpha2 "sigs.k8s.io/jobset/api/jobset/v1alpha2"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

func TestGivenAPIEnvironmentWhenReconcilingAIJobThenResourcesAndStatusAreProjected(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	jobSetCRD := os.Getenv("JOBSET_CRD_PATH")
	assert.Expect(jobSetCRD).NotTo(gomega.BeEmpty(), "JOBSET_CRD_PATH is required; run make test-api")
	localQueueCRD := os.Getenv("KUEUE_LOCALQUEUE_CRD_PATH")
	assert.Expect(localQueueCRD).NotTo(
		gomega.BeEmpty(), "KUEUE_LOCALQUEUE_CRD_PATH is required; run make test-api",
	)
	environment := &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../deploy/crd.yaml", jobSetCRD, localQueueCRD},
		ErrorIfCRDPathMissing: true,
	}
	config, err := environment.Start()
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	t.Cleanup(func() {
		assert.Expect(environment.Stop()).To(gomega.Succeed())
	})

	scheme := runtime.NewScheme()
	assert.Expect(clientgoscheme.AddToScheme(scheme)).To(gomega.Succeed())
	assert.Expect(aiv1alpha1.AddToScheme(scheme)).To(gomega.Succeed())
	assert.Expect(jobsetv1alpha2.AddToScheme(scheme)).To(gomega.Succeed())
	assert.Expect(kueuev1beta1.AddToScheme(scheme)).To(gomega.Succeed())
	apiClient, err := client.New(config, client.Options{Scheme: scheme})
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	reconciler := &AIJobReconciler{Client: apiClient, Scheme: scheme}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := types.NamespacedName{Name: "api-test", Namespace: "default"}
	queue := &kueuev1beta1.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{Name: aiv1alpha1.DefaultQueueName, Namespace: key.Namespace},
		Spec: kueuev1beta1.LocalQueueSpec{
			ClusterQueue: kueuev1beta1.ClusterQueueReference(aiv1alpha1.DefaultQueueName),
		},
	}
	assert.Expect(apiClient.Create(ctx, queue)).To(gomega.Succeed())
	job := apiTestAIJob(key)
	assert.Expect(apiClient.Create(ctx, job)).To(gomega.Succeed())
	assert.Expect(job.Spec.QueueName).To(gomega.Equal(aiv1alpha1.DefaultQueueName))
	invalid := apiTestAIJob(types.NamespacedName{Name: "invalid-queue", Namespace: key.Namespace})
	invalid.Spec.QueueName = "NOT_VALID"
	assert.Expect(apiClient.Create(ctx, invalid)).NotTo(gomega.Succeed())

	// when
	reconcileAPI(t, ctx, reconciler, key)

	// then
	assertDesiredJobSet(t, ctx, apiClient, job, key)

	// given
	jobSet := &jobsetv1alpha2.JobSet{}
	assert.Expect(apiClient.Get(ctx, key, jobSet)).To(gomega.Succeed())
	defaultedSpec := jobSet.Spec.DeepCopy()
	jobSet.Labels["jobset.x-k8s.io/defaulted"] = "keep"
	assert.Expect(apiClient.Update(ctx, jobSet)).To(gomega.Succeed())

	// when
	reconcileAPI(t, ctx, reconciler, key)

	// then
	assertPreservedAndUnique(t, ctx, apiClient, key, defaultedSpec)

	// given
	assert.Expect(apiClient.Get(ctx, key, jobSet)).To(gomega.Succeed())
	jobSet.Status.Conditions = []metav1.Condition{{
		Type: "Completed", Status: metav1.ConditionTrue,
		Reason: "AllJobsCompleted", Message: "all jobs completed",
		LastTransitionTime: metav1.Now(),
	}}
	assert.Expect(apiClient.Status().Update(ctx, jobSet)).To(gomega.Succeed())

	// when
	reconcileAPI(t, ctx, reconciler, key)

	// then
	assertProjectedStatus(t, ctx, apiClient, key)
}

func apiTestAIJob(key types.NamespacedName) *aiv1alpha1.AIJob {
	return &aiv1alpha1.AIJob{
		TypeMeta:   metav1.TypeMeta{APIVersion: aiv1alpha1.GroupVersion.String(), Kind: "AIJob"},
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		Spec: aiv1alpha1.AIJobSpec{
			Workers: 2, GPUPerWorker: 1,
			Topology: aiv1alpha1.Topology{Preference: "nvlink"},
			Image:    "worker:test", Args: []string{"--mode=complete", "--duration=1s"},
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
	assert := gomega.NewWithT(t)
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	assert.Expect(err).NotTo(gomega.HaveOccurred())
}

func assertDesiredJobSet(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	job *aiv1alpha1.AIJob,
	key types.NamespacedName,
) {
	t.Helper()
	assert := gomega.NewWithT(t)
	jobSet := &jobsetv1alpha2.JobSet{}
	assert.Expect(apiClient.Get(ctx, key, jobSet)).To(gomega.Succeed())
	assert.Expect(metav1.IsControlledBy(jobSet, job)).To(gomega.BeTrue())
	assert.Expect(jobSet.Labels).To(
		gomega.HaveKeyWithValue(topology.QueueLabel, aiv1alpha1.DefaultQueueName),
	)
	worker := jobSet.Spec.ReplicatedJobs[0].Template.Spec
	container := worker.Template.Spec.Containers[0]
	assert.Expect(worker.CompletionMode).NotTo(gomega.BeNil())
	assert.Expect(*worker.CompletionMode).To(gomega.Equal(batchv1.IndexedCompletion))
	assert.Expect(container.Args).To(gomega.Equal(job.Spec.Args))
	assert.Expect(container.Env[0].ValueFrom.FieldRef.FieldPath).To(
		gomega.Equal("metadata.labels['batch.kubernetes.io/job-completion-index']"),
	)
	assert.Expect(worker.Template.Spec.SchedulerName).To(gomega.Equal(SchedulerName))
	assert.Expect(worker.Template.Annotations).To(
		gomega.HaveKeyWithValue(topology.PreferenceAnnotation, "nvlink"),
	)
	gpuRequest := container.Resources.Requests[corev1.ResourceName("example.com/gpu")]
	assert.Expect(gpuRequest.Value()).To(gomega.Equal(int64(1)))
}

func assertPreservedAndUnique(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	key types.NamespacedName,
	defaultedSpec *jobsetv1alpha2.JobSetSpec,
) {
	t.Helper()
	assert := gomega.NewWithT(t)
	list := &jobsetv1alpha2.JobSetList{}
	assert.Expect(apiClient.List(ctx, list, client.InNamespace(key.Namespace))).To(gomega.Succeed())
	assert.Expect(list.Items).To(gomega.HaveLen(1))
	jobSet := list.Items[0]
	assert.Expect(jobSet.Labels).To(gomega.HaveKeyWithValue("jobset.x-k8s.io/defaulted", "keep"))
	assert.Expect(jobSet.Spec).To(gomega.Equal(*defaultedSpec))
}

func assertProjectedStatus(
	t *testing.T,
	ctx context.Context,
	apiClient client.Client,
	key types.NamespacedName,
) {
	t.Helper()
	assert := gomega.NewWithT(t)
	job := &aiv1alpha1.AIJob{}
	assert.Expect(apiClient.Get(ctx, key, job)).To(gomega.Succeed())
	completed := apiMeta.FindStatusCondition(job.Status.Conditions, "Completed")
	assert.Expect(completed).NotTo(gomega.BeNil())
	queueReady := apiMeta.FindStatusCondition(job.Status.Conditions, aiv1alpha1.ConditionQueueReady)
	assert.Expect(queueReady).NotTo(gomega.BeNil())
	assert.Expect(queueReady.Status).To(gomega.Equal(metav1.ConditionTrue))
	assert.Expect(job.Status.ObservedGeneration).To(gomega.Equal(job.Generation))
	assert.Expect(completed.ObservedGeneration).To(gomega.Equal(job.Generation))
}
