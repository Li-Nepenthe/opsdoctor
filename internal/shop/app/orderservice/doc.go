// Package orderservice 负责把订单服务组装起来并运行，从启动到退出的全过程都在这里。
//
// 订单服务要连的东西比库存服务多得多：MySQL、Redis、Kafka，还有库存服务的 gRPC。
// Run 先把这些连接全部准备好，再打开两个端口：业务端口 8081（商品和订单的 HTTP 接口）、
// 管理端口 6061（健康检查、指标）。退出时反过来：先关端口、等请求做完，再关各种连接。
//
// 一个重要的细节：启动时会先等库存服务的 gRPC 连接真正建立好，才开始接受请求。
// 否则第一个用户的下单请求要替服务承担建立连接的时间，可能因此超时。
//
// 订单和商品的业务规则在 order、product 包里，本包只负责把它们和各种连接接起来。
//
// 文件清单：
//   - doc.go：本说明。
//   - config.go：所有配置项和默认值，以及几个时限之间必须满足的关系（比如库存调用的时限要比整个 HTTP 请求短）。
//   - app.go：Run：按顺序创建依赖、启动服务、处理退出。
//   - dependencies.go：创建和关闭 MySQL、Redis、Kafka、gRPC 连接。
//   - servers.go：把连接组装成业务对象，创建业务端口和管理端口。
//   - metrics.go：“结果待核对”的告警日志和计数。
//   - observations.go：给缓存、回源、幂等这些操作加上计数，业务包不用直接依赖监控库（阶段 3 加入）。
//   - response_direct.go：正常构建的响应收尾，保留原处理流程。
//   - response_async.go：另一份构建中的后台响应收尾，阅读时关注通道发送和接收的配合。
//   - config_test.go：测试配置的边界和时限之间的关系。
//   - connection_test.go：测试启动时等待库存连接建立，以及连接闲置回收后重新连接时不会卡在 DNS 查询上。
//   - metrics_test.go：测试告警日志的字段和计数，以及不会把订单号这类值放进指标标签。
//
// 阅读顺序：config.go（重点看几个时限的关系）→ app.go → dependencies.go → servers.go。
package orderservice
