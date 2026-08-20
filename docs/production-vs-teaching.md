# 从教学实验到生产控制面

本文说明这个项目从教学实验走向生产系统时，哪些设计可以保留，哪些地方需要重做，以及生产代码更合适的模块组织方式。

## 结论

教学版已经展示了正确的数据流：

```text
AIJob
  -> Controller
  -> JobSet / Kueue
  -> Scheduler Framework plugin
  -> Worker Pods
```

生产化时，这条主线不需要推翻。主要工作集中在三个深模块上：

1. `AIJob` API 的长期兼容契约。
2. Controller 的幂等 reconciliation 和状态投影。
3. Scheduler plugin 背后的资源、拓扑和打分模型。

但是，生产化不是只把 controller 和 scheduler 写复杂。教学代码里的部署、测试、观测、升级、安全、资源发现和故障恢复都刻意简化了；这些会成为生产系统的大部分工程量。

## 可以保留的形状

### 复用 Kubernetes 原生扩展点

生产系统仍应优先使用 Kubernetes 已有扩展点：

- 用 CRD 表达业务意图。
- 用 controller-runtime 写 Controller。
- 用 JobSet 承载 gang-style 多 worker workload。
- 用 Kueue 做队列、准入、配额和拓扑感知资源管理。
- 用 Scheduler Framework plugin 扩展打分逻辑。

除非有明确证据证明框架能力不足，否则不要 fork kube-scheduler，也不要自己实现队列系统。

### 保持 Controller 薄

Controller 的职责应该是把 `AIJob` 翻译成下游 Kubernetes 对象，并把下游状态投影回 `AIJob.status`。它不应该内置复杂的 GPU 拓扑算法、集群容量规划或供应商设备细节。

合适的 Controller 接口是：

```text
AIJob + observed cluster state -> desired JobSet + status patch
```

调用方只需要理解这个接口。复杂的模板构造、字段所有权、status condition 合并和事件记录都藏在实现里。

### 保持 Scheduler plugin 窄入口

Scheduler plugin 的入口应继续对齐 Scheduler Framework：`PreScore` / `Score` / `NormalizeScore` 这类方法是外部接口。生产复杂度不应该泄露成大量全局配置和散落的 label 解析，而应收拢到内部拓扑模型。

## 教学版简化了什么

| 领域 | 教学版 | 生产版要求 |
| --- | --- | --- |
| API 版本 | 单个 `v1alpha1`，字段少 | 多版本兼容、conversion、defaulting、validation webhook、废弃策略 |
| 清单生成 | Go 类型/marker 生成 CRD 与 RBAC，Go manifest 包生成 Kueue 实例；运行身份和 Deployment 保持静态 | `controller-gen` 生成 CRD/RBAC/webhook，Helm/Kustomize 组装环境差异并 review 生成 diff |
| Controller | 单一 happy path，少量 status | 幂等、冲突重试、server-side apply、finalizer、事件、条件语义稳定 |
| Scheduler | 一个小型插件用 `PreFilter`/`Filter`/`PreScore`/`Score` 演示节点偏好 | 缓存、性能预算、可解释打分、真实设备拓扑模型 |
| 资源发现 | kind label 和模拟 Device Plugin | DRA、device plugin、NodeResourceTopology、厂商 API 或 inventory adapter |
| Kueue | 两个业务队列、一个实验队列和简单 cohort | 多租户隔离、动态配额、准入策略、拓扑策略升级 |
| 镜像构建 | BuildKit 缓存、去符号静态二进制、distroless nonroot | 多架构、SBOM、provenance、签名、漏洞扫描、按 digest 发布 |
| 部署 | 静态 YAML | Helm/Kustomize、RBAC 收敛、Pod 安全、HA、rollout/rollback |
| 观测 | 基础 metrics 和实验证据 | SLO、告警、审计日志、trace、debug endpoint、调度决策解释 |
| 测试 | 单元、envtest、kind e2e | 升级测试、兼容测试、性能测试、故障注入、真实 GPU staging |
| 实验工具 | `internal/lab` 与 benchmark | 与生产控制面隔离，作为验证工具或独立包发布 |

## 推荐模块结构

生产代码应围绕少数深模块组织，而不是把每个 Kubernetes 对象或每个小函数都做成一层。

```text
api/
  v1alpha1/
  v1beta1/
cmd/
  controller/
  scheduler/
config/
  crd/
  rbac/
  manager/
  scheduler/
internal/
  controller/
  scheduler/
  topology/
  resources/
  adapters/
    jobset/
    kueue/
    dra/
    deviceplugin/
  observability/
  worker/
  lab/
```

### `api/`

`api/` 是最外层接口，生产上要最保守。字段一旦进入已发布版本，就要假设有外部 YAML、CI、UI 和自动化依赖它。

生产版至少需要：

