// Package adminserver 提供独立管理端口的健康检查。
//
// app 注入依赖探测并管理监听器。这里只处理 /health 和 HTTP 排空，不包含阶段 3 的指标或 pprof。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - server.go：管理端口的健康检查和有界关闭。
//   - server_test.go：在内存中验证健康状态和管理接口边界。
//
// 阅读顺序：server.go → server_test.go。
package adminserver
