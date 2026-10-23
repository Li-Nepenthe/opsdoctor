// 本文件：在 HTTP 入口控制请求数量和处理时间。
//
// 请求进入业务前先经过这里。把入口保护单独放出来，各个接口就不用重复检查额度。
// 令牌桶像一个按固定速度补票的盒子。每个请求拿一张票，没票就立即拒绝。
//
// 主要内容：
//   - RateLimit：创建一个进程共享的令牌桶，额度用完时返回 429。
//   - RequestBudget：给后续处理和下游调用设置共同的截止时间。
//
// 阅读建议：先看 Allow 为什么不等待，再看 context 如何传给后面的 handler。

package resilience

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit 为整个服务创建一个桶。多个实例各有自己的桶，不是全局统一限流。
func RateLimit(requestsPerSecond, burst int) (gin.HandlerFunc, error) {
	if requestsPerSecond <= 0 || burst <= 0 {
		return nil, fmt.Errorf("限流速率和桶容量必须大于 0")
	}
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
	return func(ctx *gin.Context) {
		// 入口已经拥挤时不继续排队，否则等待的请求本身也会占用内存和连接。
		if !limiter.Allow() {
			httpx.Failure(ctx, http.StatusTooManyRequests, "RATE_LIMITED", "请求太多，请稍后再试")
			return
		}
		ctx.Next()
	}, nil
}

// RequestBudget 把总预算放进请求 context。下游可以再缩短预算，但不能延长它。
func RequestBudget(timeout time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestContext, cancel := context.WithTimeout(ctx.Request.Context(), timeout)
		defer cancel()
		ctx.Request = ctx.Request.WithContext(requestContext)
		ctx.Next()
	}
}
