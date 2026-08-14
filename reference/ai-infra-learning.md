# AI Infra 学习笔记

本文根据当前简历、实习日报与目标岗位 JD 整理，用于记录 AI Infra 学习路线、已有经验迁移方式和实践项目。

## 目标岗位

目标岗位为阿里巴巴 2027 届 AI Infra 工程师（容器方向），原始岗位描述见 [jd.txt](jd.txt)。岗位能力可以归纳为四个方向：

1. GPU 容器调度与编排；
2. 高性能存储与网络；
3. AI 工程平台与资源治理；
4. 容器运行时、可观测性及性能优化。

## 当前能力基线

现有经历更接近云原生数据基础设施工程。它不是 AI Infra 的终点，但已经覆盖了控制面、任务运行时、分布式处理和工程可靠性等基础能力。

| 已有经验 | AI Infra 中的对应能力 |
| --- | --- |
| Dagster Control Plane、Code Location、Worker 拆分 | 控制面与执行面解耦 |
| ECS Fargate、run worker、pool limits | 容器任务生命周期、资源配额与并发治理 |
| Terraform、IAM、VPC、Security Group | 集群基础设施、权限与网络隔离 |
| ECR 缓存、Docker 分层、EFS 代码发布 | 镜像和模型分发、冷启动优化 |
| Kafka、Flink、Spark、Glue | 分布式任务、并行计算和失败恢复 |
| Backfill、重试、双跑与蓝绿发布 | 作业容错、重执行与渐进式发布 |
| Prometheus、Grafana、SLO、负载测试 | 可观测性、容量评估与可靠性治理 |
| Go、Java、Python | Controller、Scheduler 和平台服务开发基础 |

仓库中可直接核对的背景材料是[中文简历](resume_zh.json)与[目标岗位描述](jd.txt)。学习路线不再
链接未纳入仓库的个人笔记，避免读者在实践中进入断链。

## 主要知识缺口

### Linux 与容器底层

- Linux 进程、文件系统和网络栈；
- namespace、cgroup v2、capability、seccomp；
- OCI 镜像与运行时规范；
- containerd、CRI、runc 和 Kubernetes 之间的调用链。

### Kubernetes 内部机制

- API Server、etcd、kubelet 和控制器的职责；
- Informer、本地缓存、Workqueue 与 reconciliation loop；
- Scheduler Framework 的 QueueSort、Filter、Score、Reserve、Permit、Bind 等阶段；
- CRD、Controller、Operator、Device Plugin 和 Scheduler Plugin 开发。

### GPU 与分布式 AI 系统

- GPU、显存、CUDA Driver 与 CUDA Runtime；
- PCIe、NVLink、NUMA、RDMA/RoCE 等拓扑和通信路径；
- NCCL collective communication；
- Data、Tensor、Pipeline Parallelism；
- Checkpoint、模型加载、KV Cache 和批处理策略。

### 性能工程

- 延迟、吞吐、利用率和扩展效率的定义；
- CPU、内存、磁盘、网络和 GPU 瓶颈的区分；
- perf、eBPF、FlameGraph 等工具；
- 以基准、对照实验和火焰图验证优化效果。

## AI Infra 系统地图

AI Infra 的目标，是把 GPU、网络和存储组织为可调度、可靠且利用率足够高的计算平台。

```text
训练 / 推理 / Agent
        |
作业系统：队列、配额、弹性、容错
        |
Kubernetes：Controller、Scheduler、Runtime
        |
通信与存储：NCCL、RDMA、Checkpoint、模型缓存
        |
硬件：GPU、显存、NVLink、PCIe、NUMA
```

这与当前数据平台的基本结构相似：Dagster Job 对应 AI 作业，Control Plane 对应 Kubernetes 控制面，ECS Worker 对应 Pod，而 Glue 并发限制和 Dagster Pool 对应队列及资源配额。区别在于 GPU 数量少、成本高，并且具有显存容量、互联拓扑和协同调度约束。

## 第一课：GPU 调度

### Kubernetes 如何表示 GPU

安装 GPU Device Plugin 后，节点会向 Kubernetes 上报扩展资源。Pod 可以请求 GPU：

```yaml
resources:
  limits:
    nvidia.com/gpu: 8
```

默认调度器能判断某个节点是否还有八张可分配 GPU，但仅靠资源数量不能解决所有 AI 调度问题。

### 资源碎片

假设两个节点分别拥有八张 GPU：

```text
Node A: 空闲 4 / 8
Node B: 空闲 4 / 8
集群总空闲: 8
```

