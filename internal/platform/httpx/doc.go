// Package httpx 提供所有 HTTP 接口共用的入口处理和统一的响应格式。
//
// 每个 HTTP 请求在进入业务代码之前，都要经过几道相同的处理。本包把它们集中起来，
// 业务代码就不用每个接口都重复写一遍。它们以 Gin 中间件的形式安装：
//   - RequestIDs：给请求分配一个 request_id，写进响应头和日志；请求结束时记一条访问日志。
//   - Recovery：业务代码如果 panic（类似 Java 里没被捕获的异常），在这里接住，
//     返回统一的 500 错误，而不是让整个服务崩溃。
//   - BodyLimit：限制请求体的大小（默认 1 MiB），防止超大请求把内存撑爆。
//
// 业务 handler 里还会用到三个函数：
//   - BindJSON：把请求体解析成结构体。格式不对时，它已经替你返回了 400。
//   - Success、Failure：按统一格式写响应。成功是 {"code":"OK","data":...}，
//     失败是 {"code":"错误码","message":"说明"}，两者都带 request_id。
//
// 谁调用它：订单服务（阶段 2 起）和压测工具的控制接口。本包用 logging 包写日志，不包含任何业务接口。
//
// 文件清单：
//   - user_agent.go：有界记录客户端说明，疑似凭据省略，文字保持数据身份。
//   - user_agent_test.go：验证长中文、凭据和伪造日志字段的处理。
//   - doc.go：本说明。
//   - httpx.go：上面提到的所有中间件和函数。
//   - httpx_test.go：不启动真实服务器，直接模拟请求，检查 request_id、panic 恢复和请求体限制。
//
// 阅读顺序：httpx.go 从上往下读即可。先看 RequestIDs 和 Recovery，留意它们为什么要按这个顺序安装；
// 再看 BodyLimit 和 BindJSON 怎样配合，限制住请求体的大小。
package httpx
