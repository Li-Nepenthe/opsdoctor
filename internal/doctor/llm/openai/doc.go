// Package openai 通过非流式 HTTP 调用模型。每次重试都有独立的预留和审计记录，未知费用继续占用预算。
//
// 从 Complete 跟到 attempt 查看一次发送。回复类型和 HTTP 状态需要严格校验，完整的脱敏错误回复也要留在审计中。取消信号一直传到 HTTP 请求。
//
//   - client.go：校验配置、发送请求，并按每次尝试预留和结算。
//   - protocol.go：读取有界回复，分类 HTTP 错误，并在重试前按时限等待。
//   - responses.go：转换 Chat 和 Responses 的工具往返，解析在线回复与保存的审计回复。
package openai
