// Package adminserver 提供每个服务的“管理端口”：给运维和监控用，不处理业务请求。
//
// 每个服务开两个端口。业务端口处理业务请求，比如库存服务的 gRPC 端口 9001；
// 管理端口（库存服务是 6062）提供下面这些地址：
//   - /health：健康检查。会真的 Ping 一次数据库，数据库断开时返回 503。Docker 的 healthcheck 调用的就是它。
//   - /metrics：给 Prometheus 采集的指标（阶段 3 加入）。
//   - /debug/pprof/：Go 自带的性能分析，可以查看 goroutine、CPU、内存的情况（阶段 3 加入）。
//
// 为什么单独开一个端口：这些地址是给运维和监控用的，不该和业务接口混在一起。
// 分开以后，业务端口上根本没有它们。
//
// 健康检查具体检查什么，由各服务传入（例如数据库的 PingContext），本包不认识任何业务。
//
// 文件清单：
//   - doc.go：本说明。
//   - server.go：New 创建管理端口，Shutdown 带时限地关闭它。
//   - server_test.go：不启动真实端口，测试健康检查返回 200 或 503，以及指标和性能分析地址能正常访问。
//
// 阅读顺序：server.go → server_test.go。
package adminserver
