// 本文件：让来自外部客户端的工具调用也留下可追查记录。
//
// 只读不等于可以没有审计。先保存“开始调用”，再查目标系统；如果进程中途退出，
// 库里仍有未完成记录。参数只保留短摘要，不能让查询内容或秘密无限堆进审计表。
//
// 主要内容：Entry 描述一次调用；Auditor 约定保存接口；auditCalls 包住 SDK 工具分发。
// 阅读建议：未知工具也经过中间件，不能因为菜单查找失败就漏掉调用记录。

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
	Target    string // 创建入口时绑定的系统。
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
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, request)
			}
			params, ok := request.GetParams().(*mcp.CallToolParamsRaw)
			if !ok {
				return nil, fmt.Errorf("MCP 工具参数格式无法读取")
			}
			// 十秒仍由注册表负责；外层额外一秒只给审计，不能无限占用处理器。
			bounded, cancel := context.WithTimeout(ctx, 11*time.Second)
			defer cancel()
			entry := Entry{Target: target, Tool: short(redactor.Text(params.Name), 128), Arguments: short(redactor.Text(string(params.Arguments)), 1024)}
			id, err := auditor.BeginMCP(bounded, entry)
			if err != nil {
				return nil, fmt.Errorf("无法保存 MCP 调用审计，未执行工具")
			}
			result, callErr := next(bounded, method, request)
			status := "ok"
			if callErr != nil {
				status = "protocol_error"
			} else if value, ok := result.(*mcp.CallToolResult); ok && value.IsError {
				status = "tool_error"
			}
			if bounded.Err() != nil {
				status = "cancelled"
			}
			// 客户端断开后仍留半秒写结果；不会用这个 context 再访问目标系统。
			save, stop := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
			defer stop()
			if err := auditor.EndMCP(save, id, status); err != nil {
				return nil, fmt.Errorf("MCP 查询已结束，但保存审计结果失败")
			}
			return result, callErr
		}
	}
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
