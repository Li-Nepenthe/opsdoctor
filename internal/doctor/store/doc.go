// Package store 把医生自己的工作保存到 doctor 库。页面、调度器和模型客户端使用这里的连接记录诊断、步骤和费用。
// 靶场的业务数据由独立的只读数据源查询，不与医生的写入账号共用连接。
//
// 从 alerts.go 的 ReceiveAlerts 看告警如何成为一份诊断，再沿 tasks.go、leases.go 和 steps.go 看任务如何领取、续跑和结束。
// 一次模型尝试的预留和退款在 budget.go。评测另有冻结输入和结果，第一次阅读诊断流程时可以跳过 eval_ 开头的文件。
//
//   - store.go：装配 doctor 数据库连接池、仓储回调与迁移。
//   - users.go：按邮箱创建或查找账号，保存密码摘要。
//   - alerts.go：在接收告警的事务内判断故障归属，保存诊断和告警快照。
//   - faults.go：查询故障恢复观察期和告警快照的来源。
//   - diagnoses.go：读取诊断列表和详情。
//   - tasks.go：按原状态条件更新任务，记录状态步骤并在提交后通知。
//   - mode.go：在候选、领取、告警和审批的原事务中执行评测隔离规则。
//   - leases.go：领取和续租执行权，阻止持有旧租约的执行者继续写入。
//   - steps.go：锁住诊断后分配步骤编号，支持按序补读。
//   - budget.go：在模型调用前预留费用，在得到回复后结算已知用量。
//   - model_recovery.go：从尝试审计中找回已收到但中断前未保存到步骤的回答。
//   - approvals.go：保存审批计划、决定和执行结果。
//   - mcp.go：单独保存外部工具调用的开始和结束审计。
//   - evaluation.go：记录评测模式，按调用者标识创建可重复提交而不会重复执行的评测诊断。
//   - eval_budget.go：在费用事务里一起预留和结算评测批次的 token。
//   - eval_execution.go：保存每次评测执行所用的提交号和代码哈希。
//   - eval_results.go：冻结每个实验的告警输入，记录各组终态结果，阻止结论被覆盖。
//   - eval_trials.go：记录整轮实验的故障注入、恢复、冷却进度与中断原因。
//   - eval_judge.go：绑定评委任务的输入、模型和账户，按已存回复恢复评分。
//   - eval_listing.go：查询页面使用的批次列表和结果。
//   - eval_lock.go：以数据库连接锁防止两份进程同时执行同一评测，丢锁时取消执行。
//   - eval_reliability.go：依据请求审计区分失败原因，并限制同一位置的补跑次数。
//   - eval_calibration.go：保存首次抽取的人工核对样本编号。
//   - eval_clock_amendment.go：保存历史批次中获批的校时版本变更及指定的补跑许可。
//   - eval_baseline_amendment.go：保存历史批次获准重启前的版本和注入时间，便于核对重启前后分别执行了哪些轮次。
package store