此时，单机八卡任务仍然无法运行。调度策略如果长期将小任务分散到每台机器，就会产生足够的总空闲资源，却没有足够大的连续资源块。

解决资源碎片需要结合任务画像选择策略：

- 对小任务进行 bin packing，尽量填满部分节点，保留完整节点；
- 为大任务预留资源或设置独立队列；
- 在必要时使用抢占、迁移或时间切片；
- 同时评估利用率、公平性、等待时间和抢占成本。

### Gang Scheduling

一个训练任务可能包含四个 Worker，每个 Worker 需要八张 GPU。如果只启动三个 Worker，已占用的 24 张 GPU 会等待最后一个 Worker，不能产生有效训练进度。

Gang Scheduling 要求一组 Pod 同时获得运行条件：

> 一组 Pod 要么一起被调度，要么全部继续等待。

它通常需要解决三个问题：

1. 如何定义属于同一个作业的 Pod 集合；
2. 如何确认整个集合满足最低可运行资源；
3. 资源预留失败时如何回滚，避免部分资源长期占用。

这可以类比 Dagster 作业的并发限制，但 GPU 场景还必须处理跨节点资源预留和分布式 Worker 的共同生命周期。

### 拓扑感知

GPU 之间的通信成本并不相同。一个简化的距离顺序是：

```text
同一 NVLink 域
  < 同一 PCIe Switch
  < 同一 NUMA 节点
  < 同一机器
  < 同一机架的 RDMA 网络
  < 普通跨机网络
```

调度器因此不能只问“哪里还有 GPU”，还应询问：

- GPU 是否属于同一高速互联域；
- GPU 与 CPU、网卡是否处于合适的 NUMA 节点；
- 多机训练节点之间是否具有 RDMA/RoCE 网络；
- 当前放置是否会破坏后续大任务所需的完整拓扑。

### 一次调度决策应考虑什么

```text
作业进入队列
  -> 检查租户配额和优先级
  -> 判断是否满足 Gang 最小资源
  -> Filter 不满足硬约束的节点
  -> Score GPU 拓扑、碎片和数据位置
  -> Reserve 整组资源
  -> Bind Pods
  -> 监控启动结果并在失败时回滚
```

硬约束适合放在 Filter，例如 GPU 数量不足；软目标适合放在 Score，例如尽量减少碎片。把软目标写成硬约束会降低可调度性，把硬约束写成打分则可能生成无法工作的放置方案。

## 三类 AI 工作负载

### 训练

训练任务通常运行时间长，需要多卡或多机协同，并周期性保存 Checkpoint。任一 Worker 故障都可能影响整个作业。

主要指标：

- samples/tokens per second；
- GPU 利用率；
- 多卡扩展效率；
- Checkpoint 保存及恢复时间；
- 失败后损失的有效训练时间。

### 推理

LLM 推理包含两个重要阶段：

- Prefill：处理输入 Token，通常更偏计算密集；
- Decode：逐 Token 生成结果，容易受到显存带宽和 KV Cache 的影响。

常见优化包括 Continuous Batching、KV Cache 管理、模型并行、请求路由和副本弹性。关键指标包括：

- TTFT：首个 Token 延迟；
- TPOT：后续每个 Token 的生成时间；
- P99 请求延迟；
- tokens per second；
- 单位 GPU 成本下的吞吐量。

### Agent

Agent 除模型请求外，还可能临时启动代码沙箱、工具容器或隔离执行环境。其冷启动链路可以拆为：

```text
创建 Pod
  -> 调度到节点
  -> 拉取镜像
  -> 创建容器和挂载存储
  -> 下载或挂载模型
  -> 初始化 GPU
  -> 服务 Ready
```

现有的 Docker 分层、ECR pull-through cache、EFS 发布和蓝绿切换经验可以迁移到这条链路。但大模型权重可能远大于应用镜像，还需要节点级模型缓存、并行下载、按需加载和预热机制。

## 学习路线

### 阶段一：容器和 Kubernetes 原理

目标：能够解释一个 Pod 从提交到进程运行的完整路径。

- 手工使用 namespace 和 cgroup 隔离进程；
- 理解 OCI bundle，并使用 runc 启动容器；
- 跟踪 API Server、Scheduler、kubelet、CRI、containerd、runc 的调用链；
- 编写一个简单 CRD 和 Go Controller。

验收问题：

1. Docker、containerd 与 runc 各负责什么？
2. Controller 为什么采用期望状态与实际状态的循环调谐？
3. Scheduler 完成 Bind 后，容器为什么还不一定已经启动？

