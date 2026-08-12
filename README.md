# 手写 Kubernetes Controller 与 Scheduler

这是一个面向 AI Infra 入门的最小可运行实验。它实现 `AIJob` CRD、Controller 和独立 Scheduler，并在 kind 节点上用 Label 模拟 GPU 容量与机架拓扑。

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
