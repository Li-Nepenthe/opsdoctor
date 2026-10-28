// Package openai 用普通 HTTP 实现兼容 OpenAI 的非流式工具调用协议。
//
// 一次 Complete 可能遇到限流，需要稍后重试。每次真正发送都先向账本预留费用，
// 并保存实际发出的脱敏 JSON。错误码决定是否重试，模型返回的文字不能改变这条规则。
// HTTP 请求使用诊断的 context，因此取消按钮能中断正在等待的模型回答。
//
// 文件清单：
//   - doc.go：HTTP 模型客户端的边界与阅读入口。
//   - client.go：配置检查、脱敏、发送和费用结算。
//   - protocol.go：读取回复、解释错误码、按 Retry-After 等待。
//   - client_test.go：在内存中验证重试、审计内容和取消，不调用真实模型。
//
// 阅读顺序：Complete → attempt → decodeResponse → settle。
// 本包不执行工具，也不读取系统说明书或故障场景。
package openai
