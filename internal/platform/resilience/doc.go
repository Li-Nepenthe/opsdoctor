// Package resilience 给外部调用加上限流、重试和熔断保护。
// 熔断可以理解为“下游一直出错，就先暂停访问一会儿”。本包不认识商品或订单。
// orderservice 使用 HTTP 限流，inventoryclient 使用重试和熔断。
// 令牌桶依赖 x/time/rate，熔断状态依赖 gobreaker。哪些业务错误值得重试由调用方决定。
//
// 文件清单：
//   - doc.go：说明本包的使用位置。
//   - http.go：令牌桶限流，以及请求的总时间预算。
//   - retry.go：只在调用方允许时重试，等待期间也能取消。
//   - breaker.go：封装 gobreaker，记录状态变化。
//   - resilience_test.go：验证限流、退避、取消和熔断恢复。
//
// 建议先读 http.go，再读 retry.go，最后读 breaker.go。
package resilience
