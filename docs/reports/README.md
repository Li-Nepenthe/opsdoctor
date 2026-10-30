# docs/reports：阶段汇报书

每个阶段完成时写一份，记录这个阶段交付了什么、改了哪些文件、怎么测试和验收的、还有哪些已知问题。

汇报书是给验收看的记录和证据，信息很全但很密。**学习请读 [learn](../learn/README.md) 里的学习材料**，需要查某个细节（比如某个函数在哪一行、某项验收的原始数据）时再来这里。

| 阶段 | 汇报书 |
|---|---|
| 阶段 0：地基 | [phase-00-foundation](phase-00-foundation.md) |
| 阶段 1：库存服务 | [phase-01-inventory](phase-01-inventory.md) |
| 阶段 2：订单服务 | [phase-02-order](phase-02-order.md) |
| 阶段 3：可观测性 | [phase-03-observability](phase-03-observability.md) |
| 阶段 4：故障实验 | [phase-04-chaos](phase-04-chaos.md) |
| 阶段 5：医生工具 | [phase-05-doctor-tools](phase-05-doctor-tools.md) |
| 阶段 6：Agent 与实时页面 | [phase-06-agent](phase-06-agent.md) |
| 阶段 7：审批、恢复与安全 | [phase-07-reliability](phase-07-reliability.md)（[代码清单](phase-07-code-inventory.md)） |

注意：汇报书里写的行号是写报告时的版本，后来代码改过，行号可能对不上。找当前代码请用[代码地图](../代码地图.md)。

阶段 8 进行中：先看 [手册与试跑审核材料](phase-08-pilot.md) 和 [数据库导出结果](phase-08-pilot-results.md)。这是当时第一个停点的材料；S08基线修正已获用户批准并完成复验，当前继续正式矩阵。

第一个停点已获同意。后续的[开跑前检查](phase-08-preflight.md)记录网络阻塞及重试、计时修复；原始[网络采样](phase-08-network-preflight.jsonl)和[时钟采样](phase-08-clock-preflight.json)只含状态与时间，不含凭据。这些是正式开跑前的历史记录。

第一次校时后的[六十次稳定性复核](phase-08-clock-stability.jsonl)仍有跳变，失败保留。后续[校时复验](phase-08-clock-after-sync.json)及[完整十分钟网络复验](phase-08-network-after-sync.jsonl)通过。配对实现的[结果页预览](phase-08-results-preview.png)使用原试跑记录，尚不是正式成绩。

五配置最后检查已通过，见 [lastcheck5 完整结果](phase-08-lastcheck5-results.md)。[阶段 8 汇报书草稿](phase-08-eval.md)和[代码清单](phase-08-code-inventory.md)持续更新。正式矩阵与人工一致率尚未完成，这些文件不表示阶段验收通过。

正式批次已启动：[冻结版本和随机顺序](phase-08-formal-manifest.json)、[已保存结果的当前导出](phase-08-formal-results.md)、[逐项失败分析](phase-08-failure-analysis.md)。当前导出只包含已经执行的槽位，不能把尚未运行的部分当成成功或零费用。

正式页面的[结果表截图](phase-08-formal-preview.jpg)和[评分失败展开截图](phase-08-formal-grade-failure.jpg)取自首二十份诊断。评分失败显示为缺失值，原始错误与已发生费用仍然可见。

正式运行在 S02 第二轮遇到约 918ms 的日期跳变，见[暂停与费用记录](phase-08-formal-clock-interruption.json)。暂停时已保存二十四份报告，被取消的最后一条诊断费用在暂停记录和台账单列。当时本机与运行医生切回 Mock、压测流量暂停。用户处理后已恢复原批次，原取消费用也已计入后续导出，指定补跑另占一行。

用户随后处理校时来源并批准分级守卫，见[新规则采样](phase-08-clock-policy-v3.json)与 decisions。原暂停记录是当时的事实，未改写；续跑版本和指定中断的补跑许可另外保存。

