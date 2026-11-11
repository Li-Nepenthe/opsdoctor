// Package inventory 提供查询、扣减和归还库存的业务逻辑。库存服务将 gRPC 接口接到 Service，由仓储负责并发更新。
// 从 grpc_server.go 的 Deduct 追到 service.go，再看 repository.go 怎样用条件更新防止超卖。
//
//   - model.go：定义库存快照和业务错误。
//   - grpc_server.go：把请求传给库存服务，并将业务错误映射为 gRPC 状态。
//   - service.go：检查请求是否已取消，以及商品编号和件数是否合法。
//   - repository.go：在 MySQL 中读取或更新库存，区分未命中、库存条件不满足和数据库故障。
package inventory
