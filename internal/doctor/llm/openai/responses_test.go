// 本文件：验证模型回复的工具往返、计费与历史兼容。
//
// 端点和模型名看起来像真实 API，但 Transport 完全由测试接管。测试不会读取本机 Key，
// 也不会联网。发送内容必须与预留审计完全相同。
//
// 主要内容：TestResponsesRoundtrip 检查两轮 call_id；TestResponsesUsage 检查计量边界。
// TestSavedResponseTypes 拒绝类型被写错的回复；TestSavedDeepSeekResponse 固定真实审计样本的恢复结果。
// 阅读建议：调用 ID 不是步骤号，必须原样传回；缓存写入不是额外一份输入 token。

package openai

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestSavedResponseTypes 不允许协议适配悄悄改写用量或工具参数。
func TestSavedResponseTypes(t *testing.T) {
	const chat = `{"choices":[{"message":{"role":"assistant","content":"ok","tool_calls":[{"id":"c1","type":"function","function":{"name":"inspect","arguments":"{}"}}]}}],"usage":{"prompt_tokens":20,"completion_tokens":2}}`
	const responses = `{"status":"completed","output":[{"type":"function_call","call_id":"c1","name":"inspect","arguments":"{}"}],"usage":{"input_tokens":20,"output_tokens":2}}`
	// 先证明基底有效，下面每个反例只改变一种字段类型。
	for _, body := range []string{chat, responses} {
		if _, err := DecodeSavedResponse([]byte(body), "fallback"); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ name, body string }{
		{"chat_string_token", strings.Replace(chat, `"prompt_tokens":20`, `"prompt_tokens":"20"`, 1)},
		{"chat_fraction_token", strings.Replace(chat, `"prompt_tokens":20`, `"prompt_tokens":20.5`, 1)},
		{"chat_object_content", strings.Replace(chat, `"content":"ok"`, `"content":{}`, 1)},
		{"chat_object_arguments", strings.Replace(chat, `"arguments":"{}"`, `"arguments":{}`, 1)},
		{"responses_string_token", strings.Replace(responses, `"input_tokens":20`, `"input_tokens":"20"`, 1)},
		{"responses_object_arguments", strings.Replace(responses, `"arguments":"{}"`, `"arguments":{}`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if answer, err := DecodeSavedResponse([]byte(test.body), "fallback"); err == nil || answer.UsageKnown {
				t.Fatalf("错误类型被当作有效回复：known=%v err=%v", answer.UsageKnown, err)
			}
		})
	}
}

// TestSavedDeepSeekResponse 使用阶段 9 P9-53 的已脱敏审计；错误的参数应留给工具校验，不能在恢复时修补。
func TestSavedDeepSeekResponse(t *testing.T) {
	raw, err := os.ReadFile("testdata/deepseek-p9-53-response.json")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "a24dcfa91ad34410363662247ee7c1d38ab01ba06d76b49a3324512c2808419d" {
		t.Fatal("历史样本已改变，不能作为同一份审计回归")
	}
	answer, err := DecodeSavedResponse(raw, "fallback")
	want := llm.Usage{Input: 13808, Output: 466, CacheHit: 768, CacheMiss: 13040}
	if err != nil || !answer.UsageKnown || answer.Model != "deepseek-flash" || answer.Usage != want || len(answer.Message.ToolCalls) != 1 {
		t.Fatalf("历史回复恢复不符：%+v err=%v", answer, err)
	}
	// 同时约束原始字符串和非法状态，避免“修好 JSON”后让当年的失败评分变成成功。
	arguments := []byte(answer.Message.ToolCalls[0].Function.Arguments)
	if json.Valid(arguments) || fmt.Sprintf("%x", sha256.Sum256(arguments)) != "28b92e01563cf90a8aa7d61ee8503f18dffce7bd47dbb8dc9bb97bc95825c825" {
		t.Fatal("历史工具参数被修补或改写")
	}
}

