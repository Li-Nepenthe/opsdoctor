// 本文件：把一轮模型请求发送出去，并在发送前后留下费用和内容审计。
//
// 账本不可用时不能先发请求再补记录，否则一次重启就可能丢掉花费。请求超时后
// 也不能假设供应商没有计费；没有用量的请求保留预留额，等待账单人工核对。
//
// 主要内容：New 校验配置；Complete 控制重试；attempt 完成一次发送；settle 保存结果。
// 阅读建议：先看 Reserve 在 HTTP.Do 之前，再看取消后如何给记账单独的短预算。

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Config 的 Key 只能来自环境变量，调用者不能把整个配置写入日志。
type Config struct {
	PricesAt func(time.Time) llm.Prices // 可选时段价格；每次发送前固定，OpenAI 不设置。
	Format   string                     // chat_completions 或 responses，由部署配置选择，不由模型决定。
	BaseURL  string                     // 固定供应商根地址，工具参数不能覆盖。
	Key      string                     // 仅用于 Authorization 请求头。
	Timeout  time.Duration              // 一轮逻辑调用的总预算，重试也在其中。
	Prices   llm.Prices                 // 本批请求生效的缓存命中、未命中与输出单价。
}

// Client 的依赖在启动时固定，账本与脱敏器由 app 注入。
type Client struct {
	config   Config        // 含 Key，禁止整值打印。
	http     *http.Client  // 禁止重定向，避免鉴权头被送往另一地址。
	ledger   llm.Ledger    // 真实运行时为 MySQL；测试用内存实现。
	redactor *llm.Redactor // HTTP 与审计使用相同的清理结果。
	logger   *slog.Logger  // 只写状态、耗时和用量，不写提示或回复。
}

// errRequestTimeout 区分模型单轮等待超时与用户取消或整份诊断预算到期。
// 评测只能为前者在没有收到响应时安排一次补跑，不能把主动取消伪装成断网。
var errRequestTimeout = errors.New("模型单轮网络等待超时")

// 协议体有上限，既约束内存，也为发送前的费用预留提供输入大小边界。
const (
	maximumRequestBytes = 256 << 10              // 足以容纳本阶段 40 份每份 4KB 的工具结果。
	maximumOutputTokens = 4096                   // 报告和工具参数不需要无限长的回答。
	maximumRetries      = 2                      // 计划书只允许最多两次额外尝试。
	networkRetryDelay   = time.Second            // 网络抖动先等一秒，再等两秒，避免立即重复挤同一条连接。
	auditTimeout        = 500 * time.Millisecond // 取消后短时记账，给两秒退出要求留出余量。
)

// New 允许回环 HTTP 用于 httptest，真实地址必须 HTTPS；不会在构造时联网。
func New(config Config, transport http.RoundTripper, ledger llm.Ledger, redactor *llm.Redactor, logger *slog.Logger) (*Client, error) {
	if config.Format == "" {
		config.Format = "chat_completions"
	}
	if config.Format != "chat_completions" && config.Format != "responses" {
		return nil, fmt.Errorf("LLM_API_FORMAT 只支持 chat_completions 或 responses")
	}
	if config.Format == "responses" && config.Prices.CacheWriteMicros <= 0 {
		return nil, fmt.Errorf("模型 Responses 必须配置缓存写入单价")
	}
	// 先检查目的地，再创建 HTTP 客户端；协议错误不能等到第一次付费调用才发现。
	address, err := url.Parse(config.BaseURL)
	if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return nil, fmt.Errorf("LLM_BASE_URL 必须是无凭据、无查询参数的 HTTP 根地址")
	}
	local := address.Hostname() == "127.0.0.1" || address.Hostname() == "localhost" || address.Hostname() == "::1"
	if address.Scheme != "https" && !(address.Scheme == "http" && local) {
		return nil, fmt.Errorf("真实模型地址必须使用 HTTPS")
	}
	if config.Key == "" || config.Timeout <= 0 || config.Timeout > 2*time.Minute || config.Prices.InputMicros <= 0 || config.Prices.CacheHitMicros <= 0 || config.Prices.CacheHitMicros > config.Prices.InputMicros || config.Prices.OutputMicros <= 0 || ledger == nil || redactor == nil || logger == nil {
		return nil, fmt.Errorf("模型密钥、超时、价格或记账依赖缺失")
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{config: config, http: client, ledger: ledger, redactor: redactor, logger: logger}, nil
}

