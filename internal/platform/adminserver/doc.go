// Package adminserver 供各服务在独立端口暴露健康检查、指标和性能剖析。服务在启动时提供自己的依赖探测，监控据此判断服务是否就绪。
//
//   - doc.go 说明管理端口与业务端口的分工。
//   - server.go 装配并关闭管理服务器，从 New 开始可以看到每个管理入口能访问哪些数据。
package adminserver
