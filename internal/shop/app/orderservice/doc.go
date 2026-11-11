// Package orderservice 由订单进程入口调用，负责组装商品查询与下单服务。开放业务端口前要等库存服务完成首次握手，避免第一笔订单耗时建连。停机时先排空请求，再关闭 Kafka、gRPC、Redis 和数据库连接。
//
// config.go：读取进程配置，校验库存调用、订单收尾和 HTTP 请求的时间预算。
// dependencies.go：按依赖顺序连接四个后端，启动中断时收回已经打开的连接。
// servers.go：把仓储和客户端交给商品、订单服务，再创建业务与管理端口。
// app.go：登记路由并启动两个端口，收到退出信号后等待请求完成。
// observations.go：在业务接口外统计缓存、回源和幂等结果。
// metrics.go：对首次进入待核对的订单记一次指标，并留下人工核对所需的日志。
// response_direct.go：正常构建的响应收尾，保留原处理流程。
// response_async.go：另一份构建中的后台响应收尾，阅读时关注通道发送和接收的配合。
package orderservice
