// Package httpapi 把告警、人工取证和实时页面接到同一个医生。
//
// 人登录后可以看诊断、描述症状、取消任务和决定审批，告警进入后由调度器自动执行。告警发送方
// 不需要人的登录令牌，它用所属系统的推送令牌。两种身份各走各的路，最终都不能
// 改写工具已经绑定的数据源。
//
// 文件清单：
//   - evaluations_test.go：检查结果查询的登录边界、只读约束和评分失败保留。
//   - evaluations.go：向登录用户展示批次和结果，不提供修改评分或启动实验的入口。
//   - approvals.go：展示审批并记录登录用户的决定。
//   - approvals_test.go：验证审批鉴权、CSRF 和参数不可替换。
//   - diagnostics.go：接收手动症状、取消任务，并向页面说明当前执行模式和预算。
//   - session.go：为浏览器提供 Cookie 登录和防跨站请求保护，命令行继续使用 Bearer。
//   - session_test.go：从浏览器 Cookie 路径验证登录和 CSRF 边界。
//   - stream.go：把已保存的步骤通过 SSE 发给页面，断线后按编号补回。
//   - stream_test.go：验证 SSE 续传、顺序和心跳，不依赖真实端口或真实时间。
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - handlers.go：实现登录、告警接收、就绪检查和任务查询的 HTTP 细节。
//   - router.go：把登录、系统工具和告警入口接到同一个 Gin 服务。
//   - router_test.go：在内存 HTTP 请求里检查登录、系统令牌和工具入口。
//   - recovery_test.go：核对故障开放状态和后续告警的接口字段与鉴权。
//
// 阅读顺序：New → requireLogin / receiveAlert → manual → stream。Agent 的循环不放在 HTTP 中。
package httpapi
