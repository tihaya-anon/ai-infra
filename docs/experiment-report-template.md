# 调度实验报告模板

## 问题与假设

- 问题：
- baseline expected：
- optimized expected：
- 哪个观察会否定假设：

## 执行环境

- 日期与执行人：
- 完整命令：
- Git commit：
- Kubernetes / kind Node image：
- JobSet / Kueue / Controller / Scheduler image：
- eligible Node 与模拟 GPU capacity：
- timeout 与 repetitions：

## 原始证据

只列出实际生成且 `complete=true` 的文件；不把 expected 值写进这一节。

| Profile | Repetition | Raw JSON | Evidence bundle | Complete |
| --- | ---: | --- | --- | --- |
| baseline | 1 | `out/benchmark/...json` | `out/evidence/.../manifest.json` | |
| optimized | 1 | `out/benchmark/...json` | `out/evidence/.../manifest.json` | |

## Expected 与 Measured

| 指标 | Baseline expected | Baseline measured | Optimized expected | Optimized measured |
| --- | --- | ---: | --- | ---: |
| free GPUs by Node | | | | |
| target-relative fragmentation (`G=4`) | | | | |
| probe outcome | | | | |
| admission wait | | | | |
| scheduling latency | | | | |
| makespan | | | | |
| Unschedulable count | | | | |

## 解释

- 哪些结果支持或否定假设：
- profile 以外是否出现变量变化：
- repetitions 之间是否稳定：
- objects、events、logs、metrics 如何共同解释结果：

## 限制

- 本实验使用模拟扩展资源，没有 Device Plugin/DRA 和真实 GPU；
- 结果不能说明 GPU utilization、训练吞吐、NVLink/NCCL 性能或生产规模行为；
- kind、节点数量、异步控制循环和本机负载可能影响时间指标；
- 仍缺少的 observation 或 incomplete run：

## 后续实验

- 下一条可证伪假设：
- 需要新增的控制变量或证据：
