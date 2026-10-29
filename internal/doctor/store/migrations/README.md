# 医生自己的表

医生既要查看靶场，也要保存自己的工作。这是两套权限。这里的迁移只作用于 doctor 库，不会更改业务表。

文件按编号执行。每个文件只有一条 SQL，已经执行过的文件不能修改；需要变更结构时新增下一个编号。

| 文件 | 保存什么，为什么需要 |
|---|---|
| 0001_users.sql | 登录账号和密码摘要 |
| 0002_diagnoses.sql | 一次诊断的初始输入、状态和报告；保留阶段 5 已建立的结构 |
| 0003_diagnosis_alerts.sql | 同一诊断收到的各条告警最新快照 |
| 0004_agent_fields.sql | 下一步骤号、provider、缓存命中量和执行计数 |
| 0005_diagnosis_steps.sql | 每一步的参数、结果、实际模型与用量，页面断线后据此补发 |
| 0006_alert_groups.sql | 故障是否仍开放；报告完成后，后续通知仍并入原任务 |
| 0007_llm_budget.sql | 跨诊断、跨进程重启的累计费用和预留额 |
| 0008_llm_attempts.sql | 每次真实 HTTP 尝试发送前后的脱敏正文和费用；重试也单独记账 |
| 0009_cancel_legacy_pending.sql | 把阶段 5 留下的告警任务标为过时，升级不能把旧任务变成付费诊断 |
| 0010_budget_stop.sql | 实际用量超过预留估算时，保存停用原因；重启也不会绕过人工核对 |
| 0011_alert_recovery.sql | 整组恢复后的观察截止时间；晚到症状仍归到原诊断 |
| 0012_recovery_alerts.sql | 首次在观察期内补入的时间；之后恢复也保留这条来历 |
| 0013_pricing_and_report.sql | 保存每次请求的三档价格，旧账不改写 |
| 0014_report_reason.sql | 保存报告是主动提交还是预算触发，以及具体原因 |
| 0015_mcp_audit.sql | 保存每一次外部 MCP 调用，未知工具也留下记录 |
| 0016_lease_clock.sql | 记录执行时间、续约时间和用户取消意图 |
| 0017_step_identity.sql | 为一轮请求和工具结果分配唯一标识，恢复不重复记步骤 |
| 0018_attempt_call.sql | 把实际 HTTP 尝试对应到发送前保存的请求 |
| 0019_scheduler_lock.sql | 创建跨进程分配名额时使用的锁表 |
| 0020_scheduler_guard.sql | 写入唯一的协调行，两个进程不能同时抢到同一名额 |
| 0021_budget_currency.sql | 为账本固定币种，旧金额仍为人民币 |
| 0022_diagnosis_currency.sql | 保存诊断币种和缓存写入用量 |
| 0023_step_currency.sql | 保存每步币种、缓存写入和思考 token |
| 0024_attempt_cache_write.sql | 保存每次调用的缓存写入用量和当时单价 |
| 0025_approvals.sql | 保存不可变审批计划、一次决定、执行者与旧 DDL 连接线索 |

任务结束与故障恢复是两件事。阶段 6 不再用 active_group 判断故障开放期，而用 alert_groups 保存整组恢复和最近通知时间。旧列保留以兼容已有迁移，后续没有拿它创建第二套去重规则。

网络中断时，供应商可能已经产生费用。llm_attempts 保留预留额，不自动退回。这样失败或重启不会悄悄扩大剩余额度；实际账单差异由实验记录说明。
