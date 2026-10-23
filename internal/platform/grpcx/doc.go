// Package grpcx 处理一元 RPC 的通用拦截和有界停机。
//
// 库存 app 把它安装到 grpc.Server；这里依赖 gRPC 与 logging，不依赖库存协议，也不处理流式 RPC。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - interceptor.go：给每个一元 RPC 加日志和 panic 恢复。
//   - interceptor_test.go：验证拦截器顺序与日志脱敏。
//   - shutdown.go：给 gRPC 的优雅停机加上总预算。
//   - shutdown_test.go：证明 gRPC 停机超时能取消在途请求。
//
// 阅读顺序：interceptor.go → shutdown.go → interceptor_test.go → shutdown_test.go。
package grpcx
