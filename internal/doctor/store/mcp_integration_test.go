// 本文件：在医生专用测试库核对 MCP 审计不是只写在日志里。
//
// 外部客户端即使没有诊断任务，也要留下独立的调用记录。这里先保存开始，再保存
// 结果，并确认重复结束不能改掉第一次结果。测试只清理自己生成的审计编号。
//
// 主要内容：TestIntegrationMCPAudit 检查持久化、脱敏与结束条件。
// 阅读建议：SQL 只查随机主键，不读取真实诊断或其他客户端的记录。

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/mcpserver"
)

// TestIntegrationMCPAudit 保存时先 running，再变 ok；重放失败结果不能覆盖已经完成的审计。
func TestIntegrationMCPAudit(t *testing.T) {
	repository, target := integrationStore(t)
	id, err := repository.BeginMCP(t.Context(), mcpserver.Entry{Target: target, Tool: "service_topology", Arguments: `{"password":"demo-audit-password"}`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = repository.database.ExecContext(ctx, "DELETE FROM mcp_audit WHERE id=?", id)
	})
	var status, args string
	if err = repository.database.QueryRowContext(t.Context(), "SELECT status,arguments_summary FROM mcp_audit WHERE id=?", id).Scan(&status, &args); err != nil || status != "running" || strings.Contains(args, "demo-audit-password") {
		t.Fatal("调用开始未保存或参数未脱敏")
	}
	// 条件更新保证结果只写一次；读工具失败也不能把已成功审计改成另一份结果。
	if err = repository.EndMCP(t.Context(), id, "ok"); err != nil {
		t.Fatal(err)
	}
	if err = repository.EndMCP(t.Context(), id, "tool_error"); err != nil {
		t.Fatal(err)
	}
	if err = repository.database.QueryRowContext(t.Context(), "SELECT status FROM mcp_audit WHERE id=?", id).Scan(&status); err != nil || status != "ok" {
		t.Fatal("结束状态被重放覆盖")
	}
}
