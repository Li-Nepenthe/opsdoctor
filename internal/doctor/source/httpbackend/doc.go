// Package httpbackend 让医生在固定地址上查询监控后端。请求只用 GET，拒绝重定向，并限制等待时间和读取量。后端错误正文可能含凭据，只返回分类或状态。
//
// 从 New 查看地址如何绑定，再看 Bytes 如何限制每次读取。
//
//   - client.go：绑定监控后端，读取有大小上限的 JSON 或文本响应。
package httpbackend
