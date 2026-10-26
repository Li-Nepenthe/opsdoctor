// Package tools 把“医生可以问哪些问题”做成一份固定的工具清单。
//
// 工具注册表就像一张菜单。每个问题都有名字、参数格式和用途。调用者只能选择
// 菜单里的问题，不能临时要求执行 SQL、访问 URL 或运行命令。每个系统创建自己的
// 注册表，执行函数已经绑定它的数据源，参数中没有“换一个系统”的选项。
//
// 所有调用先校验参数，再在十秒内查询，最后把结果限制到 4KiB。结果太长时会说明
// 截断前有多大，并保留一个合法 JSON 预览。错误也是结果，程序不会因错误参数崩溃。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - catalog.go：根据一个系统实际配置的数据源，建立它自己的只读工具菜单。
//   - catalog_logs.go：登记由代码构造查询语句的日志搜索。
//   - catalog_metrics.go：登记告警、指标、消息积压和变更查询。
//   - catalog_mysql.go：登记六种用途固定的 MySQL 查看工具。
//   - catalog_runtime.go：登记 Redis 运行信息和 goroutine 堆栈摘要。
//   - catalog_test.go：验证完整工具菜单的共同边界，以及调用链摘要遇到坏数据时的行为。
//   - catalog_traces.go：登记慢请求搜索与单条调用链树。
//   - registry.go：让所有工具共用参数校验、十秒预算和四千字节结果限制。
//   - registry_test.go：验证工具统一入口的参数、超时、截断和系统绑定。
//   - schema.go：用同一份字段描述生成 JSON Schema，并校验实际工具参数。
//   - summary.go：把长时间线和重复的调用步骤整理成可阅读的证据摘要。
//
// 阅读顺序：Registry.Run → Schema.validate → finish。
package tools