- 校时修订免费复验：[网络120次](phase-08-network-after-timesyncd-stop.jsonl)、[网络期间时钟](phase-08-clock-policy-v3-during-network.json)。

- 用户批准的校时修订：[原版本与续跑版本的独立审计](phase-08-clock-amendment.json)。

- 正式续跑中的环境记录：[小幅校时事件](phase-08-clock-events.jsonl)、[S02恢复流量后的症状](phase-08-s02-resume-observation.json)、[S03基线检查后的查询](phase-08-s03-baseline-check.json)、[十九个容器的内存](phase-08-memory-s03-third.json)。这些记录说明当时看到什么，不把事后推测当成当时的原始响应。
- 另一份真实评分失败：[S03第一轮E3的协议检查记录](phase-08-s03-grade-failure.json)。JSON语法合法，但多出协议不允许的字段；原费用保留。

- [阶段8 S05第二轮内存快照](phase-08-memory-s05-second.json)：仅记录项目容器的实际内存。

- [阶段8 S06首次恢复的消息积压](phase-08-s06-recovery.json)：原健康检查失败，后续只读观察到追平。

- [阶段8 S06恢复后的内存](phase-08-memory-s06-resumed.json)：续跑时十九个容器的当前采样。

- [阶段8 S06第二轮恢复的消息积压](phase-08-s06-second-recovery.json)：再次保留健康检查超时与后续数值。

- [阶段8 S06第三轮的基线查询](phase-08-s06-third-baseline.json)：失败当次响应未保存，事后数值不能反推唯一根因。

- [S08基线误拦截](phase-08-s08-baseline-interruption.json)：场景要求400QPS，但诊断入口仍要求200QPS；模型槽位和费用增量均为零。该附件保留暂停当时的事实；后续批准和重启审计另列。

[流量守卫检查统计](phase-08-baseline-checks.json)记录本次 Mock 回归与零跳过集成检查；S08 原中断附件保持历史原样。

- [S08获准修订的审计](phase-08-baseline-amendment.json)：只修正实验流量守卫，原无模型轮次保留后重新注入。
- [每次执行的代码版本](phase-08-execution-versions.json)：记录各阶段与模型槽位使用的提交和源码哈希。
- S08续跑前的[时钟检查](phase-08-clock-s08-preflight.json)、[十分钟网络检查](phase-08-network-s08-preflight.jsonl)与[内存采样](phase-08-memory-s08-preflight.json)。
- S08第三轮恢复后的[内存采样](phase-08-memory-s08-third.json)，以及S09第一轮恢复后的[内存采样](phase-08-memory-s09-first.json)。
- [S09第二轮恢复后的采集空档](phase-08-s09-second-recovery.json)：用数据库的精确中断时间核对缺失指标，保留暂停与续跑记录。

- [S09第三轮故障期间内存](phase-08-memory-s09-third-fault.json)：与恢复后的快照分开，不能把单次采样当作峰值。

- [S10调用链ID核对](phase-08-s10-trace-lookup.json)：两条ID未找到，服务列表正常；只记录实际响应，不猜未采样或过期。

- [S10第三轮恢复后的内存](phase-08-memory-s10-third.json)：十九容器采样，沿用6GiB暂停线。

- [S13第二轮故障期间内存](phase-08-memory-s13-second-fault.json)：记录请求放大期间的单次占用。

- [正式矩阵最终计数](phase-08-final-numbers.json)：报告、有效评分、失败与两币种费用分开。
- [二十份人工盲评材料](phase-08-human-calibration.md)：空白表与原始观察，不填写用户分数；[抽样覆盖](phase-08-calibration-coverage.json)不暴露各份报告的模型身份。

- [人工评分填写入口](phase-08-human-scores.md)：只填这一张短表，每行可跳到对应原材料。
- [收尾检查统计](phase-08-final-checks.json)：Mock全仓竞态与零跳过集成分别计数。

- [最终Mock状态与余额对账](phase-08-final-runtime.json)、[最终十九容器内存](phase-08-memory-final-mock.json)：付费调用已结束，余额差额保留未归因。
