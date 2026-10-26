// Package httpapi 让人手动调用医生工具，也让告警进入医生数据库。
//
// 阶段 5 的医生还不会自己决定下一步。人先登录，再选择系统和工具。告警发送方
// 不需要人的登录令牌，它用所属系统的推送令牌。两种身份各走各的路，最终都不能
// 改写工具已经绑定的数据源。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - handlers.go：实现登录、告警接收、就绪检查和任务查询的 HTTP 细节。
//   - router.go：把登录、系统工具和告警入口接到同一个 Gin 服务。
//   - router_test.go：在内存 HTTP 请求里检查登录、系统令牌和工具入口。
//
// 阅读顺序：New → requireLogin / receiveAlert → runTool。这里没有页面或 Agent 循环。
package httpapi
