// 本文件：每个实验组在故障的一轮注入中占一个结果槽位。先保存槽位和告警再创建模型任务，断点续跑才能取回原输入和诊断编号。
// 终态只写一次，包括失败结果。需要再运行时须使用新批次、新轮次或获准的补跑槽位。

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// EvalResult 包含运行位置和原始输入；Data 在诊断结束前为空。
type EvalResult struct {
	JudgeID          string          `json:"judge_id"`           // 独立的人民币审计归属。
	Judgement        json.RawMessage `json:"judgement"`          // 评分失败也保存，原医生摘要不被改写。
	ExecutionProfile string          `json:"execution_profile"`  // 同组的不同模型分别占槽位。
	Attempt          int             `json:"attempt"`            // 0 是原运行，1 是唯一一次基础设施补跑。
	RetryOf          string          `json:"retry_of,omitempty"` // 指回原失败，导出时不能把两次尝试混成一次成功。
	Key              string          `json:"key"`
	RunID            string          `json:"run_id"`    // 批次拥有自己的 token 额度。
	Scenario         string          `json:"scenario"`  // 仅评测器和结果页使用，不发给诊断模型。
	Iteration        int             `json:"iteration"` // 从一开始的重复轮次。
	Group            string          `json:"group"`
	Position         int             `json:"position"`         // 同次故障中实际执行的先后位置。
	FaultStartedAt   time.Time       `json:"fault_started_at"` // 配对组共用的开始时间。
	Input            json.RawMessage `json:"input"`            // 最初告警快照，续跑不重抓一份来替换。
	DiagnosisID      string          `json:"diagnosis_id"`
	Data             json.RawMessage `json:"result"` // 不可变终态摘要；完整步骤仍在诊断表。
}

// EvalDiagnosisState 的轮询只取一个状态字段，不反复加载可能很长的模型对话。
func (store *Store) EvalDiagnosisState(ctx context.Context, id string) (string, error) {
	var status string
	err := store.database.QueryRowContext(ctx, "SELECT status FROM diagnoses WHERE id=? AND `trigger`='eval'", id).Scan(&status)
	return status, err
}

// PrepareEvalResult 不覆盖旧告警。调用方必须使用返回的快照，而不是刚刚抓到的新快照。
func (store *Store) PrepareEvalResult(ctx context.Context, value EvalResult) (EvalResult, error) {
	if value.Attempt != 0 || value.RetryOf != "" || value.Key == "" || len(value.Key) > 128 || value.Iteration < 1 || value.Position < 1 || value.FaultStartedAt.IsZero() || len(value.Input) > 65536 || !json.Valid(value.Input) {
		return value, fmt.Errorf("实验槽位或输入不合法")
	}
	clean, err := store.config.Redactor.JSON(value.Input)
	if err != nil {
		return value, err
	}
	// 与恢复文件的比较统一用微秒截断，不能交给数据库自行四舍五入。
	_, err = store.database.ExecContext(ctx, "INSERT INTO eval_results(result_key,run_id,scenario,iteration,experiment_group,position,fault_started_at,input,execution_profile) VALUES(?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE result_key=result_key", value.Key, value.RunID, value.Scenario, value.Iteration, value.Group, value.Position, value.FaultStartedAt.UTC().Truncate(time.Microsecond), clean, value.ExecutionProfile)
	if err != nil {
		return value, err
	}
	saved, err := store.readEvalResult(ctx, value.Key)
	if err != nil {
		return value, err
	}
	// 同编号的配置不同是调用错误；相同编号的告警已变化则只是重试，沿用旧快照。
	if saved.ExecutionProfile != value.ExecutionProfile || saved.Attempt != 0 || saved.RetryOf != "" || saved.RunID != value.RunID || saved.Scenario != value.Scenario || saved.Iteration != value.Iteration || saved.Group != value.Group || saved.Position != value.Position {
		return value, fmt.Errorf("实验槽位已有不同配置")
	}
	return saved, nil
}