// Complete 每次重试都重新记账；尚未收到任何响应的网络错误可有限重试，取消立即退出。
func (client *Client) Complete(parent context.Context, request llm.Request) (result llm.Response, err error) {
	// 所有重试共用一个截止时间，不能每次失败都额外获得一分钟。
	bounded, cancel := context.WithTimeoutCause(parent, client.config.Timeout, errRequestTimeout)
	defer cancel()
	request.Thinking.Type = "disabled"
	request.Stream = false
	if request.DiagnosisID == "" || request.Model == "" || request.MaxTokens < 1 || request.MaxTokens > maximumOutputTokens {
		return result, fmt.Errorf("模型请求缺少归属、模型名或合法输出上限")
	}
	raw, err := encodeRequest(request, client.config.Format)
	if err != nil {
		return result, fmt.Errorf("编码模型请求：%w", err)
	}
	raw, err = client.redactor.JSON(raw)
	if err != nil || len(raw) > maximumRequestBytes {
		return result, fmt.Errorf("模型请求无法脱敏或超过 256 KiB")
	}
	for attempt := 0; attempt <= maximumRetries; attempt++ {
		if bounded.Err() != nil {
			return result, bounded.Err()
		}
		answer, retry, delay, callErr := client.attempt(bounded, request, raw, attempt)
		result.CostMicros += answer.CostMicros
		result.Model, result.Message, result.Usage, result.UsageKnown = answer.Model, answer.Message, answer.Usage, answer.UsageKnown
		result.AttemptIDs = append(result.AttemptIDs, answer.AttemptIDs...)
		if callErr == nil {
			result.Model, result.Message, result.Usage, result.UsageKnown = answer.Model, answer.Message, answer.Usage, answer.UsageKnown
			return result, nil
		}
		err = callErr
		if !retry || attempt == maximumRetries {
			return result, err
		}
		if err := wait(bounded, delay); err != nil {
			return result, err
		}
	}
	return result, err
}

