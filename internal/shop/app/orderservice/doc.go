// Package orderservice 组装订单 HTTP 进程。
// 它拥有 MySQL、Redis、Kafka 和库存 gRPC 连接，先准备依赖，再开放业务与管理端口。
// cmd/order-service 只调用 Run。订单和商品规则留在业务包，本包负责把实现连接起来。
//
// 文件清单：
//   - doc.go：说明进程资源的所有权。
//   - config.go：读取端口、缓存、幂等与服务治理参数。
//   - app.go：创建依赖、连接路由、排空请求并关闭资源。
//   - metrics.go：记录用户要求的待核对告警与计数。
//   - metrics_test.go：检查日志字段和指标，避免把订单编号放进指标标签。
//   - config_test.go：验证超时关系与治理配置边界。
//   - connection_test.go：验证首次连接完成握手，以及不可达时受启动预算限制。
//
// 先读 config.go 的预算关系，再读 app.go，最后看 metrics.go 的窄范围接入。
package orderservice
