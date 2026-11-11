// Package httpapi 把医生的告警、取证和实时页面接入同一个 HTTP 服务。告警发送方用所属系统的推送令牌，人用登录会话看诊断、描述症状或决定审批。
// 从 router.go 的 New 查看各条路由的身份要求。然后跟随一次告警到 handlers.go，或从 stream.go 看浏览器如何接收已保存的步骤。
//
//   - router.go：组装路由与登录保护，按系统绑定工具和告警身份。
//   - handlers.go：处理登录、就绪检查、告警落库和诊断查询。
//   - session.go：让页面使用 HttpOnly Cookie，并校验修改请求来自同一会话。
//   - diagnostics.go：接收手动任务和取消，展示医生实际启用的执行模式及预算。
//   - stream.go：按步骤号补发并推送事件，限制慢连接的写入时间。
//   - approvals.go：展示原审批单，记录登录用户的决定并唤醒诊断。
//   - evaluations.go：展示已保存的批次、结果和原评分。
package httpapi
