// 本文件：把模型 HTTP 回复转换为对话消息，限定可重试的错误。
//
// HTTP 200 也可能带损坏的 JSON，不能当作“零 token 的成功”。供应商错误正文
// 不直接作为用户错误提示，避免代理或服务端把鉴权内容反射进日志。
//
// 主要内容：decodeResponse 检查用量和消息；httpProblem 给出中文原因；wait 可被取消。
// 阅读建议：Retry-After 太长时由外层 deadline 结束等待，不提前违反限流要求。

package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// decodeResponse 限制回复大小，保留接口 model；缺少该字段时才用请求模型。
func decodeResponse(response *http.Response, fallback string, redactor *llm.Redactor) (llm.Response, []byte, bool, time.Duration, error) {
	const maximumResponseBytes = 1 << 20 // 非流式回复最多 1 MiB，超过说明接口行为不符合本项目预算。
	result := llm.Response{Model: fallback}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(raw) > maximumResponseBytes {
		return result, nil, false, 0, fmt.Errorf("模型回复读取失败或过大，费用待核对")
	}
	clean, cleanErr := redactor.JSON(raw)
	if response.StatusCode != http.StatusOK {
		problem, retry := httpProblem(response.StatusCode)
		// 不存非法 JSON；其中可能是反向代理的 HTML 错误页。
		if cleanErr != nil {
			clean = []byte(`null`)
		}
		return result, clean, retry, retryAfter(response.Header.Get("Retry-After")), problem
	}
	if cleanErr != nil {
		return result, clean, false, 0, fmt.Errorf("模型回复不是合法 JSON，费用待核对")
	}
	result, err = DecodeSavedResponse(clean, fallback)
	return result, clean, false, 0, err
}

// httpProblem 的白名单不包含网络错误，避免无法确认是否计费时盲目重复生成。
func httpProblem(status int) (error, bool) {
	switch status {
	case 400:
		return fmt.Errorf("模型请求参数错误（400），请检查模型和工具对话格式"), false
	case 401:
		return fmt.Errorf("模型鉴权失败（401），请检查本机 LLM_API_KEY"), false
	case 402:
		return fmt.Errorf("模型账户余额不足（402），本次诊断停止"), false
	case 429:
		return fmt.Errorf("模型请求被限流（429）"), true
	case 500, 502, 503, 504:
		return fmt.Errorf("模型服务暂时不可用（%d）", status), true
	default:
		return fmt.Errorf("模型接口返回 HTTP %d，不自动重试", status), false
	}
}

// retryAfter 支持秒数和 HTTP 日期；未提供时等待一秒，避免立即重试形成突发。
func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		// 超过一天的等待仍远大于两分钟总预算；封顶防止 duration 溢出，绝不提前重试。
		return time.Duration(min(seconds, 86400)) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(when))
	}
	return time.Second
}

// wait 用 select 同时等待计时器和取消，不使用无法提前结束的 Sleep。
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
