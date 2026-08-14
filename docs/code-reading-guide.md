# 源码阅读指南

本文是项目源码的导航索引，关注“代码在哪里、按什么顺序读、各文件如何接力”。Kubernetes 控制循环、JobSet、Kueue 和 GPU 拓扑等背景知识，以及安装和实验步骤，统一放在[教程](tutorial.md)中，不在这里重复展开。

## 阅读主线

建议沿着一份 `AIJob` 从声明到运行的方向阅读：

```text
AIJob YAML
  -> CRD 与 Go API 类型
  -> Controller 启动和 Reconcile
  -> JobSet、Job 与 Pod 模板
  -> Kueue 准入和 Scheduler 节点评分
  -> Worker 容器进程
```

### 1. 从用户提交的对象开始

1. [`examples/aijob.yaml`](../examples/aijob.yaml)：最小的业务输入。先记住 `workers`、
   `gpuPerWorker`、`topology`、有序 `args` 和队列 Label，后续查它们分别被谁消费。
2. [`deploy/crd.yaml`](../deploy/crd.yaml)：API Server 看到的 `AIJob` 契约，包括字段校验、默认值、不可变规则、status 子资源和打印列。
3. [`api/v1alpha1/aijob_types.go`](../api/v1alpha1/aijob_types.go)：Controller 使用的 Go 类型，以及 Kubernetes runtime 所需的深拷贝实现。对照 CRD 阅读，可以区分服务端约束和进程内数据结构。
4. [`api/v1alpha1/groupversion_info.go`](../api/v1alpha1/groupversion_info.go)：把 `AIJob`、`AIJobList` 注册到 `infra.example.io/v1alpha1` Scheme。

### 2. 看 Controller 如何启动

1. [`cmd/controller/main.go`](../cmd/controller/main.go)：进程入口。重点看内置类型、AIJob 和 JobSet 如何加入同一个 Scheme，以及 Manager 如何创建并启动 Reconciler。
2. [`internal/controller/aijob_controller.go`](../internal/controller/aijob_controller.go) 中的 `SetupWithManager`：声明 Controller 监听 `AIJob`，同时把其拥有的 `JobSet` 变更映射回父对象。

读到这里，应能回答“一个 AIJob 变化为什么会进入 `Reconcile`”以及“JobSet 状态变化为什么也会触发它”。

### 3. 沿 Reconcile 主路径阅读

继续阅读 [`internal/controller/aijob_controller.go`](../internal/controller/aijob_controller.go)，建议按函数调用顺序展开：

1. `Reconcile`：按 namespace/name 读取最新 AIJob，依次协调下游 JobSet 和上游 status。
2. `reconcileJobSet`、`desiredJobSet`：构造目标 JobSet，并用 OwnerReference 建立生命周期关系。
3. `workerJobSpec`、`workerContainer`、`schedulingAnnotations`：把 worker 数量、镜像、GPU 请求和拓扑意图翻译到 Indexed Job 与 PodTemplate。
4. `reconcileOwnedFields`：只维护本 Controller 拥有的字段，避免覆盖 JobSet webhook 或 Kueue 写入的内容。
5. `reconcileStatus`、`statusFromJobSet`：把 JobSet Conditions 投影回 AIJob status，并避免无变化更新。
6. [`internal/controller/aijob_controller_test.go`](../internal/controller/aijob_controller_test.go)：紧接着验证翻译结果、字段所有权和 status 投影；测试用例也是这段代码最短的行为规格。

### 4. 跟踪拓扑意图到调度器

1. [`internal/topology/constants.go`](../internal/topology/constants.go)：Controller、Scheduler Plugin、部署清单之间共享的 Label、Annotation 和模拟 GPU 资源名。遇到字符串时先回到这里确认协议含义。
2. [`internal/plugin/gputopology/plugin.go`](../internal/plugin/gputopology/plugin.go)：实现 Scheduler Framework `ScorePlugin`。阅读 `preference` 和 `topologyScore`，确认它只对已通过默认过滤的 Node 打分。
3. [`internal/plugin/gputopology/plugin_test.go`](../internal/plugin/gputopology/plugin_test.go)：列出 NVLink、PCIe 和 `same-rack` 的评分边界；后者不由该插件处理。
4. [`cmd/scheduler/main.go`](../cmd/scheduler/main.go)：将插件工厂注册进自定义 kube-scheduler 二进制。
5. [`deploy/scheduler-config.yaml`](../deploy/scheduler-config.yaml)：用 `ai-scheduler` profile 启用插件并设置权重，同时展示调度器进程的部署方式。

