// Package events 把已经保存的步骤送给正在看页面的人。
//
// 数据库是完整历史，内存通道只是实时通知。一个浏览器太慢时，只断开它自己的
// 订阅，不让诊断等它读取。它重连后按 Last-Event-ID 从数据库补齐遗漏。
//
// 文件清单：doc.go 是本说明；hub.go 管理订阅；hub_test.go 验证隔离和慢订阅者断开。
// 阅读顺序：hub.go 的 Subscribe → Publish → Subscription.Close。
// 本包不直接写 HTTP，也不负责持久化。
package events
