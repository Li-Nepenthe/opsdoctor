// Package alerts 校验 Alertmanager 通知，并把同一系统、同一分组的重发交给仓储去重。HTTP 层已经认证推送方，接收回执只说明通知的处理结果。
// 从 Service.Receive 读到 Receiver，通知通过校验、仓储保存成功后，HTTP 入口才确认收到。
//
//   - service.go：检查分组状态和各条告警，生成展示标题，并转交仓储持久化。
package alerts
