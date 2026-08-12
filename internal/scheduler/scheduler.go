package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	schedulerName    = "ai-scheduler"
	gpuCapacityLabel = "infra.example.io/gpu-capacity"
	gpuRequestLabel  = "infra.example.io/gpu-request"
	rackLabel        = "infra.example.io/rack"
)

type Scheduler struct {
	kube   kubernetes.Interface
	pods   corelisters.PodLister
	nodes  corelisters.NodeLister
	queue  workqueue.RateLimitingInterface
	start  func(<-chan struct{})
	synced cache.InformerSynced
	logger *slog.Logger
}

type nodeState struct {
	name     string
	rack     string
	capacity int64
	used     int64
}

func New(config *rest.Config, logger *slog.Logger) *Scheduler {
	client := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactory(client, 30*time.Second)
	pods := factory.Core().V1().Pods()
	nodes := factory.Core().V1().Nodes()
	scheduler := &Scheduler{
		kube: client, pods: pods.Lister(), nodes: nodes.Lister(),
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "unscheduled-ai-pods"),
		start: factory.Start, logger: logger,
	}
	scheduler.synced = func() bool { return pods.Informer().HasSynced() && nodes.Informer().HasSynced() }
	pods.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    scheduler.enqueue,
		UpdateFunc: func(oldObject, newObject any) { scheduler.enqueue(newObject) },
	})
	return scheduler
}

func (s *Scheduler) Run(ctx context.Context) error {
	defer s.queue.ShutDown()
	s.start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), s.synced) {
		return fmt.Errorf("sync Pod and Node informer caches")
	}
	s.logger.Info("scheduler started", "schedulerName", schedulerName)
	go wait.UntilWithContext(ctx, s.runWorker, time.Second)
	<-ctx.Done()
	return nil
}

func (s *Scheduler) enqueue(object any) {
	pod, ok := object.(*corev1.Pod)
	if !ok || pod.Spec.SchedulerName != schedulerName || pod.Spec.NodeName != "" {
		return
	}
	key, err := cache.MetaNamespaceKeyFunc(pod)
	if err == nil {
		s.queue.Add(key)
	}
}

func (s *Scheduler) runWorker(ctx context.Context) {
	for s.processNext(ctx) {
	}
}

func (s *Scheduler) processNext(ctx context.Context) bool {
	item, shutdown := s.queue.Get()
	if shutdown {
		return false
	}
	defer s.queue.Done(item)

	key := item.(string)
	if err := s.schedule(ctx, key); err != nil {
		s.logger.Warn("schedule Pod", "pod", key, "error", err)
		s.queue.AddRateLimited(key)
		return true
	}
	s.queue.Forget(item)
	return true
}

func (s *Scheduler) schedule(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	pod, err := s.pods.Pods(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if pod.Spec.NodeName != "" {
		return nil
	}
	// Read occupancy from the API server so a just-completed Bind is visible
	// before the next queued Pod is scored.
	allPodsList, err := s.kube.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	allNodes, err := s.nodes.List(labels.Everything())
	if err != nil {
		return err
	}
	allPods := make([]*corev1.Pod, 0, len(allPodsList.Items))
	for index := range allPodsList.Items {
		allPods = append(allPods, &allPodsList.Items[index])
	}

	node, score, err := chooseNode(pod, allNodes, allPods)
	if err != nil {
		return err
	}
	binding := &corev1.Binding{ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace}, Target: corev1.ObjectReference{Kind: "Node", Name: node}}
	if err := s.kube.CoreV1().Pods(namespace).Bind(ctx, binding, metav1.CreateOptions{}); err != nil {
		return err
	}
	s.logger.Info("bound Pod", "pod", key, "node", node, "score", score)
	return nil
}

func chooseNode(pod *corev1.Pod, nodes []*corev1.Node, pods []*corev1.Pod) (string, int64, error) {
	request, err := positiveLabel(pod.Labels, gpuRequestLabel)
	if err != nil {
		return "", 0, err
	}
	states := buildNodeStates(nodes, pods)
	preferredRack := currentJobRack(pod, pods, states)

	bestName := ""
	bestScore := int64(-1)
	for _, state := range states {
		free := state.capacity - state.used
		if free < request {
			continue
		}
		// Fill partially used nodes first; preserve untouched nodes for larger jobs.
		score := state.used * 10
		if preferredRack != "" && state.rack == preferredRack {
			score += 1000
		}
		if score > bestScore || (score == bestScore && state.name < bestName) {
			bestName, bestScore = state.name, score
		}
	}
	if bestName == "" {
		return "", 0, fmt.Errorf("no node has %d simulated GPUs available", request)
	}
	return bestName, bestScore, nil
}

func buildNodeStates(nodes []*corev1.Node, pods []*corev1.Pod) map[string]*nodeState {
	states := make(map[string]*nodeState, len(nodes))
	for _, node := range nodes {
		capacity, err := positiveLabel(node.Labels, gpuCapacityLabel)
		if err != nil || node.Spec.Unschedulable {
			continue
		}
		states[node.Name] = &nodeState{name: node.Name, rack: node.Labels[rackLabel], capacity: capacity}
	}
	for _, pod := range pods {
		state := states[pod.Spec.NodeName]
		if state == nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		request, err := positiveLabel(pod.Labels, gpuRequestLabel)
		if err == nil {
			state.used += request
		}
	}
	return states
}

func currentJobRack(pod *corev1.Pod, pods []*corev1.Pod, states map[string]*nodeState) string {
	jobName := pod.Labels["infra.example.io/aijob"]
	if pod.Labels["infra.example.io/topology"] != "same-rack" || jobName == "" {
		return ""
	}
	for _, peer := range pods {
		if peer.Labels["infra.example.io/aijob"] == jobName && peer.Spec.NodeName != "" {
			if state := states[peer.Spec.NodeName]; state != nil {
				return state.rack
			}
		}
	}
	return ""
}

func positiveLabel(labels map[string]string, name string) (int64, error) {
	value, err := strconv.ParseInt(labels[name], 10, 64)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("label %s must be a positive integer", name)
	}
	return value, nil
}
