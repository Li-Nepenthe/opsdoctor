// 本文件：每次工具调用先登记再分派，即使进程中断也留有开始记录。
// auditCalls 保存脱敏后的有限长度摘要，并为结束状态单独留出写入时间。

package mcpserver

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Entry 的系统来自固定入口，客户端只提供工具名和参数，二者都尚未被信任。
type Entry struct {
	Target    string
	Tool      string // 请求里的工具名，最多 128 字节。
	Arguments string // 脱敏后的参数摘要，最多 1024 字节。
}

// Auditor 把开始与结束分开，进程崩溃也不会完全抹掉这次外部调用。
type Auditor interface {
	BeginMCP(context.Context, Entry) (string, error)
	EndMCP(context.Context, string, string) error
}

// auditCalls 包住工具分发，因此非法参数、未知工具与查询失败也都会进入审计。
func auditCalls(target string, auditor Auditor, redactor *llm.Redactor) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		call := auditHandler{target: target, auditor: auditor, redactor: redactor, next: next}
		return call.handle
	}
}

type auditHandler struct {
	target   string
	auditor  Auditor
	redactor *llm.Redactor
	next     mcp.MethodHandler
}

func (call auditHandler) handle(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
	if method != "tools/call" {
		return call.next(ctx, method, request)
	}
	params, ok := request.GetParams().(*mcp.CallToolParamsRaw)
	if !ok {
		return nil, fmt.Errorf("MCP 工具参数格式无法读取")
	}
	// 注册表仍限制工具执行时间，外层额外一秒供审计使用。
	bounded, cancel := context.WithTimeout(ctx, 11*time.Second)
	defer cancel()
	entry := Entry{
		Target:    call.target,
		Tool:      short(call.redactor.Text(params.Name), 128),
		Arguments: short(call.redactor.Text(string(params.Arguments)), 1024),
	}
	id, err := call.auditor.BeginMCP(bounded, entry)
	if err != nil {
		return nil, fmt.Errorf("无法保存 MCP 调用审计，未执行工具")
	}
	result, callErr := call.next(bounded, method, request)
	status := auditStatus(bounded, result, callErr)
	// 请求被取消后仍留半秒写结束状态，这个预算只用于审计。
	save, stop := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer stop()
	if err := call.auditor.EndMCP(save, id, status); err != nil {
		return nil, fmt.Errorf("MCP 查询已结束，但保存审计结果失败")
	}
	return result, callErr
}

// 调用超时或取消时以 cancelled 记录，即使工具已返回其他错误。
func auditStatus(ctx context.Context, result mcp.Result, callErr error) string {
	status := "ok"
	if callErr != nil {
		status = "protocol_error"
	} else if value, ok := result.(*mcp.CallToolResult); ok && value.IsError {
		status = "tool_error"
	}
	if ctx.Err() != nil {
		status = "cancelled"
	}
	return status
}

// short 按 UTF-8 边界截断，数据库不能收到半个汉字。
func short(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	value := text[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