// readEvalResult 在一个查询里取回完整槽位，避免分别读取时拼出不同版本的快照。
func (store *Store) readEvalResult(ctx context.Context, key string) (EvalResult, error) {
	var row struct {
		EvalResult
		data, judgement []byte // 结果和评分在各自结束前可以是 SQL NULL。
	}
	err := store.database.QueryRowContext(ctx, "SELECT result_key,run_id,scenario,iteration,experiment_group,position,fault_started_at,input,COALESCE(diagnosis_id,''),result,attempt,COALESCE(retry_of,''),execution_profile,COALESCE(judge_id,''),judgement FROM eval_results WHERE result_key=?", key).Scan(
		&row.Key,
		&row.RunID,
		&row.Scenario,
		&row.Iteration,
		&row.Group,
		&row.Position,
		&row.FaultStartedAt,
		&row.Input,
		&row.DiagnosisID,
		&row.data,
		&row.Attempt,
		&row.RetryOf,
		&row.ExecutionProfile,
		&row.JudgeID,
		&row.judgement,
	)
	row.Data, row.Judgement = row.data, row.judgement
	return row.EvalResult, err
}

// AttachEvalDiagnosis 只接受同批次、同幂等编号的任务，不能把另一次好结果移过来替换失败。
func (store *Store) AttachEvalDiagnosis(ctx context.Context, key, id string) error {
	changed, err := store.database.ExecContext(ctx, "UPDATE eval_results r JOIN diagnoses d ON d.id=? AND d.eval_key=r.result_key AND d.eval_run_id=r.run_id AND d.eval_profile=r.execution_profile SET r.diagnosis_id=d.id WHERE r.result_key=? AND (r.diagnosis_id IS NULL OR r.diagnosis_id=d.id)", id, key)
	if err != nil {
		return err
	}
	if _, err = changed.RowsAffected(); err != nil {
		return err
	}
	saved, err := store.readEvalResult(ctx, key)
	if err != nil {
		return err
	}
	if saved.DiagnosisID != id {
		return fmt.Errorf("诊断与实验槽位不符")
	}
	return nil
}

// FinishEvalResult 失败也保存；重试允许相同结果，不允许用新摘要覆盖旧结果。
func (store *Store) FinishEvalResult(ctx context.Context, key string, result json.RawMessage) error {
	// 数据库也检查任务已经结束，防止调用方把运行中的中间值当作最终结果。
	if !json.Valid(result) || len(result) > 1<<20 {
		return fmt.Errorf("评测结果格式或大小不合法")
	}
	clean, err := store.config.Redactor.JSON(result)
	if err != nil {
		return err
	}
	if _, err = store.database.ExecContext(ctx, "UPDATE eval_results r JOIN diagnoses d ON d.id=r.diagnosis_id SET r.result=?,r.finished_at=UTC_TIMESTAMP(6) WHERE r.result_key=? AND r.result IS NULL AND d.status IN ('succeeded','failed','cancelled','budget_exceeded')", clean, key); err != nil {
		return err
	}
	saved, err := store.readEvalResult(ctx, key)
	if err != nil {
		return err
	}
	if !sameJSON(saved.Data, clean) {
		return fmt.Errorf("终态结果已保存，拒绝覆盖")
	}
	return nil
}

// EvalResults 先取小编号列表再逐个读结果，避免占着连接池里的游标申请第二条连接。
func (store *Store) EvalResults(ctx context.Context, runID string) ([]EvalResult, error) {
	// 先关掉第一轮查询，再读取各个快照；连接池只有一个空闲连接时也能完成。
	rows, err := store.database.QueryContext(ctx, "SELECT result_key FROM eval_results WHERE run_id=? ORDER BY scenario,iteration,position,attempt", runID)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	values := []EvalResult{}
	for _, key := range keys {
		value, err := store.readEvalResult(ctx, key)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
