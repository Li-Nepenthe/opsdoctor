// Package telemetry 负责可观测性的公共部分：指标和调用链。各服务用它统一接入，业务代码不用关心细节。
//
// 先分清两个概念（第三个概念“日志”在 logging 包）：
//   - 指标：一段时间内的数字，比如每秒请求数、P99 耗时、错误率，回答“系统整体怎么样”。
//     Prometheus 每 10 秒到各服务的管理端口（/metrics）取一次。
//   - 调用链：一次请求经过了哪些操作、每一步花了多久，回答“这一笔请求慢在哪里”。
//     一次请求叫一条 trace，其中每一步操作叫一个 span。span 会被批量发送给 Jaeger。
//
// 用法：每个服务启动时调用 New，得到一个 Runtime。它包含三样东西：指标注册表（Metrics）、
// 负责创建 span 的 Provider、负责在服务之间传递调用链信息的 Propagator。
// app 把它们分别交给 HTTP、gRPC、数据库、Redis、Kafka 这些组件，不使用全局变量。
//
// 跨服务传递：HTTP 和 gRPC 有现成的插件，会自动在请求头里带上调用链信息；kafka-go 没有，
// 所以 kafka.go 手动把调用链信息写进 Kafka 消息头，消费者再从消息头里读出来。
// 这样一笔下单从订单服务一直到 order-worker，都在同一条 trace 里。
//
// 采样：默认只记录十分之一的请求（由环境变量控制），减少本机的资源占用。
//
// 本包不认识商品、订单这些业务。
//
// 文件清单：
//   - doc.go：本说明。
//   - metrics.go：指标注册表，以及各服务共用的指标：HTTP 和 gRPC 的耗时、Go 运行时和数据库连接池的状态等。
//   - tracing.go：New 和 Runtime；把 span 发送给 Jaeger 的导出器；退出时把还没发出的 span 发完。
//   - middleware.go：HTTP 中间件和 gRPC 拦截器，记录每个请求的耗时和状态码。
//   - kafka.go：让调用链信息能写进、读出 Kafka 消息头。
//   - telemetry_test.go：测试路由标签（/orders/1 和 /orders/2 都记为 /orders/:id）、gRPC 状态码，以及消息前后的调用链能串起来。
//   - tracing_test.go：测试发送 span 时跳过 DNS 的 TXT 查询（阶段 3 返工的修复），并且 span 能真正送达。
//
// 阅读顺序：metrics.go → tracing.go → middleware.go → kafka.go。
package telemetry
