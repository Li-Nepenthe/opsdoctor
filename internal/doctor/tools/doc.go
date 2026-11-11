// Package tools 定义医生能做的只读查询。每个系统有自己的注册表，调用者只能从该系统已绑定的数据源取证。
// 所有入口都经过 Registry.Run。参数先校验，后端查询受十秒预算约束，调用方更短的时限优先。
// 整个 JSON 结果最多 4 KiB。超限时返回合法的预览，并保留原始大小和截断标记。
//
// 先读 catalog.go 看系统配置如何决定菜单，再沿 registry.go 的 Run 查看一次调用。
// 具体的参数约束在 schema.go，时间范围如何随故障起点变化见 window.go。
//
//   - catalog.go：从已配置的数据源构造当前系统的工具注册表。
//   - catalog_logs.go：把日志筛选条件交给当前系统的日志适配器。
//   - catalog_metrics.go：登记指标、告警、消息积压和近期变更查询。
//   - catalog_mysql.go：登记受限的执行计划与数据库元数据查询。
//   - catalog_runtime.go：登记 Redis 状态、慢命令和已配置服务的堆栈查询。
//   - catalog_traces.go：登记慢调用搜索与单条调用链查询。
//   - registry.go：统一校验并运行工具，记录状态、耗时和截断前后的大小。
//   - schema.go：从同一份字段声明构造公开 Schema 与运行时校验规则。
//   - summary.go：将时间序列和调用链整理成保留数量、极值及关系的摘要。
//   - runbooks.go：按关键词检索启动时载入的手册，并返回片段出处。
//   - window.go：按诊断起点设置默认回看范围，将本次可用的工具与其他诊断隔开。
package tools
