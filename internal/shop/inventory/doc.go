// Package inventory 是库存服务的业务代码：查询库存、扣减库存、归还库存。
//
// 一次扣减请求在本包里依次经过三层，每层只做一件事：
//  1. grpc_server.go（GRPCServer）：接收 gRPC 请求，把 protobuf 消息转成普通的 Go 值；
//     处理完后，再把业务错误翻译成 gRPC 状态码，例如“库存不足”对应 FailedPrecondition。
//  2. service.go（Service）：检查参数，比如数量必须大于 0。
//  3. repository.go（MySQLStockRepository）：执行 SQL。扣减用一条带条件的 UPDATE 完成，保证不会超卖。
//
// 为什么分三层：gRPC 的类型只出现在第 1 层，SQL 只出现在第 3 层。Service 只依赖一个
// StockRepository 接口，所以测试时可以换成假的仓储，不用启动数据库。
//
// 谁调用它：inventoryservice 在启动时把三层组装起来，注册到 gRPC 服务器上。
// 本包只管库存，不创建订单，也不发送消息。
//
// 文件清单：
//   - doc.go：本说明。
//   - model.go：库存快照 Stock，以及四种业务错误：参数不合法、商品不存在、库存不足、存储暂时不可用。
//   - service.go：StockRepository 接口和 Service。
//   - repository.go：三条 SQL，以及把数据库错误归类的逻辑。
//   - grpc_server.go：gRPC 接口的实现，以及业务错误到状态码的映射。
//   - service_test.go：用假仓储测试参数检查和取消。
//   - repository_test.go：不连数据库，测试数据库错误的归类。
//   - repository_integration_test.go：连真实 MySQL，测试 100 个请求同时抢 50 件库存，以及超时能否传到数据库。
//   - grpc_server_test.go：在内存里起一个 gRPC 服务器，测试真实的请求往返和错误码。
//
// 阅读顺序：model.go → service.go → repository.go → grpc_server.go，然后按同样的顺序读测试。
// 最重要的是 repository.go 里 deductStockSQL 上面的注释。
package inventory
