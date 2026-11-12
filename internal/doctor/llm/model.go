// 本文件：约定 Agent 与模型客户端之间的请求、回复和用量。
// 并行工具靠调用 ID 配对回复。供本机审计的诊断编号和预算不随对话发给供应商。

package llm

import (
	"context"
	"encoding/json"
)

// Client 同时由真实 HTTP 客户端和脚本客户端实现；取消由同一个 ctx 一路传递。
type Client interface {
	Complete(context.Context, Request) (Response, error)
}

// Function 是模型想执行的函数；Arguments 是 JSON 字符串，仍需工具自己校验。
type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall 的 ID 只用于对话关联，不能当成数据库步骤号。
type ToolCall struct {
	ID       string   `json:"id"`   // 供应商生成的调用编号。
	Type     string   `json:"type"` // 本项目只接受 function。
	Function Function `json:"function"`
}

// Message 使用非流式对话需要的字段，页面的实时更新由步骤事件提供。
type Message struct {
	Role       string     `json:"role"`    // system、user、assistant、tool。
	Content    string     `json:"content"` // 发送前必须脱敏。
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool 回复所对应的 ID。
}

// Tool 把注册表的参数说明包在 OpenAI 兼容协议要求的 function 中。
type Tool struct {
	Type     string         `json:"type"`     // 固定 function。
	Function ToolDefinition `json:"function"` // 不含执行函数或连接地址。
}

// ToolDefinition 把工具参数按原有 Schema 发给模型，报告参数也可包含嵌套对象。
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request 的诊断 ID 只交给本机审计，不进入外部请求或指标标签。
type Request struct {
	CallID               string    `json:"-"` // 一次逻辑请求的本机编号；重试共用，恢复时据此找回已收到的回复。
	DiagnosisID          string    `json:"-"` // 费用归属，不能由模型指定。
	InputUpperBound      int64     `json:"-"` // 已计量历史加新增文本的保守上界，只在本机使用。
	DiagnosisLimitMicros int64     `json:"-"` // 由 Agent 固定，供应商和模型都不能修改。
	Model                string    `json:"model"`
	Messages             []Message `json:"messages"` // 固定前缀加逐步追加的历史。
	Tools                []Tool    `json:"tools"`
	MaxTokens            int       `json:"max_tokens"` // 限制单次输出，费用预留也使用同一值。
	Stream               bool      `json:"stream"`     // 使用非流式回复，实时进度由步骤事件提供。
	Thinking             Thinking  `json:"thinking"`   // 显式关闭默认可能开启的思考模式。
}

// Thinking 是 DeepSeek 的模式字段；本项目真实调用固定 disabled。
type Thinking struct {
	Type string `json:"type"`
}

// Usage 保留供应商的计量；命中的输入用较低价格结算，不能重复计入未命中部分。
type Usage struct {
	CacheWrite int64 `json:"cache_write_tokens"` // 写入缓存的输入，已包含在 Input 中，不能再加一次。
	Reasoning  int64 `json:"reasoning_tokens"`   // 输出中用于推理的部分，已包含在 Output 中。
	Input      int64 `json:"prompt_tokens"`
	Output     int64 `json:"completion_tokens"`
	CacheHit   int64 `json:"prompt_cache_hit_tokens"`
	CacheMiss  int64 `json:"prompt_cache_miss_tokens"` // 供应商提供时保留。
}

// Add 分别累计供应商的六项计量，缓存和推理的子项已经包含在输入输出中。
func (usage *Usage) Add(next Usage) {
	usage.Input += next.Input
	usage.Output += next.Output
	usage.CacheHit += next.CacheHit
	usage.CacheWrite += next.CacheWrite
	usage.Reasoning += next.Reasoning
	usage.CacheMiss += next.CacheMiss
}

// Response 即使伴随 error 也可能已有费用，调用方必须先记账再判断是否继续。
type Response struct {
	UsageKnown bool     `json:"usage_known"` // 明确计量后，即使回复因长度截断而不可用，也按实际费用结算。
	Model      string   `json:"model"`       // 优先使用接口实际返回值。
	Message    Message  `json:"message"`     // 下一轮可直接追加的 assistant 消息。
	Usage      Usage    `json:"usage"`
	CostMicros int64    `json:"cost_micros"` // 含失败尝试的结算费用及未知预留额，单位为账户币种的百万分之一。
	AttemptIDs []string `json:"attempt_ids"` // 指向本机逐次 HTTP 审计。
}
