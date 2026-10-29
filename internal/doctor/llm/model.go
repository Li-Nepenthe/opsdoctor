// 本文件：描述一轮模型对话的请求、回复与工具调用关联。
//
// agent 只依赖这些类型，不认识 HTTP 或某一家供应商。工具结果里的 ToolCallID
// 指回模型提出的那次调用；一轮并行调用多个工具时，也能分清每份结果属于谁。
//
// 主要内容：Client 是使用方需要的能力；Request、Response 保存对话；Usage 保存计费依据。
// 阅读建议：先把 assistant 的 ToolCalls 和下一条 tool 消息的 ToolCallID 对起来。

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
	Name      string `json:"name"`      // 固定注册名。
	Arguments string `json:"arguments"` // 尚未信任的参数。
}

// ToolCall 的 ID 只用于对话关联，不能当成数据库步骤号。
type ToolCall struct {
	ID       string   `json:"id"`       // 供应商生成的调用编号。
	Type     string   `json:"type"`     // 本项目只接受 function。
	Function Function `json:"function"` // 请求执行的工具。
}

// Message 使用非流式对话需要的字段，页面的实时更新由步骤事件提供。
type Message struct {
	Role       string     `json:"role"`                   // system、user、assistant、tool。
	Content    string     `json:"content"`                // 发送前必须脱敏。
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant 提出的请求。
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool 回复所对应的 ID。
}

// Tool 把注册表的参数说明包在 OpenAI 兼容协议要求的 function 中。
type Tool struct {
	Type     string         `json:"type"`     // 固定 function。
	Function ToolDefinition `json:"function"` // 不含执行函数或连接地址。
}

// ToolDefinition 支持嵌套报告 Schema；普通查看工具仍沿用阶段 5 的 Schema。
type ToolDefinition struct {
	Name        string          `json:"name"`        // 精确工具名。
	Description string          `json:"description"` // 给模型看的使用说明。
	Parameters  json.RawMessage `json:"parameters"`  // 代码生成的 JSON Schema。
}

// Request 的诊断 ID 只交给本机审计，不进入外部请求或指标标签。
type Request struct {
	CallID               string    `json:"-"`          // 一次逻辑请求的本机编号；重试共用，恢复时据此找回已收到的回复。
	DiagnosisID          string    `json:"-"`          // 费用归属，不能由模型指定。
	InputUpperBound      int64     `json:"-"`          // 已计量历史加新增文本的保守上界，只在本机使用。
	DiagnosisLimitMicros int64     `json:"-"`          // 由 Agent 固定，供应商和模型都不能修改。
	Model                string    `json:"model"`      // 开始时选择的模型。
	Messages             []Message `json:"messages"`   // 固定前缀加逐步追加的历史。
	Tools                []Tool    `json:"tools"`      // 只来自本系统的注册表。
	MaxTokens            int       `json:"max_tokens"` // 限制单次输出，费用预留也使用同一值。
	Stream               bool      `json:"stream"`     // 本阶段固定 false。
	Thinking             Thinking  `json:"thinking"`   // 显式关闭默认可能开启的思考模式。
}

// Thinking 是 DeepSeek 的模式字段；本项目真实调用固定 disabled。
type Thinking struct {
	Type string `json:"type"`
}

// Usage 保留供应商的计量；命中的输入用较低价格结算，不能重复计入未命中部分。
type Usage struct {
	CacheWrite int64 `json:"cache_write_tokens"`       // 写入缓存的输入，已包含在 Input 中，不能再加一次。
	Reasoning  int64 `json:"reasoning_tokens"`         // 输出中用于推理的部分，已包含在 Output 中。
	Input      int64 `json:"prompt_tokens"`            // 本次完整输入 token 数。
	Output     int64 `json:"completion_tokens"`        // 本次输出 token 数。
	CacheHit   int64 `json:"prompt_cache_hit_tokens"`  // 输入中命中缓存的部分。
	CacheMiss  int64 `json:"prompt_cache_miss_tokens"` // 供应商提供时保留。
}

// Response 即使伴随 error 也可能已有费用，调用方必须先记账再判断是否继续。
type Response struct {
	UsageKnown bool     `json:"usage_known"` // 明确计量后，即使回复因长度截断而不可用，也按实际费用结算。
	Model      string   `json:"model"`       // 优先使用接口实际返回值。
	Message    Message  `json:"message"`     // 下一轮可直接追加的 assistant 消息。
	Usage      Usage    `json:"usage"`       // 成功回答的用量。
	CostMicros int64    `json:"cost_micros"` // 含失败尝试的结算费用及未知预留额，单位百万分之一元。
	AttemptIDs []string `json:"attempt_ids"` // 指向本机逐次 HTTP 审计。
}
