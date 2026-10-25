// 本文件：发送真实 HTTP 请求，并按约定重试超时下单。
//
// 每次尝试都单独记录耗时和结果。重试成功也不会把第一次的超时从错误率里抹去。
//
// 主要内容：
//   - execute：下单超时后最多再试一次，保留原请求体和幂等键。
//   - attempt：构造请求，读取有上限的响应并检查成功信封。
//   - timeoutError：识别网络或 context 超时，不重试普通业务拒绝。
//
// 阅读建议：503 和待核对的 409 都不自动重试，只有超时符合本工具的重试条件。

package loadgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// execute 把一次逻辑下单限制在两次 HTTP 尝试，第二次沿用同一个 job 的 key。
// 返回值表示最终是否失败，供单批商品查询统计使用；全程统计仍保留每次真实尝试。
func (generator *generator) execute(ctx context.Context, value job) bool {
	for attempt := 0; attempt < requestAttempts; attempt++ {
		started := time.Now()
		failed, timedOut := generator.attempt(ctx, value)
		generator.stats.record(value.kind, time.Since(started), failed)
		result := "success"
		if failed {
			result = "failed"
		}
		// 数组下标来自内部枚举，标签只会有三种操作，不使用实际 URL。
		operation := [...]string{"product", "order", "list"}[value.kind]
		generator.metrics.Loadgen.WithLabelValues(operation, result).Inc()
		if value.kind != orderRequest || !timedOut || ctx.Err() != nil {
			return failed
		}
	}
	return true // 两次下单都超时，逻辑请求仍失败；每次尝试已经分别记入统计。
}

// attempt 只接受 HTTP 2xx 且 code 为 OK 的完整 JSON，避免代理返回 HTML 200 也被计成成功。
func (generator *generator) attempt(parent context.Context, value job) (failed, timedOut bool) {
	path, method, body := "/products/"+strconv.FormatInt(value.skuID, 10), http.MethodGet, ""
	switch value.kind {
	case orderRequest:
		path, method = "/orders", http.MethodPost
		body = fmt.Sprintf(`{"user_id":%d,"sku_id":%d,"quantity":1}`, value.userID, value.skuID)
	case listRequest:
		path = "/orders?user_id=" + strconv.FormatInt(value.userID, 10) + "&page=1&page_size=20"
	}
	ctx, cancel := context.WithTimeout(parent, generator.settings.RequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, generator.settings.Target+path, strings.NewReader(body))
	if err != nil {
		return true, false
	}
	if value.kind == orderRequest {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", value.key)
		request.GetBody = nil // 写请求的重试统一由 execute 计数，不让 Transport 自动重放正文。
	}
	response, err := generator.client.Do(request)
	if err != nil {
		return true, timeoutError(err)
	}
	defer response.Body.Close()
	// 订单列表最大一百条，这里一 MiB 足够。异常服务器不能让生成器无限读入响应。
	const responseLimit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return true, timeoutError(err)
	}
	if len(data) > responseLimit {
		return true, false
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return true, response.StatusCode == http.StatusGatewayTimeout
	}
	var envelope struct {
		Code string `json:"code"`
	} // 只校验通用成功字段，业务内容由服务测试负责。
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Code != "OK" {
		return true, false
	}
	return false, false
}

// timeoutError 不把所有网络故障都当成超时，连接拒绝和普通业务错误不在重试范围内。
func timeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