这一段的关键连接是：Controller 写入 Pod Annotation 和 `schedulerName`，调度器 profile 选中 Pod，插件再读取 Annotation 并返回节点分数。

### 5. 最后补齐运行和部署边界

1. [`cmd/worker/main.go`](../cmd/worker/main.go) 与
   [`internal/worker/worker.go`](../internal/worker/worker.go)：从信号入口读到参数校验、complete/wait
   状态机、indexed identity 和 NDJSON lifecycle 记录。
2. [`Dockerfile`](../Dockerfile)：把 Controller、Scheduler 和 Worker 构建为三个二进制，并放入同一个运行时镜像。
3. [`deploy/controller.yaml`](../deploy/controller.yaml) 与 [`deploy/rbac.yaml`](../deploy/rbac.yaml)：Controller 的启动命令、ServiceAccount，以及读写 AIJob/JobSet 所需的最小权限；RBAC 也包含自定义 Scheduler 的权限绑定。
4. [`deploy/kueue-resources.yaml`](../deploy/kueue-resources.yaml)：实验使用的 Topology、ResourceFlavor、ClusterQueue 和 LocalQueue，它们承接 Controller 传下来的队列与跨节点拓扑意图。
5. [`kind.yaml`](../kind.yaml) 与 [`scripts/label-nodes.sh`](../scripts/label-nodes.sh)：创建实验节点，并用 Label 和扩展资源模拟 rack、GPU fabric 与 GPU 容量。
6. [`Makefile`](../Makefile)：把构建、建集群、部署依赖、提交样例和清理串成完整实验流程。最后读它，可以将前面的源码和清单映射到实际执行顺序。

### 6. 沿实验结果反向阅读

1. [`internal/lab/result.go`](../internal/lab/result.go)：先看版本化 JSON 合同，明确一次 measured
   run 必须回答哪些问题。
2. [`internal/lab/calculate.go`](../internal/lab/calculate.go)：核对 free GPU、Unschedulable 和
   target-relative fragmentation 的纯计算。
3. [`internal/lab/cluster.go`](../internal/lab/cluster.go)：查看 AIJob、JobSet、Kueue Workload、
   Job、Pod、Node、Deployment、Event、log 与 metrics 如何被 typed client 发现和等待。
4. [`internal/lab/benchmark.go`](../internal/lab/benchmark.go)：沿“切 profile -> 三个 holder -> capacity
   snapshot -> probe -> partial result -> cleanup/restore”阅读主实验。
5. [`internal/lab/evidence.go`](../internal/lab/evidence.go) 与
   [`internal/lab/exercise.go`](../internal/lab/exercise.go)：理解失败时为何先收证据、后清理。
6. [`cmd/labctl/main.go`](../cmd/labctl/main.go)：最后看 CLI 如何对所有会修改集群的命令应用
   context guard、timeout 和输出目录。

## 按问题回查

| 想确认的问题 | 首先查看 |
| --- | --- |
| AIJob 接受哪些字段，哪些值会被拒绝 | `deploy/crd.yaml` |
| 一个字段如何进入 JobSet 或 Pod | `internal/controller/aijob_controller.go` |
| Controller 会覆盖哪些字段 | `reconcileOwnedFields` 及其测试 |
| `nvlink`、`pcie`、`same-rack` 分别由谁处理 | `schedulingAnnotations`、`internal/topology/constants.go` |
| 自定义调度器怎样找到并启用插件 | `cmd/scheduler/main.go`、`deploy/scheduler-config.yaml` |
| 队列、配额和 rack 拓扑在哪里配置 | `deploy/kueue-resources.yaml` |
| 本地实验如何模拟 GPU 节点 | `kind.yaml`、`scripts/label-nodes.sh` |
| 为什么 baseline 和 optimized 可比较 | `deploy/scheduler-profiles/`、`benchmark_test.go` |
| 失败后证据写到哪里、是否完整 | `evidence.go`、bundle 的 `manifest.json` |
