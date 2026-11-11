// Package inventoryclient 供订单和商品查询调用库存 gRPC。查询允许重试，扣减和归还各只发一次：写请求丢失响应时，客户端无法判断库存是否已变。
// 从 GetStock 的总时限读起，再对照 Deduct 和 Release。查询和扣减分别统计技术故障，归还不受扣减熔断器阻挡。连接由订单服务关闭。
//
//   - inventory_client.go：执行库存查询、扣减和归还，将 RPC 错误映射为业务调用方能判断的结果。
//   - metrics.go：暴露查询和扣减各自的熔断状态，记录实际发出的查询重试。
package inventoryclient
