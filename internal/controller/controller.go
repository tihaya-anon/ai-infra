package controller

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/tihaya-anon/ai-infra-lab/internal/aijob"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const schedulerName = "ai-scheduler"

// Controller reconciles AIJob resources into their desired worker Pods.
type Controller struct {
	kube    kubernetes.Interface
	dynamic dynamic.Interface
	lister  cache.GenericLister
	queue   workqueue.RateLimitingInterface
	start   func(<-chan struct{})
	synced  cache.InformerSynced
	logger  *slog.Logger
}

// New wires AIJob and Pod informers to a shared rate-limited queue.
func New(config *rest.Config, logger *slog.Logger) *Controller {
	kubeClient := kubernetes.NewForConfigOrDie(config)
	dynamicClient := dynamic.NewForConfigOrDie(config)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 10*time.Minute, metav1.NamespaceAll, nil)
	informer := factory.ForResource(aijob.GVR)
	podFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	podInformer := podFactory.Core().V1().Pods().Informer()
	controller := &Controller{
		kube: kubeClient, dynamic: dynamicClient, lister: informer.Lister(),
		queue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "aijobs"),
		synced: informer.Informer().HasSynced, logger: logger,
	}
	controller.start = func(stop <-chan struct{}) {
		factory.Start(stop)
		podFactory.Start(stop)
	}
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueue,
		UpdateFunc: func(oldObject, newObject any) { controller.enqueue(newObject) },
		DeleteFunc: controller.enqueue,
	})
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueOwner,
		UpdateFunc: func(oldObject, newObject any) { controller.enqueueOwner(newObject) },
		DeleteFunc: controller.enqueueOwner,
	})
	controller.synced = func() bool { return informer.Informer().HasSynced() && podInformer.HasSynced() }
	return controller
}

func (c *Controller) enqueueOwner(object any) {
	pod, ok := object.(*corev1.Pod)
	if !ok {
		return
	}
	name := pod.Labels["infra.example.io/aijob"]
	if name != "" {
		c.queue.Add(pod.Namespace + "/" + name)
	}
}

// Run starts informers, waits for cache synchronization, and processes AIJobs.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer c.queue.ShutDown()
	c.start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), c.synced) {
		return fmt.Errorf("sync AIJob informer cache")
	}

	c.logger.Info("controller started", "workers", workers)
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}
	<-ctx.Done()
	return nil
}

func (c *Controller) enqueue(object any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(object)
	if err != nil {
		c.logger.Error("build queue key", "error", err)
		return
	}
	c.queue.Add(key)
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNext(ctx) {
	}
}

func (c *Controller) processNext(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key := item.(string)
	if err := c.reconcile(ctx, key); err != nil {
		c.logger.Error("reconcile AIJob", "key", key, "error", err)
		c.queue.AddRateLimited(key)
		return true
	}
	c.queue.Forget(item)
	return true
}

func (c *Controller) reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	object, err := c.lister.ByNamespace(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	job := object.(*unstructured.Unstructured)
	spec, err := aijob.Parse(job)
	if err != nil {
		return err
	}

	selector := labels.Set{"infra.example.io/aijob": name}.AsSelector().String()
	pods, err := c.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	for index := int64(0); index < spec.Workers; index++ {
		// Stable names make repeated reconciliation idempotent.
		podName := fmt.Sprintf("%s-worker-%d", name, index)
		if podExists(pods.Items, podName) {
			continue
		}
		if _, err := c.kube.CoreV1().Pods(namespace).Create(ctx, workerPod(job, spec, podName), metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		c.logger.Info("created worker", "aijob", key, "pod", podName)
	}
	return c.updateStatus(ctx, job, pods.Items)
}

func podExists(pods []corev1.Pod, name string) bool {
	for _, pod := range pods {
		if pod.Name == name {
			return true
		}
	}
	return false
}

func workerPod(job *unstructured.Unstructured, spec aijob.Spec, name string) *corev1.Pod {
	labels := map[string]string{
		"infra.example.io/aijob":       job.GetName(),
		"infra.example.io/gpu-request": fmt.Sprint(spec.GPUPerWorker),
		"infra.example.io/topology":    spec.Topology,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: job.GetNamespace(), Labels: labels, OwnerReferences: []metav1.OwnerReference{aijob.OwnerReference(job)}},
		Spec: corev1.PodSpec{
			SchedulerName: schedulerName, RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{Name: "worker", Image: spec.Image, ImagePullPolicy: corev1.PullIfNotPresent, Args: []string{"--component=worker"}}},
		},
	}
}

func (c *Controller) updateStatus(ctx context.Context, job *unstructured.Unstructured, pods []corev1.Pod) error {
	var pending, running, succeeded, failed int64
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			running++
		case corev1.PodSucceeded:
			succeeded++
		case corev1.PodFailed:
			failed++
		default:
			pending++
		}
	}
	status := map[string]any{"pending": pending, "running": running, "succeeded": succeeded, "failed": failed}
	// Avoid a status write that would immediately enqueue the same AIJob again.
	if reflect.DeepEqual(job.Object["status"], status) {
		return nil
	}
	copy := job.DeepCopy()
	copy.Object["status"] = status
	_, err := c.dynamic.Resource(aijob.GVR).Namespace(job.GetNamespace()).UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	return err
}
