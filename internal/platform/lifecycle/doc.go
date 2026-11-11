// Package lifecycle 让服务在退出时为进行中的工作留出收尾时间。服务组装层负责安排关闭顺序，例如先排空请求，再关闭业务依赖的数据库。
//
//   - doc.go 说明停机时由谁安排资源的关闭顺序。
//   - lifecycle.go 处理退出通知和资源清理，从 Shutdown 开始可以看到关闭回调如何共用一个时限。
package lifecycle
