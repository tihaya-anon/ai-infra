# Kubernetes AIJob、JobSet 与 GPU 拓扑调度

这是一个面向 AI Infra 入门的实验：使用薄 `AIJob` Controller 生成 JobSet，复用 Kueue 完成队列和整组拓扑准入，并通过 Scheduler Framework `ScorePlugin` 演示集群特有的 GPU 节点偏好。具体 GPU 的 PCIe/NVLink 设备分配属于 DRA 或厂商设备驱动。

文档分为两个入口：

- [教程](docs/tutorial.md)：理解架构、组件边界、GPU 拓扑背景和实验操作；
- [源码阅读指南](docs/code-reading-guide.md)：按数据流跟读文件、关键函数和测试。

运行实验：

```bash
make test
make cluster CLUSTER=ai-infra-lab-v134
make deploy CLUSTER=ai-infra-lab-v134
make demo
kubectl get aijob,pods -o wide
```

实验固定使用 Kubernetes 1.34.8。构建默认通过 DaoCloud 代理获取 `gcr.io` 的 distroless 基础镜像；切回官方源可传入 `RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot`。完整的环境要求、组件职责和排错方式见教程。

开发时运行 `make fmt` 格式化并整理 Go import，运行 `make verify` 检查格式、100 字符行宽、`go vet` 和测试。首次克隆后运行 `make hooks` 安装相同的 pre-commit 流程；提交涉及 Go 源码或模块文件时会自动执行。

清理实验集群：

```bash
make clean CLUSTER=ai-infra-lab-v134
```
