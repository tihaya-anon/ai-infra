# 维护者功能需求练习

这些需求假设你刚接手这个代码库，需要在现有教学项目上继续演进。每个需求只描述目标和验收标准，不给出具体实现路径；实现前应先阅读源码、部署清单和测试，自己判断改动落点。

## 需求 1：为 AIJob 增加失败策略

### 背景

当前 `AIJob` 能表达 worker 数量、GPU 请求、拓扑偏好、镜像和参数，但失败行为基本依赖下游 JobSet/Job 默认语义。维护者需要让用户能声明训练任务失败后的处理策略。

### 目标

在 `AIJobSpec` 中增加一个失败策略字段，支持至少两种行为：

- 任务失败后立即停止整个 AIJob。
- 任务失败后允许有限次数重试。

### 验收标准

- CRD schema 能校验非法策略。
- Controller 能把策略传递到下游 workload。
- `AIJob.status.conditions` 能反映最终失败原因。
- 已有无该字段的 YAML 仍能正常运行。

### 覆盖范围

API 类型、CRD、Controller、测试、示例 YAML。

## 需求 2：支持暂停和恢复 AIJob

### 背景

生产集群经常需要临时释放资源，或在队列中暂停某些训练任务。现在的 `AIJob` 一旦提交，主要生命周期由下游对象推进。

### 目标

允许用户通过 `AIJobSpec` 暂停任务，并在取消暂停后恢复调度。

### 验收标准

- 暂停后不应继续创建新的 worker Pod。
- 恢复后应继续使用原有 spec 重新进入调度流程。
- status 能区分 `Paused`、`Admitted`、`Running`、`Completed`、`Failed` 中至少三个关键阶段。
- Controller 重启后行为仍然幂等。

### 覆盖范围

API、Controller reconcile、status 投影、Kueue/JobSet 交互、envtest。

## 需求 3：让拓扑策略支持硬约束

### 背景

当前拓扑偏好更偏教学演示：调度器插件对节点打分，但不一定阻止不满足偏好的节点被选中。生产上有些任务需要硬性约束，例如必须在同 rack 或必须使用某类 GPU fabric。

### 目标

扩展拓扑策略，使用户能声明“偏好”或“必须满足”的拓扑要求。

### 验收标准

- 软策略仍按分数影响节点排序。
- 硬策略在条件不满足时应阻止调度。
- 调度失败时能从事件、status 或实验结果中看出原因。
- baseline 和 optimized scheduler profile 仍能用于对照实验。

### 覆盖范围

API、Controller annotation/label 协议、Scheduler Framework plugin、拓扑常量、调度实验。

## 需求 4：把 Kueue 队列从固定配置改成用户可选

### 背景

示例中 `LocalQueue` 和 `ClusterQueue` 是固定的。生产环境通常会按团队、优先级或资源池配置多个队列。

### 目标

允许 `AIJob` 选择目标队列；没有显式设置时使用默认队列。

### 验收标准

- 示例 YAML 能展示默认队列和显式队列两种用法。
- Controller 生成的下游对象包含正确队列信息。
- 非法或不存在的队列不会被静默当作成功。
- 文档说明队列配置和 AIJob 字段之间的关系。

### 覆盖范围

API 默认值、Controller、Kueue resources、deploy YAML、docs。

## 需求 5：增加 worker 运行时资源上报

### 背景

当前 worker 主要用于合成 workload 和生命周期验证。维护者需要让实验结果能看到每个 worker 的关键运行数据，例如启动时间、完成时间、退出原因和参数摘要。

### 目标

让 worker 输出结构化运行记录，并让实验工具能收集这些记录。

### 验收标准

- worker 日志仍保持机器可解析。
- `labctl` 收集证据时能包含每个 worker 的运行记录。
- 失败 worker 和成功 worker 都有记录。
- 不依赖真实 GPU 才能运行。

### 覆盖范围

Worker、Controller 传参、lab evidence、benchmark/failure exercise、测试。

## 需求 6：给调度实验增加“资源碎片恢复”场景

### 背景

现有 benchmark 可以比较不同 scheduler profile 下的碎片情况，但还没有明确验证“释放占位任务后，新任务是否能恢复调度”的场景。

### 目标

增加一个实验：先制造资源碎片和 Unschedulable AIJob，再释放部分 holder workload，观察目标 AIJob 是否完成。

### 验收标准

- 实验结果写入版本化 JSON。
- 结果中能区分首次不可调度和释放后成功调度。
- 失败时 evidence bundle 足够定位 holder、target、Pod、Event 和 Node 状态。
- 不影响现有 benchmark 输出格式的兼容性。

### 覆盖范围

`labctl`、`internal/lab`、结果 schema、证据收集、文档。

## 需求 7：为控制面增加最小生产观测面

### 背景

生产维护者需要快速判断 Controller 和 Scheduler 是否健康，以及最近的调度/协调失败集中在哪里。

### 目标

增加最小观测能力，不要求完整 SLO 平台，但要能支持本地和 kind 环境排查。

### 验收标准

- Controller 暴露 reconcile 成功、失败、重试或耗时指标。
- Scheduler plugin 暴露打分次数、跳过原因或异常输入计数。
- 部署清单暴露 metrics 端口。
- 文档说明如何在实验环境中查看这些指标。

### 覆盖范围

Controller metrics、Scheduler plugin metrics、deploy manifests、docs、测试。

## 需求 8：完善生成式部署清单流程

### 背景

项目已经用 Go 类型和 kubebuilder marker 生成 `AIJob` CRD，`make generate` 也会更新
deepcopy 和 `deploy/crd.yaml`。但 RBAC、controller/scheduler Deployment 和依赖资源仍主要依赖静态 YAML。
随着权限、webhook 或部署参数增加，手写清单仍容易漂移。

### 目标

继续扩大生成流程覆盖范围：用代码标注生成 RBAC，明确哪些 deploy 清单是生成物、哪些是教学用静态入口，
并保留适合阅读的部署路径。

### 验收标准

- `make generate` 继续生成 deepcopy 和 CRD，并新增 RBAC 生成。
- 生成后的清单能被当前部署流程使用。
- 手写 deploy 文件和生成产物之间没有重复维护的权限字段。
- 文档说明修改 API 或 RBAC marker 后需要运行的命令。

### 覆盖范围

Kubebuilder markers、Makefile、deploy/config 布局、docs、CI/verify 流程。

## 使用方式

建议一次只实现一个需求。每个需求都应先写或更新能失败的测试，再阅读相关源码确认改动位置。不要为了完成练习提前大规模重构；只有当某个需求让复杂度在多个地方重复出现时，再抽出更深的模块。
