# Kubernetes AIJob、JobSet 与 GPU 拓扑调度

这是一个面向 AI Infra 入门的实验：使用薄 `AIJob` Controller 生成 JobSet，复用 Kueue
完成队列和整组拓扑准入，并通过 Scheduler Framework `ScorePlugin` 演示集群特有的 GPU
节点偏好。项目还提供确定性的合成 Worker、三层验证、调度碎片对照实验和可追溯证据包。
具体 GPU 的 PCIe/NVLink 设备分配仍属于 DRA 或厂商设备驱动。

文档分为两个入口：

- [教程](docs/tutorial.md)：理解架构、组件边界、GPU 拓扑背景和实验操作；
- [源码阅读指南](docs/code-reading-guide.md)：按数据流跟读文件、关键函数和测试；
- [调度实验指南](docs/scheduling-experiment.md)：运行 baseline/optimized 对照并解释原始结果；
- [生产化差异说明](docs/production-vs-teaching.md)：说明教学实验走向生产控制面的保留点、重做点和模块结构；
- [实验报告模板](docs/experiment-report-template.md)：记录环境、证据、实测结果和限制。

运行实验：

```bash
./scripts/install-dev-deps.sh
make verify
make test-api
make cluster CLUSTER=ai-infra-lab-v134
make deploy CLUSTER=ai-infra-lab-v134
make demo
make headlamp CLUSTER=ai-infra-lab-v134
kubectl get aijob,pods -o wide
make test-e2e CLUSTER=ai-infra-lab-v134
make benchmark CLUSTER=ai-infra-lab-v134
```

不要把这些命令当成一组重复测试。`make verify` 验证纯函数和转换逻辑，不需要集群；
`make test-api` 启动固定版本的 envtest API Server 验证 Reconcile；`make test-e2e` 才验证
部署后的 JobSet、Kueue、Scheduler、Worker 与垃圾回收。benchmark 结果默认写入忽略版本控制的
`out/benchmark/`，没有真实执行就不应在报告中填写 measured 数据。

`scripts/install-dev-deps.sh` 面向 Ubuntu/Debian Linux，安装 Go 1.24.0、kubectl 1.34.8、
kind、Docker Engine、make、Bash、pre-commit 和项目本地 Go 工具。`make headlamp`
会在当前 kind 集群中安装 Headlamp，并创建本地实验用的管理员 ServiceAccount。

实验固定使用 Kubernetes 1.34.8。kind 节点镜像、builder 镜像、distroless 基础镜像和
JobSet/Kueue 控制器镜像默认通过 DaoCloud 代理获取，Go 模块默认通过 `goproxy.cn` 下载；
切回官方源可覆盖 `KIND_NODE_IMAGE`、`GO_BUILDER_IMAGE`、`GOPROXY`、`RUNTIME_IMAGE` 和
`EXTERNAL_IMAGE_MIRROR=`。完整的环境要求、组件职责和排错方式见教程。

开发时运行 `make fmt` 格式化并整理 Go import，运行 `make verify` 检查格式、100 字符行宽、`go vet` 和测试。首次克隆后运行 `make hooks` 安装相同的 pre-commit 流程；提交涉及 Go 源码或模块文件时会自动执行。

清理实验集群：

```bash
make clean CLUSTER=ai-infra-lab-v134
```
