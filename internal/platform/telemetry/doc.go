// Package telemetry 为每个进程建立独立的指标注册表和调用链。服务组装时显式传入这些组件，
// 同一进程内的不同实例也能各自采集和导出数据。
//
// 从 tracing.go 的 New 看服务名、采样和导出地址如何生效，再看 metrics.go 中看板和告警使用哪些指标。
// HTTP 与 gRPC 由插件传递调用链。Kafka 需要在发送和消费时读写消息头，才能把两端的操作连起来。
//
//   - tracing.go：创建调用链组件，并在进程退出前给最后一批记录留出导出时间。
//   - metrics.go：注册请求、消息和运行时指标，为所有采集器添加相同的服务标签。
//   - middleware.go：在 HTTP 和 gRPC 请求结束时记录耗时、路由或方法及状态。
//   - kafka.go：在 Kafka 消息头中读写调用链信息，保持业务消息格式不变。
package telemetry
