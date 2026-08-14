# 调度碎片对照实验

这份实验不是为了证明 `MostAllocated` 永远优于 `LeastAllocated`，而是练习怎样提出一个可证伪的
调度假设、控制变量、保留原始证据，再解释结果。这里的 GPU 是 kind Node status 中的
`example.com/gpu` 扩展资源，不代表真实设备、利用率或通信性能。

## 一、先写下假设

集群有三个 eligible Worker Node，每个 Node 有四个模拟 GPU。三个 holder 各请求两个 GPU，并且
保持运行；随后 probe 请求四个 GPU。

- baseline 使用 `NodeResourcesFit/LeastAllocated`，预期 holder 分散为 `[2,2,2]` 空闲量，probe
  因没有单个 Node 能容纳四个 GPU 而 Unschedulable；
- optimized 使用 `MostAllocated`，预期 holder 更集中，可能形成 `[0,2,4]`，从而保留一个完整
  Node 给 probe；
- 两份 profile 的其他 filter、Kueue quota、拓扑插件、节点、工作负载和提交顺序完全相同。

这是 expected behavior。只有运行生成的 JSON 才能写进 measured results。

## 二、准备并核对环境

```bash
make cluster CLUSTER=ai-infra-lab-v134
make deploy CLUSTER=ai-infra-lab-v134
./scripts/label-nodes.sh
kubectl config current-context
kubectl get nodes -l infra.example.io/gpu-node=true \
  -o custom-columns=NAME:.metadata.name,GPU:.status.allocatable.example\.com/gpu
```

context 必须是 `kind-ai-infra-lab-v134`，并且恰好看到三个 GPU Node、每个值为四。runner 在修改
Scheduler ConfigMap 前会重复验证这些条件；不匹配时直接退出。

## 三、运行一次，再决定重复次数

```bash
make benchmark CLUSTER=ai-infra-lab-v134
go run ./cmd/labctl benchmark \
  --cluster ai-infra-lab-v134 \
  --repetitions 3 \
  --timeout 2m \
  --output out/benchmark
make benchmark-validate
```

每个 holder 都会等到 `PodScheduled=True` 后才提交下一个，所以异步调度不会改变提交顺序。两个
profile 串行使用同一个 `ai-scheduler` 名称；runner 等待 Deployment rollout，结束时恢复原配置。
每个 profile 和 repetition 各写一个 JSON。超时文件仍会存在，但 `complete=false` 且 `missing`
解释缺了什么，命令同时返回非零。

## 四、读懂指标，而不是只比较一个数字

对目标请求 `G=4`，碎片率定义为：

```text
usableFree = sum(floor(nodeFree / G) * G)
fragmentation = (totalFree - usableFree) / totalFree
```

当 `totalFree=0` 时比率定义为零。`[2,2,2]` 的 totalFree 是六、usableFree 是零、比率是一；
`[0,2,4]` 的 usableFree 是四、比率是三分之一。这个值只相对于四 GPU probe 有意义。

JSON 还记录：

- Workload 创建到 Admitted condition 的 admission wait；
- Pod 创建/提交到 PodScheduled condition 的 scheduling latency；
- 场景开始到预期 probe condition 的 makespan；
- 观察到 Unschedulable condition 的 Pod 数；
- Pod placement、completion index、outcome、软件版本和 cluster capacity。

如果 optimized 的碎片率更低但 admission wait 更高，报告必须同时呈现，不能只选择支持假设的指标。

## 五、从原始对象解释偏差

先打开 JSON 的 `missing`、`lifecycle`、`placements` 和 `fragmentation.freeByNode`。若结果不符合预期，
按顺序检查：Workload 是否已 Admitted、holder 是否真的逐个绑定、Pod 是否请求
`example.com/gpu`、Scheduler 是否完成 profile rollout、events 中是否有其他 filter 失败。

报告从[模板](experiment-report-template.md)开始，必须列出命令、环境、每份 raw JSON 的相对路径、
限制和 expected/measured 对照。不要提交猜测值，也不要把模拟 GPU 结果描述成硬件性能。
