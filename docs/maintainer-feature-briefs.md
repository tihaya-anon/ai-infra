# 维护者功能需求练习

这些需求假设你刚接手这个代码库，需要在现有教学项目上继续演进。每个需求只描述要达到的结果，不指定实现路径；实现前应先阅读源码、部署清单和测试，自己判断改动落点。

建议一次只实现一个需求。完成前至少能回答三个问题：用户如何声明这个能力，Controller 或 Scheduler 如何消费它，失败时维护者从哪里看到原因。

## 需求 1：为 `AIJob` 增加失败策略

- 背景：当前 `AIJob` 能表达 worker 数量、GPU 请求、拓扑偏好、镜像和参数，但失败行为基本依赖下游 JobSet/Job 默认语义。
- 声明：用户可以在 `AIJob` 中声明训练任务失败后的处理策略，至少覆盖失败后停止整个 AIJob 和允许有限次数重试两类语义。
- 范围：API 类型、CRD、Controller、测试、示例 YAML。
- 实现：`spec.jobSetOverrides.failurePolicy` 透传 JobSet，示例覆盖失败即停和有限重试。

## 需求 2：支持暂停和恢复 `AIJob`

- 背景：生产集群经常需要临时释放资源，或在队列中暂停某些训练任务。现在的 `AIJob` 一旦提交，主要生命周期由下游对象推进。
- 声明：用户可以暂停一个 `AIJob`，并在取消暂停后让它使用原有 spec 重新进入调度流程。
- 范围：API、Controller reconcile、status 投影、Kueue/JobSet 交互、envtest。
- 实现：`spec.jobSetOverrides.suspend` 可反复更新，Controller 保留原 spec 并投影 JobSet 状态。

## 需求 3：让拓扑策略支持硬约束

- 背景：当前拓扑偏好更偏教学演示：调度器插件对节点打分，但不一定阻止不满足偏好的节点被选中。生产上有些任务需要硬性约束。
- 声明：用户可以区分“偏好某种拓扑”和“必须满足某种拓扑”，软策略继续影响排序，硬策略在条件不满足时阻止调度。
- 范围：API、Controller annotation/label 协议、Scheduler Framework plugin、拓扑常量、调度实验。
- 实现：拓扑策略区分 preferred/required，硬约束由 Kueue topology request 和 Scheduler filter 执行。

## 需求 4：把 Kueue 队列从固定配置改成用户可选

- 背景：示例中 `LocalQueue` 和 `ClusterQueue` 是固定的。生产环境通常会按团队、优先级或资源池配置多个队列。
- 声明：用户可以选择目标队列；没有显式设置时使用默认队列，非法或不存在的队列不能被静默当作成功。
- 范围：API 默认值、Controller、Kueue resources、deploy YAML、docs。
- 实现：`spec.queueName` 默认选择 `training`，可选择生成清单定义的多个 LocalQueue；队列不存在时 status 明确报错并重试。

## 需求 5：增加 worker 运行时资源上报

- 背景：当前 worker 主要用于合成 workload 和生命周期验证。维护者需要让实验结果能看到每个 worker 的关键运行数据。
- 声明：worker 输出结构化运行记录，实验工具能够收集成功和失败 worker 的启动时间、完成时间、退出原因和参数摘要等信息。
- 范围：Worker、Controller 传参、lab evidence、benchmark/failure exercise、测试。
- 实现：Worker NDJSON 包含参数摘要、启动/完成时间、退出原因和退出码；benchmark 结果与 evidence bundle 汇总每个 Pod 的运行报告并保留原始日志。

## 需求 6：给调度实验增加“资源碎片恢复”场景

- 背景：现有 benchmark 可以比较不同 scheduler profile 下的碎片情况，但还没有明确验证释放占位任务后，新任务是否能恢复调度。
- 声明：实验先制造资源碎片和 Unschedulable AIJob，再释放部分 holder workload，并记录目标 AIJob 是否恢复调度。
- 范围：`labctl`、`internal/lab`、结果 schema、证据收集、文档。
- 实现：baseline 确认 probe Unschedulable 后删除一个 holder，等待同一 probe 恢复并完成；v2 结果记录释放、恢复时间和延迟，benchmark 自动收集证据。

## 需求 7：为控制面增加最小生产观测面

- 背景：生产维护者需要快速判断 Controller 和 Scheduler 是否健康，以及最近的调度/协调失败集中在哪里。
- 声明：Controller 和 Scheduler plugin 暴露有边界的基础 metrics，部署清单暴露 metrics 端口，文档说明如何在实验环境中查看。
- 范围：Controller metrics、Scheduler plugin metrics、deploy manifests、docs、测试。
- 实现：Controller reconcile/错误/JobSet 变更指标和 Scheduler filter/score 指标已注册，并通过两个 metrics Service 暴露。

## 需求 8：完善生成式部署清单流程

- 背景：项目已经用 Go 类型和 kubebuilder marker 生成 `AIJob` CRD，`make generate` 也会更新 deepcopy 和 `deploy/crd.yaml`。但 RBAC、controller/scheduler Deployment 和依赖资源仍主要依赖静态 YAML。
- 声明：继续扩大生成流程覆盖范围，让 RBAC 也进入生成链路，并明确哪些 deploy 清单是生成物、哪些是教学用静态入口。
- 范围：Kubebuilder markers、Makefile、deploy/config 布局、docs、CI/verify 流程。
- 实现：Controller 权限由 kubebuilder RBAC marker 生成到 `deploy/rbac.yaml`；身份和绑定保留在静态 `deploy/access.yaml`，`make verify` 检查生成物是否过期。

## 使用方式

不要为了完成练习提前大规模重构。只有当某个需求让复杂度在多个地方重复出现时，再抽出更深的模块。