// TestResponsesRoundtrip 把模型第一轮的函数调用接到第二轮输入，确认没有丢失 ID 或增加托管工具。
func TestResponsesRoundtrip(t *testing.T) {
	// 运输层完全替换成测试函数，下面两次调用都不会联网或产生费用。
	ledger := &memoryLedger{}
	count := 0
	transport := transportFunc(func(req *http.Request) (*http.Response, error) {
		count++
		raw, _ := io.ReadAll(req.Body)
		if req.URL.Path != "/v1/responses" || string(raw) != string(ledger.reservations[count-1].Request) || strings.Contains(string(raw), "sk-demo") {
			t.Fatal("端点或脱敏审计不一致")
		}
		var body struct {
			Store     bool             `json:"store"`
			Input     []map[string]any `json:"input"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Tools []map[string]any `json:"tools"`
		}
		if json.Unmarshal(raw, &body) != nil || body.Store || body.Reasoning.Effort != "none" || body.Input[0]["role"] != "developer" || body.Tools[0]["type"] != "function" {
			t.Fatal("请求扩大了模型权限或丢失固定前缀")
		}
		if count == 2 {
			call, output := body.Input[len(body.Input)-2], body.Input[len(body.Input)-1]
			if call["type"] != "function_call" || output["type"] != "function_call_output" || call["call_id"] != "supplier-42" || output["call_id"] != "supplier-42" {
				t.Fatal("工具往返关联丢失")
			}
		}
		return reply(200, `{"model":"gpt-6.1-terra","status":"completed","output":[{"type":"function_call","call_id":"supplier-42","name":"service_topology","arguments":"{}"}],"usage":{"input_tokens":12000,"output_tokens":10,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":7000,"ttl_12h_tokens":0}},"output_tokens_details":{"reasoning_tokens":0}}}`), nil
	})
	prices := llm.Prices{InputMicros: 1_000_000, CacheHitMicros: 50_000, CacheWriteMicros: 1_250_000, OutputMicros: 5_000_000}
	client, err := New(Config{Format: "responses", BaseURL: "https://api.openai.com/v1", Key: "sk-demo123456789", Timeout: time.Minute, Prices: prices}, transport, ledger, llm.NewRedactor(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := requestForTest()
	request.Model = "gpt-6.1-terra"
	request.Messages = []llm.Message{{Role: "system", Content: "key sk-demo123456789"}, {Role: "user", Content: "查看架构"}}
	request.Tools = []llm.Tool{{Type: "function", Function: llm.ToolDefinition{Name: "service_topology", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}}
	first, err := client.Complete(t.Context(), request)
	if err != nil || first.CostMicros != 10000 || first.Usage.CacheWrite != 7000 || first.Message.ToolCalls[0].ID != "supplier-42" {
		t.Fatalf("结果=%+v err=%v", first, err)
	}
	request.Messages = append(request.Messages, first.Message, llm.Message{Role: "tool", ToolCallID: first.Message.ToolCalls[0].ID, Content: `{"step":7,"untrusted_data":"架构"}`})
	if _, err = client.Complete(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(ledger.settlements) != 2 {
		t.Fatal("往返数量不符")
	}
	if got := prices.ReserveCost(1000, 100); got != 1750 {
		t.Fatalf("没有按最高缓存写入价预留：%d", got)
	}
}

// TestResponsesUsage 计量矛盾不能当零费用；合法但截断的回复仍保留已知用量。
func TestResponsesUsage(t *testing.T) {
	for _, test := range []struct {
		name, body string
		known      bool
	}{
		{"截断仍有实际计量", `{"status":"incomplete","output":[],"usage":{"input_tokens":10,"output_tokens":4096,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}}`, true},
		{"命中与写入不能超过总量", `{"status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":8,"cache_write_tokens":8}}}`, false},
		{"未请求十二小时价格", `{"status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":10,"cache_write_tokens_details":{"ttl_12h_tokens":10}}}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			answer, err := DecodeSavedResponse([]byte(test.body), "gpt-6.1-terra")
			if err == nil || answer.UsageKnown != test.known {
				t.Fatalf("known=%v err=%v", answer.UsageKnown, err)
			}
		})
	}
}
