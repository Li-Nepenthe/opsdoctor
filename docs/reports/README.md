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

阶段 8 进行中：先看 [手册与试跑审核材料](phase-08-pilot.md) 和 [数据库导出结果](phase-08-pilot-results.md)。这是第一个停点，不是阶段完成汇报。

第一个停点已获同意。后续的[开跑前检查](phase-08-preflight.md)记录网络阻塞及重试、计时修复；原始[网络采样](phase-08-network-preflight.jsonl)和[时钟采样](phase-08-clock-preflight.json)只含状态与时间，不含凭据。正式评测尚未开始。

用户校时后的[六十次稳定性复核](phase-08-clock-stability.jsonl)仍有跳变，当前先等待时钟稳定。
