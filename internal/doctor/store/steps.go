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
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
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
	if step.Kind != "llm" && step.Kind != "tool" && step.Kind != "system" && step.Kind != "approval" {
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
	// 创建、开始和结束时间都使用数据库 UTC 时钟。不同进程的系统时间
	// 可能相差几秒，不能把它们混在同一条诊断的时间线上。
	if err = tx.QueryRowContext(ctx, "SELECT next_seq,UTC_TIMESTAMP(6) FROM diagnoses WHERE id=? FOR UPDATE", id).Scan(&step.Seq, &step.CreatedAt); err != nil {
		return step, err
	}
	// 行锁也把租约检查与写入放在一起，旧执行者不能在检查后被接手又继续写。
	if owner := llm.Lease(ctx); owner != "" {
		if err = requireLease(ctx, tx, id, owner); err != nil {
			return step, err
		}
	}
	if step.EventKey != "" {
		previous, lookupErr := readStep(ctx, tx, id, step.EventKey)
		if lookupErr == nil {
			previous.AlreadySaved = true
			return previous, nil
		}
		if lookupErr != sql.ErrNoRows {
			return step, lookupErr
		}
	}
	step.Currency = store.config.Currency
	step.Seq++
	// 一个事务同时写行与递增编号；回滚时两者一起消失，不留下无内容的编号。
	_, err = tx.ExecContext(ctx, "INSERT INTO diagnosis_steps(diagnosis_id,seq,kind,model,tool_name,args,result,status,duration_ms,tokens_in,tokens_out,cache_hit_tokens,cost_micros,created_at,event_key,parent_seq,call_id,counted,currency,cache_write_tokens,reasoning_tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", id, step.Seq, step.Kind, step.Model, step.ToolName, step.Args, step.Result, step.Status, step.DurationMS, step.Usage.Input, step.Usage.Output, step.Usage.CacheHit, step.CostMicros, step.CreatedAt, nullableKey(step.EventKey), step.ParentSeq, step.CallID, step.Counted, step.Currency, step.Usage.CacheWrite, step.Usage.Reasoning)
	if err != nil {
		return step, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE diagnoses SET next_seq=?,tokens_in=tokens_in+?,tokens_out=tokens_out+?,cache_hit_tokens=cache_hit_tokens+?,cost_micros=cost_micros+?,cache_write_tokens=cache_write_tokens+? WHERE id=?", step.Seq, step.Usage.Input, step.Usage.Output, step.Usage.CacheHit, step.CostMicros, step.Usage.CacheWrite, id)
	return step, err
}

// publish 不持有数据库锁；回调只能做有界的内存投递，慢浏览器不能拖住事务。
func (store *Store) publish(id string, step agent.Step) {
	if store.config.Published != nil && !step.AlreadySaved {
		store.config.Published(id, step)
	}
}

// StepsAfter 每次最多读取一百步，SSE 会继续翻页，不把所有历史一次塞进内存。
func (store *Store) StepsAfter(ctx context.Context, id string, after int) ([]agent.Step, error) {
	if after < 0 {
		return nil, fmt.Errorf("步骤编号不能为负数")
	}
	rows, err := store.database.QueryContext(ctx, "SELECT seq,kind,model,tool_name,args,result,status,duration_ms,tokens_in,tokens_out,cache_hit_tokens,cost_micros,created_at,COALESCE(event_key,''),parent_seq,call_id,counted,currency,cache_write_tokens,reasoning_tokens FROM diagnosis_steps WHERE diagnosis_id=? AND seq>? ORDER BY seq LIMIT 100", id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 即使没有后续步骤也返回空数组，页面不必区分 null 与列表。
	steps := []agent.Step{}
	for rows.Next() {
		var step agent.Step
		if err = rows.Scan(&step.Seq, &step.Kind, &step.Model, &step.ToolName, &step.Args, &step.Result, &step.Status, &step.DurationMS, &step.Usage.Input, &step.Usage.Output, &step.Usage.CacheHit, &step.CostMicros, &step.CreatedAt, &step.EventKey, &step.ParentSeq, &step.CallID, &step.Counted, &step.Currency, &step.Usage.CacheWrite, &step.Usage.Reasoning); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

// nullableKey 让旧记录和无须去重的状态事件使用 SQL NULL，避免空串互相冲突。
func nullableKey(key string) any {
	if key == "" {
		return nil
	}
	return key
}

// readStep 在同一事务内返回已保存的完整结果，重试保存不会分配第二个步骤号。
func readStep(ctx context.Context, tx *sql.Tx, id, key string) (agent.Step, error) {
	var step agent.Step
	err := tx.QueryRowContext(ctx, "SELECT seq,kind,model,tool_name,args,result,status,duration_ms,tokens_in,tokens_out,cache_hit_tokens,cost_micros,created_at,COALESCE(event_key,''),parent_seq,call_id,counted,currency,cache_write_tokens,reasoning_tokens FROM diagnosis_steps WHERE diagnosis_id=? AND event_key=?", id, key).Scan(&step.Seq, &step.Kind, &step.Model, &step.ToolName, &step.Args, &step.Result, &step.Status, &step.DurationMS, &step.Usage.Input, &step.Usage.Output, &step.Usage.CacheHit, &step.CostMicros, &step.CreatedAt, &step.EventKey, &step.ParentSeq, &step.CallID, &step.Counted, &step.Currency, &step.Usage.CacheWrite, &step.Usage.Reasoning)
	return step, err
}
