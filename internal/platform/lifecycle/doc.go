// Package lifecycle 提供退出信号监听和有总时限的清理流程。
// app 组装层先停止接收工作、取消任务，再按依赖顺序关闭资源；本包不认识具体服务。
// 它仅依赖标准库，不启动自有 goroutine；信号资源由调用方使用返回的 stop 释放。
package lifecycle
