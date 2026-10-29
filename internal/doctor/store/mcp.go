// 本文件：保存 MCP 客户端调用工具的审计，不把外部调用伪装成医生诊断。
//
// 外部客户端没有 diagnosis_id，所以单独记时间、绑定系统、工具名和参数摘要。
// 先写 running，再更新结果；进程崩溃留下的 running 表示结果未知，不能假装未调用。
//
// 主要内容：BeginMCP 保存调用开始；EndMCP 仅结束仍在运行的记录。
// 阅读建议：参数已经在入口脱敏，这里再次处理，防止以后出现漏掉入口清理的调用方。

package store

import (
	"context"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/mcpserver"
)

// BeginMCP 保存成功后才返回审计 ID，调用方此时才能访问目标系统。
func (store *Store) BeginMCP(ctx context.Context, entry mcpserver.Entry) (string, error) {
	if entry.Target == "" || len(entry.Target) > 32 || len(entry.Tool) > 128 || len(entry.Arguments) > 1024 {
		return "", fmt.Errorf("MCP 审计字段超出范围")
	}
	id := newID()
	// 主键随机生成，结果更新走主键；系统时间索引用于后续按系统查阅，不记录凭据。
	_, err := store.database.ExecContext(ctx, "INSERT INTO mcp_audit(id,target,tool_name,arguments_summary,created_at,status) VALUES(?,?,?,?,?,'running')", id, entry.Target, entry.Tool, store.config.Redactor.Text(entry.Arguments), store.config.Now().UTC())
	return id, err
}

// EndMCP 只接受有限结果分类，错误正文不能借审计状态字段泄露后端信息。
func (store *Store) EndMCP(ctx context.Context, id, status string) error {
	switch status {
	case "ok", "tool_error", "protocol_error", "cancelled":
	default:
		return fmt.Errorf("MCP 审计结果不合法")
	}
	_, err := store.database.ExecContext(ctx, "UPDATE mcp_audit SET status=?,finished_at=? WHERE id=? AND status='running'", status, store.config.Now().UTC(), id)
	return err
}