- Kubebuilder markers 生成 CRD schema。
- defaulting webhook 处理默认镜像、默认队列、默认拓扑策略。
- validation webhook 表达跨字段约束。
- conversion webhook 支持版本迁移。
- 明确 status condition 类型、reason 和 message 稳定性。

### `internal/controller/`

Controller 是一个深模块：外部接口是少量 watched resources 和 `Reconcile` 行为，内部实现负责对象翻译、字段所有权和状态合并。

生产版应拆出这些内部模块：

- `desired`: 从 `AIJob` 生成 JobSet/PodTemplate。
- `ownership`: owner reference、managed fields、server-side apply field manager。
- `status`: JobSet/Kueue/Pod 状态到 AIJob conditions 的投影。
- `events`: 用户可读的事件和失败原因。

这些可以是内部 seam，但不要暴露给 `cmd/controller`。

### `internal/scheduler/`

Scheduler plugin 要保持 Kubernetes Framework 接口，复杂度放到内部模型：

- `topology.Snapshot`: 一次调度周期看到的节点、GPU、rack、节点拓扑等级。
- `resources.Request`: 从 Pod/Workload 解析出的资源和拓扑需求。
- `scoring.Model`: 可测试、可解释、与 Kubernetes framework 解耦的打分逻辑。

这样 plugin 只做适配：

```text
framework.NodeInfo + Pod
  -> resources.Request + topology.Snapshot
  -> scoring.Model.Score
  -> framework score
```

这能让核心算法用普通 Go 单元测试覆盖，不必每个边界都启动 scheduler。

### `internal/adapters/`

生产环境的外部依赖会变化，应该通过 adapter 收敛：

- Kueue adapter：读取 Workload、Admission、ClusterQueue 状态。
- JobSet adapter：构造和观察 JobSet。
- DRA adapter：读取 ResourceClaim、ResourceSlice 或厂商驱动暴露的信息。
- Inventory adapter：接入资产系统、节点标签、GPU 设备拓扑数据源。

一个 adapter 的接口要小，例如“给定节点名返回 GPU 拓扑事实”，不要把整个 Kubernetes client 暴露给算法层。

### `internal/lab/`

教学和实验代码应留在 `internal/lab` 或独立仓库。它可以继续调用生产模块，但生产模块不能依赖实验 runner、benchmark 输出格式或 kind 集群假设。

## 生产化工作重点

### 1. API 先稳定

先把 `AIJobSpec` 和 `AIJobStatus` 当成产品接口设计，而不是 controller 的内部参数。

需要回答：

- 用户表达的是“训练任务”还是“GPU gang workload”？
- `topology` 是偏好、硬约束，还是策略名？
- 队列、优先级、重试、失败策略、suspend/resume 是否属于 `AIJob`？
- status 中哪些 condition 是用户可依赖的自动化信号？

这些问题一旦晚于 controller/scheduler 实现再回答，会导致 API 反复破坏兼容。

### 2. 拓扑模型独立出来

不要让 scheduler plugin 到处直接读 label 和 annotation。生产中 GPU 拓扑事实可能来自 Node labels、DRA、device plugin、厂商 API 或 CMDB。算法层应该看到统一的拓扑模型，而不是看到数据源细节。

这个模型是生产系统最值得做深的模块。

### 3. Controller 和 Scheduler 分别测试

Controller 测试重点：

- 输入 `AIJob` 后生成的 JobSet 是否稳定。
- 已存在 JobSet 被外部 webhook 改过时是否只维护自己拥有字段。
- JobSet/Kueue/Pod 失败时 status condition 是否准确。
- 删除、重建、controller restart 是否幂等。

Scheduler 测试重点：

- 每种拓扑需求的节点分数边界。
- 资源不足、部分可用、数据缺失时的降级行为。
- 打分性能和缓存一致性。
- Normalize 后是否保持预期排序。

### 4. 部署和升级单独设计

生产控制面至少需要：

- controller 和 scheduler 独立镜像或明确的多二进制镜像策略。
- leader election 和 HA deployment。
- RBAC 最小化。
- CRD 升级顺序和 rollback 策略。
- webhook 证书和可用性策略。
- metrics、alerts 和 structured logs。

这些不应该塞进业务代码；它们属于部署和运行模块。

## 迁移顺序

建议按这个顺序推进：

1. 继续扩大 `controller-gen` 覆盖范围，补齐 webhook 配置和证书生命周期。
2. 把静态 deploy YAML 迁到 `config/`，用 Kustomize 或 Helm 组织环境差异。
3. 固化 `AIJob` API 语义，补 defaulting/validation webhook。
4. 从 scheduler plugin 中抽出 `topology`、`resources`、`scoring` 三个深模块。
5. 把 Kueue、JobSet、DRA/设备信息接入做成 adapter。
6. 扩展 envtest 和 kind e2e，加入 controller restart、资源不足、升级兼容测试。
7. 将 `internal/lab` 明确标记为实验验证工具，避免生产路径依赖它。

这条路径保留教学项目的主线，同时把生产复杂度放到稳定 seam 后面。