// attempt 在发送前记录预留额。输入按 UTF-8 字节加协议余量估计，比按中文字数计 token 更保守。
func (client *Client) attempt(ctx context.Context, request llm.Request, raw []byte, number int) (llm.Response, bool, time.Duration, error) {
	prices := client.config.Prices
	if client.config.PricesAt != nil {
		prices = client.config.PricesAt(time.Now())
	}
	// 回调只能在既定峰时上限内选价，不能临时扩大预算或产生负费用。
	if prices.InputMicros <= 0 || prices.CacheHitMicros <= 0 || prices.CacheHitMicros > prices.InputMicros || prices.OutputMicros <= 0 || prices.InputMicros > client.config.Prices.InputMicros || prices.CacheHitMicros > client.config.Prices.CacheHitMicros || prices.OutputMicros > client.config.Prices.OutputMicros || prices.CacheWriteMicros != client.config.Prices.CacheWriteMicros {
		return llm.Response{}, false, 0, fmt.Errorf("模型时段价格超出已配置范围")
	}
	const protocolTokenMargin = 1024 // 覆盖消息分隔等供应商协议 token，正文按每字节一个 token 预留。
	inputBound := int64(len(raw) + protocolTokenMargin)
	// Agent 已有前一轮的真实 token 数时，只对新增部分按字节预留，避免把历史再次高估。
	if request.InputUpperBound > 0 {
		inputBound = min(inputBound, request.InputUpperBound)
	}
	reserved := prices.ReserveCost(inputBound, int64(request.MaxTokens))
	id, err := client.ledger.Reserve(ctx, llm.Reservation{MaximumTokens: inputBound + int64(request.MaxTokens), CallID: request.CallID, DiagnosisID: request.DiagnosisID, Model: request.Model, Request: raw, MaximumMicros: reserved, DiagnosisLimitMicros: request.DiagnosisLimitMicros, Prices: prices})
	if err != nil {
		return llm.Response{}, false, 0, err
	}
	result := llm.Response{Model: request.Model, CostMicros: reserved, AttemptIDs: []string{id}}
	path := "/chat/completions"
	if client.config.Format == "responses" {
		path = "/responses"
	}
	// Do 返回错误并不等于对面什么都没发过：响应头只传到一半也会报错。
	// 记录第一个响应字节，严格限制新增的网络重试只发生在“尚未收到响应”时。
	// 回调可能由 Transport 的 goroutine 调用，所以用 atomic.Bool 避免读写竞争。
	var received atomic.Bool
	traced := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotFirstResponseByte: func() { received.Store(true) }})
	httpRequest, err := http.NewRequestWithContext(traced, http.MethodPost, client.config.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		result.CostMicros = 0
		return result, false, 0, client.settle(ctx, id, llm.Settlement{Status: "invalid_request"}, err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.config.Key)
	httpRequest.Header.Set("Content-Type", "application/json")
	started := time.Now()
	client.logger.Info("开始模型请求", "diagnosis_id", request.DiagnosisID, "attempt_id", id, "model", request.Model)
	response, err := client.http.Do(httpRequest)
	if err != nil {
		status, problem := "network_error", error(fmt.Errorf("模型网络请求未完成，费用待核对"))
		if ctx.Err() != nil {
			status, problem = "cancelled", ctx.Err()
			if errors.Is(context.Cause(ctx), errRequestTimeout) && !received.Load() {
				status = "network_timeout" // 没收到任何响应；未知费用仍保留，不在已到期的一轮内再重试。
			}
		}
		client.logger.Info("模型请求已中断", "diagnosis_id", request.DiagnosisID, "attempt_id", id, "reason", status, "duration_ms", time.Since(started).Milliseconds())
		settled := client.settle(ctx, id, llm.Settlement{Unknown: true, Status: status}, problem)
		// 没拿到响应也可能已经生成过 token，因此保留旧预留额。下一次 attempt
		// 必须重新 Reserve；审计保存失败、请求取消或已收到响应时都不继续花钱。
		retry := settled == problem && ctx.Err() == nil && !received.Load()
		return result, retry, networkRetryDelay << number, settled
	}
	defer response.Body.Close()
	answer, body, retry, delay, problem := decodeResponse(response, request.Model, client.redactor)
	// 请求头已经收到也可能仍在等正文。取消这段等待同样要返回 context 的原因。
	if ctx.Err() != nil {
		client.logger.Info("模型请求已中断", "diagnosis_id", request.DiagnosisID, "attempt_id", id, "reason", "cancelled", "duration_ms", time.Since(started).Milliseconds())
		return result, false, 0, client.settle(ctx, id, llm.Settlement{Response: body, Unknown: true, Status: "cancelled"}, ctx.Err())
	}
	settlement := llm.Settlement{Response: body, Usage: answer.Usage, Status: "ok"}
	if problem != nil {
		settlement.Status = fmt.Sprintf("http_%d", response.StatusCode)
		// 明确被拒绝的请求没有生成 token。5xx 或损坏的成功回复不能这样假定。
		settlement.Unknown = !answer.UsageKnown && (response.StatusCode >= 500 || response.StatusCode < 400)
		if answer.UsageKnown {
			settlement.CostMicros = prices.Charge(answer.Usage)
		}
	} else {
		settlement.CostMicros = prices.Charge(answer.Usage)
	}
	if !settlement.Unknown {
		result.CostMicros = settlement.CostMicros
	}
	result.Model, result.Message, result.Usage, result.UsageKnown = answer.Model, answer.Message, answer.Usage, answer.UsageKnown
	if auditErr := client.settle(ctx, id, settlement, nil); auditErr != nil {
		return result, false, 0, auditErr
	}
	client.logger.Info("模型请求结束", "diagnosis_id", request.DiagnosisID, "attempt_id", id, "status", settlement.Status, "duration_ms", time.Since(started).Milliseconds(), "tokens_in", answer.Usage.Input, "tokens_out", answer.Usage.Output)
	return result, retry, delay, problem
}

// settle 保留取消信号中的追踪信息，只给记账半秒；失败时预留额已经在库中，不会丢账。
func (client *Client) settle(parent context.Context, id string, value llm.Settlement, problem error) error {
	bounded, cancel := context.WithTimeout(context.WithoutCancel(parent), auditTimeout)
	defer cancel()
	if len(value.Response) == 0 {
		value.Response = json.RawMessage(`null`)
	}
	if err := client.ledger.Settle(bounded, id, value); err != nil {
		return errors.Join(problem, fmt.Errorf("模型审计结算失败，保留预留费用：%w", err))
	}
	return problem
}
