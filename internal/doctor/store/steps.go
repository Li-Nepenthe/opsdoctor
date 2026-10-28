// 本文件：给步骤分配编号，并为页面断线后的补发提供有序读取。
//
// 模型可能同时请求四个工具。不能用“先数有多少行”来分配下一号，而要锁住诊断行，
// 在同一个事务中递增序号和写步骤。发布通知放在提交以后，页面才能按编号补查。
//
// 主要内容：RecordStep 保存一步；appendStep 是事务内公共写入；StepsAfter 读取增量。
// 阅读建议：并发完成顺序与模型请求顺序可能不同，报告必须引用保存后返回的编号。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
)

// RecordStep 在数据库提交之后才发布事件，失败时不会产生只有页面看得到的虚假步骤。
func (store *Store) RecordStep(ctx context.Context, id string, step agent.Step) (agent.Step, error) {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return step, err
	}
	defer tx.Rollback()
	saved, err := store.appendStep(ctx, tx, id, step)
	if err != nil {
		return step, err
	}
	if err = tx.Commit(); err != nil {
		return step, err
	}
	store.publish(id, saved)
	return saved, nil
}

// appendStep 被普通步骤和状态转换共用，使终态事件也有可补发的数据库编号。
func (store *Store) appendStep(ctx context.Context, tx *sql.Tx, id string, step agent.Step) (agent.Step, error) {
	if step.Kind != "llm" && step.Kind != "tool" && step.Kind != "system" {
		return step, fmt.Errorf("不支持的步骤类型")
	}
	if len(step.Args) == 0 {
		step.Args = json.RawMessage(`{}`)
	}
	clean, err := store.config.Redactor.JSON(step.Args)
	if err != nil {
		return step, err
	}
	step.Args, step.Result = clean, store.config.Redactor.Text(step.Result)
	if err = tx.QueryRowContext(ctx, "SELECT next_seq FROM diagnoses WHERE id=? FOR UPDATE", id).Scan(&step.Seq); err != nil {
		return step, err
	}
	step.Seq++
	step.CreatedAt = store.config.Now().UTC()
	// 一个事务同时写行与递增编号；回滚时两者一起消失，不留下无内容的编号。
	_, err = tx.ExecContext(ctx, "INSERT INTO diagnosis_steps(diagnosis_id,seq,kind,model,tool_name,args,result,status,duration_ms,tokens_in,tokens_out,cache_hit_tokens,cost_micros,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", id, step.Seq, step.Kind, step.Model, step.ToolName, step.Args, step.Result, step.Status, step.DurationMS, step.Usage.Input, step.Usage.Output, step.Usage.CacheHit, step.CostMicros, step.CreatedAt)
	if err != nil {
		return step, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE diagnoses SET next_seq=?,tokens_in=tokens_in+?,tokens_out=tokens_out+?,cache_hit_tokens=cache_hit_tokens+?,cost_micros=cost_micros+? WHERE id=?", step.Seq, step.Usage.Input, step.Usage.Output, step.Usage.CacheHit, step.CostMicros, id)
	return step, err
}

// publish 不持有数据库锁；回调只能做有界的内存投递，慢浏览器不能拖住事务。
func (store *Store) publish(id string, step agent.Step) {
	if store.config.Published != nil {
		store.config.Published(id, step)
	}
}

// StepsAfter 每次最多读取一百步，SSE 会继续翻页，不把所有历史一次塞进内存。
func (store *Store) StepsAfter(ctx context.Context, id string, after int) ([]agent.Step, error) {
	if after < 0 {
		return nil, fmt.Errorf("步骤编号不能为负数")
	}
	rows, err := store.database.QueryContext(ctx, "SELECT seq,kind,model,tool_name,args,result,status,duration_ms,tokens_in,tokens_out,cache_hit_tokens,cost_micros,created_at FROM diagnosis_steps WHERE diagnosis_id=? AND seq>? ORDER BY seq LIMIT 100", id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 即使没有后续步骤也返回空数组，页面不必区分 null 与列表。
	steps := []agent.Step{}
	for rows.Next() {
		var step agent.Step
		if err = rows.Scan(&step.Seq, &step.Kind, &step.Model, &step.ToolName, &step.Args, &step.Result, &step.Status, &step.DurationMS, &step.Usage.Input, &step.Usage.Output, &step.Usage.CacheHit, &step.CostMicros, &step.CreatedAt); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}
