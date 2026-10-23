// Package alertsink 提供阶段 3 的临时告警接收进程。
//
// cmd/alertsink 调用 Run，本包只依赖公共 HTTP、配置、日志和指标能力。
// 它验证推送令牌并记录通知，不负责诊断、审批，也不接触业务数据库。
//
// 文件清单：
//   - doc.go：说明这个临时接收器的位置和边界。
//   - app.go：读取配置、装配端口和关闭服务。
//   - handler.go：验证令牌、限制正文并打印通知摘要。
//   - handler_test.go：验证令牌、正文限制和日志不泄露秘密。
//
// 先读 handler.go 理解“收到告警”的含义，再读 app.go 的生命周期。
package alertsink
