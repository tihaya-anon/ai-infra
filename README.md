# Kubernetes AIJob、JobSet 与 GPU 拓扑调度

这是一个面向 AI Infra 入门的实验：使用薄 `AIJob` Controller 生成 JobSet，复用 Kueue 完成队列和整组拓扑准入，并通过 Scheduler Framework `ScorePlugin` 演示集群特有的 GPU 节点偏好。具体 GPU 的 PCIe/NVLink 设备分配属于 DRA 或厂商设备驱动。

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
