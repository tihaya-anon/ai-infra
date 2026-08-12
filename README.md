# Kubernetes AIJob Controller 与 GPU 拓扑调度插件

这是一个面向 AI Infra 入门的最小实验：使用 controller-runtime 实现 `AIJob` Reconcile，并通过 Scheduler Framework `ScorePlugin` 扩展默认 kube-scheduler 的 GPU 拓扑偏好。

先阅读 [教程](docs/tutorial.md)，再运行：

```bash
make test
make cluster
make deploy
make demo
kubectl get aijob,pods -o wide
```

清理实验集群：

```bash
make clean
```
