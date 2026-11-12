// Package events 把已经提交的步骤通知正在查看该诊断的页面。数据库保存完整历史，浏览器重连时用 Last-Event-ID 补回遗漏。
// 从 Subscribe 读到 Publish。每个订阅有自己的缓冲，慢浏览器只会断开自己的连接。
//
//   - hub.go：管理订阅、投递与关闭，在同一把锁下保护通道。
package events
