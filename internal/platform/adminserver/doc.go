// Package adminserver 提供独立管理端口的健康检查、指标和运行时剖析。
//
// app 注入依赖探测与指标注册表，并管理监听器。本包不提供任何业务接口或运维写操作。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - server.go：管理路由、健康检查和有界关闭。
//   - server_test.go：在内存中验证健康状态和管理接口边界。
//
// 阅读顺序：server.go → server_test.go。
package adminserver
