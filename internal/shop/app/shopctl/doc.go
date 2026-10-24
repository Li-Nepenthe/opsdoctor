// Package shopctl 是 shopctl 命令行工具的实现，用来在本机手动操作靶场。
//
// 三个子命令：
//   - seed：造测试数据（50 万商品、200 万订单）。会先连接数据库、执行迁移。
//   - stock get、stock deduct：通过 gRPC 调用库存服务，查询或扣减某个商品的库存。
//     它和订单服务调用的是同一个接口，所以能用来手动验证库存服务。
//   - check：核对“订单里卖出的总件数”和“销量表里统计的总件数”是否相等（阶段 2）。只读，不改数据。
//
// 谁调用它：cmd/shopctl/main.go 把命令行参数、配置读取器和输出位置传给 Run。
//
// 文件清单：
//   - doc.go：本说明。
//   - app.go：Run 分派子命令，以及 seed 命令的流程。
//   - stock_client.go：stock 命令：解析参数，调用库存服务。
//   - stock_client_test.go：测试参数解析，比如数量超出 int32 范围时会报错，而不是被悄悄截断。
//   - check.go：check 命令。
//   - check_test.go：测试总量不相等时会报告失败，而不是误报通过。
//
// 阅读顺序：app.go → stock_client.go → stock_client_test.go。check.go 留到阶段 2 再读。
package shopctl
