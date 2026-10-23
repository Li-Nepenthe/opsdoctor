// Package inventoryclient 把商品和订单需要的库存操作转换成 gRPC 调用。
// 商品查询可以重试，扣减和归还各发一次。两个方向不共用重试逻辑，免得重复修改库存。
// app 创建连接并负责关闭，本包只借用生成的客户端。
// 本包依赖 gen/inventory/v1 的契约和 platform/resilience 的保护，不执行库存 SQL。
//
// 文件清单：
//   - doc.go：说明谁使用本包，以及连接由谁管理。
//   - inventory_client.go：为库存调用加超时、重试和熔断，并转换错误。
//   - metrics.go：登记熔断状态与实际查询重试次数。
//   - inventory_client_test.go：用假的 RPC 客户端检查调用次数和错误分类。
//
// 建议先读 GetStock，再比较 Deduct 和 Release 为什么没有重试。
package inventoryclient
