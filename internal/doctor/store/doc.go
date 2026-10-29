// Package store 保存医生自己的用户、告警、诊断步骤和费用账本。
//
// 医生既要查看别的系统，也要记录自己的工作。这是两种不同的权限：source 里的
// 连接只能查看靶场；这里的连接可以写 doctor 库，但不连接 shop。把两者分开，
// 才不会因为需要保存任务，就顺便把业务库的写权限交给医生。
//
// 文件清单：
//   - approvals.go：事务保存审批、一次决定和执行线索。
//   - approval_integration_test.go：验证等待、争抢、过期和执行恢复。
//   - leases.go：领取和续租执行权，过时断点不再继续。
//   - model_recovery.go：从费用审计找回崩溃前已收到的回答。
//   - lease_integration_test.go：验证租约争抢、回复恢复和币种隔离。
//   - mcp_integration_test.go：用专用库验证外部调用审计和脱敏。
//   - mcp.go：把外部 MCP 调用的开始和结果单独保存。
//   - agent_integration_test.go：在专用数据库验证故障开放期、任务清理、步骤编号和累计费用。
//   - budget.go：把真实模型的费用预留与结算保存到医生数据库。
//   - steps.go：给步骤分配编号，并为页面断线后的补发提供有序读取。
//   - tasks.go：保存调度状态变化，隔离启动清理、排队和结束操作。
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - alerts.go：在一个事务中去重告警分组、创建任务、保存告警快照。
//   - diagnoses.go：读取已经创建的诊断任务，供验收和后续页面使用。
//   - faults.go：读取故障开放状态，以及恢复观察期补入的告警。
//   - recovery_integration_test.go：用可控时间和真实事务核对晚到告警、期限与任务数。
//   - store.go：建立医生仓储并应用医生自己的迁移。
//   - store_integration_test.go：在专用医生测试库验证迁移、通知事务和分组去重。
//   - users.go：保存首次登录账号，并按邮箱查找密码摘要。
//
// 阅读顺序：Migrate → ReceiveAlerts → StartTask → RecordStep → EndTask；费用另从 budget.go 阅读。
package store
