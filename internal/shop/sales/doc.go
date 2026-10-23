// Package sales 把订单消息累加成商品销量，并提供订单与销量的总量核对。
// Kafka 只负责可能重复地送达消息。本包用数据库唯一键保证一笔订单只累加一次。
// orderworker 处理消息时调用本包，shopctl 借用 Check 核对总量。存储只依赖 MySQL。
// 本包不提交 Kafka 位点，也不补发丢失的订单消息。
//
// 文件清单：
//   - doc.go：说明销量处理与消息层的分工。
//   - model.go：总量核对结果。
//   - service.go：校验消息后调用仓储。
//   - repository.go：在同一事务中去重和累加，并查询一致性总量。
//   - repository_integration_test.go：在专用 MySQL 库验证并发重复与事务回滚。
//
// 先读 repository.go 的两条写语句，再看为什么它们必须在同一个事务里。
package sales
