// Package grpcx 提供 gRPC 服务共用的三样东西：请求日志、panic 恢复和带时限的停机。
//
// 它和 httpx 是一对：httpx 管 HTTP 接口，grpcx 管 gRPC 接口。
//
// 拦截器（interceptor）是 gRPC 里的中间件：每个请求进来时先经过它们，再交给业务方法。
// 本包提供两个，安装顺序是 Logging 在外、Recovery 在内：
//   - Logging：给请求分配编号；请求结束后记一条日志，包括调用的方法、结果状态码和耗时。
//   - Recovery：业务方法如果 panic，把它转成 INTERNAL 错误返回，服务不会崩溃。
//
// Recovery 要放在内层：panic 被它转成错误之后，外层的 Logging 才能把这个结果记下来。
//
// Shutdown：gRPC 自带的 GracefulStop 会等所有正在处理的请求做完，但它不接受时限，
// 可能一直等下去。Shutdown 给它加了一个时限：时间到了还没做完，就强制停止。
//
// 本包只处理一元 RPC（一次请求、一次响应），不处理流式 RPC，也不依赖任何业务协议。
//
// 文件清单：
//   - doc.go：本说明。
//   - interceptor.go：Logging 和 Recovery。
//   - shutdown.go：带时限的 Shutdown。
//   - interceptor_test.go：测试两个拦截器的顺序，以及日志里不会出现请求内容。
//   - shutdown_test.go：测试时限到了以后，正在处理的请求会被取消。
//
// 阅读顺序：interceptor.go → shutdown.go → 两个测试。
package grpcx
