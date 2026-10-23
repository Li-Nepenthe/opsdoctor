// Package orderservice 组装订单 HTTP 进程。
// 它拥有 MySQL、Redis、Kafka 和库存 gRPC 连接，先准备依赖，再开放业务与管理端口。
// cmd/order-service 只调用 Run。订单和商品规则留在业务包，本包负责把实现连接起来。
//
// 文件清单：
//   - doc.go：说明进程资源的所有权。
//   - config.go：读取端口、缓存、幂等与服务治理参数。
//   - app.go：创建依赖、连接路由、排空请求并关闭资源。
//   - dependencies.go：准备连接、安装插件，失败或停机时按顺序关闭。
//   - servers.go：把连接装配成业务服务，创建业务与管理 HTTP 入口。
//   - metrics.go：记录用户要求的待核对告警与计数。
//   - observations.go：给缓存、回源和幂等接口加计数，业务包不直接认识 Prometheus。
//   - metrics_test.go：检查日志字段和指标，避免把订单编号放进指标标签。
//   - config_test.go：验证超时关系与治理配置边界。
//   - connection_test.go：验证首次连接完成握手，以及不可达时受启动预算限制。
//
// 先读 config.go 的预算关系，再读 app.go，最后看两个指标包装文件如何保留原接口。
package orderservice