### 阶段二：GPU 容器和调度

目标：能够解释默认 Kubernetes 调度器为什么不足以支撑多机多卡训练。

- 理解 NVIDIA Driver、CUDA、Container Toolkit、Device Plugin；
- 学习 Extended Resource 和 Scheduler Framework；
- 实现 GPU 数量、拓扑和碎片感知的 Filter/Score；
- 理解 Gang Scheduling、Queue、Quota 和 Preemption。

验收问题：

1. 容器为什么可以使用宿主机上的 GPU Driver？
2. 八张空闲 GPU 为什么不等于一个八卡任务可运行？
3. Gang 资源预留失败后需要回滚哪些状态？

### 阶段三：训练、推理和存储网络

目标：能够根据工作负载选择调度、存储和网络方案。

- 理解 Data、Tensor、Pipeline Parallelism；
- 理解 NCCL collective 和通信拓扑；
- 分析 Checkpoint 写入及恢复链路；
- 理解 Prefill、Decode、KV Cache 和 Continuous Batching；
- 比较本地盘、共享文件系统与对象存储的职责。

### 阶段四：性能分析和生产治理

目标：能够用数据证明瓶颈位置和优化收益。

- 建立端到端延迟和资源利用率指标；
- 使用 perf、eBPF 和火焰图定位系统开销；
- 区分 CPU、GPU、网络、存储与调度等待；
- 建立容量基线、SLO、错误预算和故障 runbook。

## 实践项目：AIJob Operator

使用 Go 开发一个 Kubernetes `AIJob` Operator，将现有平台工程经验迁移到 AI 场景。

### 当前 API

```yaml
apiVersion: infra.example.io/v1alpha1
kind: AIJob
metadata:
  name: demo-training
spec:
  workers: 4
  gpuPerWorker: 2
  topology: same-rack
  image: ai-infra-lab:dev
  args: ["--mode=complete", "--duration=5s"]
```

### 迭代计划

已完成的教学闭环是：AIJob 到 JobSet 的薄转换、Kueue queue/quota/TAS、Node 级拓扑打分、
确定性 Worker、组件指标、unit/envtest/kind 三层验证、碎片对照实验和 run-scoped failure evidence。

下一阶段应在不混淆边界的前提下继续：Checkpoint 恢复、镜像预拉取、模型缓存预热、真实 DRA
设备选择，以及真实 GPU 上的训练/通信 benchmark。它们必须有各自的 measured evidence，不能从
本仓库的模拟扩展资源结果外推。

### 当前项目的独立验收路径

不要只运行最终 demo。每一层回答的问题不同，也可以独立重跑：

| 层次 | 命令 | 验收问题 |
| --- | --- | --- |
| 纯逻辑 | `make verify` | 参数、转换、指标标签和碎片公式是否稳定 |
| API 控制循环 | `make test-api` | 真实 API Server 下 Reconcile 是否幂等、status 是否正确 |
| 部署链路 | `make test-e2e CLUSTER=ai-infra-lab-v134` | JobSet/Kueue/Scheduler/Worker/GC 是否接力 |
| 调度实验 | `make benchmark CLUSTER=ai-infra-lab-v134` | profile 是否改变可观察的碎片与 probe outcome |
| 故障诊断 | `make failure-worker CLUSTER=ai-infra-lab-v134` 等 | 能否从对象、事件、日志和指标定位阶段 |

完成一层的标准不是“命令跑过”，而是能解释输入、预期 condition、原始证据和不属于该层的结论。

没有真实 GPU 时，可以通过 Node Label 和测试用扩展资源模拟节点拓扑。项目重点是控制器、调度决策、资源状态和可靠性，而不是训练模型本身。

## 第一阶段自测

完成第一轮学习后，应当能够独立回答：

1. 一个 Pod 从提交 YAML 到容器进程启动，经过了哪些组件？
2. Kubernetes Controller 与 Dagster Control Plane 有哪些相似和不同？
3. 为什么 GPU 调度需要同时考虑数量、显存、拓扑和作业集合？
4. Bin packing 为什么能缓解碎片，又会引入哪些可靠性风险？
5. Gang Scheduling 如何避免部分 Worker 空占 GPU？
6. 模型服务冷启动中，镜像、模型权重和 GPU 初始化分别如何优化？

后续学习笔记应继续沿用“概念、已有经验映射、系统机制、实践任务、验收问题”的结构，避免只积累名词而不形成可验证的工程能力。
