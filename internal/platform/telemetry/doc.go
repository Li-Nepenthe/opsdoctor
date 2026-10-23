// Package telemetry 把指标与调用链的公共规则集中起来。
//
// 服务 app 创建 Runtime，然后把指标和 Provider 显式交给适配器。
// 本包不认识商品、订单或销量，只提供计数、计时和上下文传播。
//
// 文件清单：
//   - doc.go：说明公共监控包的位置和边界。
//   - metrics.go：独立注册表、固定指标名称、Go 与连接池统计。
//   - tracing.go：OTLP 初始化、版本信息与有限时间的排空。
//   - middleware.go：HTTP 和 gRPC 的耗时、状态码与限流指标。
//   - kafka.go：把 Kafka 消息头适配给 W3C 上下文传播器。
//   - telemetry_test.go：验证路由标签、RPC 状态和跨消息的父子关系。
//
// 建议先读 metrics.go，再读 tracing.go，最后看两种跨服务入口。
package telemetry
