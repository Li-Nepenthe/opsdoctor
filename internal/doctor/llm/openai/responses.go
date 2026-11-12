// 本文件：转换模型工具调用和 Responses 条目，按供应商的 call_id 关联工具结果。断点恢复读取本地记录，不依赖供应商保留对话。

package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// encodeRequest 使用非思考模式和标准价格档，不提供网络搜索或供应商托管工具。
func encodeRequest(request llm.Request, format string) ([]byte, error) {
	if format == "chat_completions" {
		return json.Marshal(request)
	}
	items := []any{}
	for _, message := range request.Messages {
		switch message.Role {
		case "tool":
			items = append(items, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content})
		case "assistant":
			if message.Content != "" {
				items = append(items, map[string]any{"role": "assistant", "content": message.Content})
			}
			for _, call := range message.ToolCalls {
				items = append(items, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
			}
		case "system", "user":
			role := message.Role
			if role == "system" {
				role = "developer"
			}
			items = append(items, map[string]any{"role": role, "content": message.Content})
		default:
			return nil, fmt.Errorf("不支持的对话角色")
		}
	}
	definitions := []any{}
	for _, tool := range request.Tools {
		// 保留项目自己的校验规则。strict=false 避免 API 擅自把可选参数都改成必填。
		definitions = append(definitions, map[string]any{"type": "function", "name": tool.Function.Name, "description": tool.Function.Description, "parameters": tool.Function.Parameters, "strict": false})
	}
	return json.Marshal(map[string]any{"model": request.Model, "input": items, "tools": definitions, "max_output_tokens": request.MaxTokens, "store": false, "stream": false, "reasoning": map[string]string{"effort": "none"}, "service_tier": "default", "prompt_cache_options": map[string]string{"mode": "implicit", "ttl": "30m"}})
}

// DecodeSavedResponse 同时用于收到 HTTP 回复和恢复已保存的回复，避免两套解析规则产生不同工具调用。
func DecodeSavedResponse(raw []byte, fallback string) (llm.Response, error) {
	// 由已保存的结构识别协议，换模型后仍能读懂上一轮回答。
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return llm.Response{}, fmt.Errorf("模型回复不是合法 JSON")
	}
	if _, ok := envelope["output"]; ok {
		return decodeResponses(raw, fallback)
	}
	result := llm.Response{Model: fallback}
	var body struct {
		Model   string     `json:"model"`
		Usage   *llm.Usage `json:"usage"`
		Choices []struct {
			Message llm.Message `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Usage == nil || len(body.Choices) != 1 || body.Choices[0].Message.Role != "assistant" {
		return result, fmt.Errorf("模型回复缺少消息或用量，费用待核对")
	}
	if err := validateUsage(*body.Usage); err != nil {
		return result, err
	}
	if body.Model != "" {
		result.Model = body.Model
	}
	result.Message, result.Usage, result.UsageKnown = body.Choices[0].Message, *body.Usage, true
	return result, nil
}

// responsesUsage 保留供应商拆出的命中、写入及推理量，结算时要区分其归属。
type responsesUsage struct {
	Input        int64 `json:"input_tokens"`
	Output       int64 `json:"output_tokens"`
	InputDetails struct {
		Cached  int64 `json:"cached_tokens"`
		Written int64 `json:"cache_write_tokens"`
		Writes  struct {
			Short int64 `json:"ttl_30m_tokens"`
			Long  int64 `json:"ttl_12h_tokens"`
		} `json:"cache_write_tokens_details"`
	} `json:"input_tokens_details"`
	OutputDetails struct {
		Reasoning int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// responseItem 按原顺序携带工具调用或文本，一个回复可以有多个条目。
type responseItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// decodeResponses 先确认费用是否已知，再解析回答。回答不完整时仍带回已确认的用量。
func decodeResponses(raw []byte, fallback string) (llm.Response, error) {
	result := llm.Response{Model: fallback, Message: llm.Message{Role: "assistant"}}
	var body struct {
		Model  string          `json:"model"`
		Status string          `json:"status"`
		Usage  *responsesUsage `json:"usage"`
		Output []responseItem  `json:"output"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Usage == nil {
		return result, fmt.Errorf("模型 Responses 回复缺少用量，费用待核对")
	}
	u := body.Usage
	result.Usage = llm.Usage{Input: u.Input, Output: u.Output, CacheHit: u.InputDetails.Cached, CacheWrite: u.InputDetails.Written, Reasoning: u.OutputDetails.Reasoning, CacheMiss: u.Input - u.InputDetails.Cached}
	if err := validateUsage(result.Usage); err != nil {
		return llm.Response{}, err
	}
	// 请求固定 30m。若供应商返回另一档，不能用较低写入价悄悄结算。
	if u.InputDetails.Writes.Long != 0 || (u.InputDetails.Writes.Short != 0 && u.InputDetails.Writes.Short != u.InputDetails.Written) {
		return llm.Response{}, fmt.Errorf("供应商返回了非预期缓存写入档位，保留预留额并停止")
	}
	result.UsageKnown = true
	if body.Model != "" {
		result.Model = body.Model
	}
	if body.Status != "completed" {
		return result, fmt.Errorf("模型 Responses 未完成完整回答（%s），已记录接口返回的用量", body.Status)
	}
	err := readResponseItems(body.Output, &result.Message)
	return result, err
}

// readResponseItems 保留工具调用的供应商编号。遇到未支持的条目时返回已读到的调用，不把未读完的文本作为完整回答。
func readResponseItems(items []responseItem, message *llm.Message) error {
	var text []string
	for _, item := range items {
		switch item.Type {
		case "function_call":
			message.ToolCalls = append(message.ToolCalls, llm.ToolCall{ID: item.CallID, Type: "function", Function: llm.Function{Name: item.Name, Arguments: item.Arguments}})
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text = append(text, part.Text)
				}
			}
		case "reasoning":
			// 当前明确使用 none；若供应商仍返回推理条目，不丢弃后假装可安全续接。
			return fmt.Errorf("非思考模式返回了推理条目，需核对协议后继续")
		default:
			return fmt.Errorf("模型 Responses 返回了未注册的条目类型")
		}
	}
	message.Content = strings.Join(text, "\n")
	return nil
}

// validateUsage 既防负数，也防把同一批输入同时当成命中和写入来重复抵扣费用。
func validateUsage(usage llm.Usage) error {
	if usage.Input < 0 || usage.Input > 2_000_000 || usage.Output < 0 || usage.Output > maximumOutputTokens || usage.CacheHit < 0 || usage.CacheWrite < 0 || usage.CacheHit+usage.CacheWrite > usage.Input || usage.Reasoning < 0 || usage.Reasoning > usage.Output {
		return fmt.Errorf("模型用量超出协议范围，费用待核对")
	}
	return nil
}
