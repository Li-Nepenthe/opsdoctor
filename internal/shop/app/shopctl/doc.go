// Package shopctl 组装宿主机造数与库存调试命令。
//
// cmd/shopctl 调用 Run；seed 走迁移和造数包，stock 走生成的 gRPC 客户端。本包不实现下单或后台消费。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - app.go：分派宿主机调试命令并组织造数流程。
//   - stock_client.go：解析库存命令并通过正式 gRPC 契约调用服务。
//   - stock_client_test.go：验证命令行参数不会被误解或截断。
//
// 阅读顺序：app.go → stock_client.go → stock_client_test.go。
package shopctl
