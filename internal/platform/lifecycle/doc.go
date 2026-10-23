// Package lifecycle 将退出信号转换成取消通知，再按顺序清理资源。
//
// app 提供 Hook 并决定顺序。本包依赖 context 与系统信号，不知道 HTTP/gRPC 的具体关闭方法。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - lifecycle.go：把退出信号和资源清理组织成可复用流程。
//   - lifecycle_test.go：验证清理顺序、取消信号和停机预算。
//
// 阅读顺序：lifecycle.go → lifecycle_test.go。
package lifecycle
