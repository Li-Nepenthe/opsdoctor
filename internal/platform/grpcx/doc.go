// Package grpcx 为一元 RPC 记录调用结果、处理 panic，并为服务停机设置期限。协议和业务错误由服务自己定义。
// 从 interceptor.go 的安装顺序读起，再看 shutdown.go 如何排空在途 RPC。
//
//   - interceptor.go：把 Logging 放在 Recovery 外层，让访问日志能记下 panic 恢复后的最终状态。
//   - shutdown.go：在清理期限内等待在途请求，超时后强制停止服务。
package grpcx
