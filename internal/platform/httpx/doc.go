// Package httpx 提供 Gin 的 Request ID、统一 JSON 响应、请求体限制和安全的 panic 恢复。
// shop 和 doctor 的 HTTP 适配层调用它；本包依赖 Gin 和 platform/logging，不认识业务错误。
// 路由、认证和状态码的业务映射由调用方负责；本包不会记录请求体或 panic 的原始内容。
package httpx
