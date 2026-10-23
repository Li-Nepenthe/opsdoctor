// Package inventory 实现库存查询、扣减和归还，分开业务、数据与传输职责。
//
// inventoryservice 负责组装。GRPCServer 调用 Service，Service 经 StockRepository 调用 MySQL 实现。本包不生成订单或发送消息。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - grpc_server.go：把 protobuf 请求接到库存业务并映射状态码。
//   - grpc_server_test.go：验证 gRPC 错误契约和内存通道往返。
//   - model.go：库存领域的快照类型与错误分类。
//   - repository.go：库存表的查询与原子更新。
//   - repository_integration_test.go：真实 MySQL 的并发扣减与 deadline 实验。
//   - repository_test.go：离线区分暂时故障和 SQL 配置错误。
//   - service.go：库存请求进入仓储前的业务规则。
//   - service_test.go：离线验证参数规则、取消和仓储错误传递。
//
// 阅读顺序：model.go → service.go → repository.go → grpc_server.go，再按相同顺序读测试。
package inventory
