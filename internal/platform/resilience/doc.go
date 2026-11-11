// Package resilience 为订单入口限流，并在调用下游时控制重试和熔断。由调用方判断哪些错误值得重试，哪些应计入熔断。
// 从 http.go 的请求时限读起，再看 retry.go 怎样分配剩余时间，以及 breaker.go 怎样限制探测流量。
//
//   - http.go：在单个实例内限流，并把请求的总时限传给后续处理。
//   - retry.go：在预算内按退避时间重试，调用方取消时结束等待。
//   - breaker.go：按失败比例暂停下游调用，并记录熔断器的状态变化。
package resilience
