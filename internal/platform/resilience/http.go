// 本文件：在 HTTP 入口限制每个实例的流量，并为业务处理和下游调用设置共同的截止时间。
// 订单入口先由 RateLimit 判断是否放行，RequestBudget 再将期限传给下游。

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

// RateLimit 在一个进程内共享令牌桶；扩容后各实例独立限流。
func RateLimit(requestsPerSecond, burst int) (gin.HandlerFunc, error) {
	if requestsPerSecond <= 0 || burst <= 0 {
		return nil, fmt.Errorf("限流速率和桶容量必须大于 0")
	}
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
	return func(ctx *gin.Context) {
		// 额度用完立即拒绝，排队等待还会占用连接和内存。
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
