// 本文件：保存实验槽位、最初告警和最终结果，供评测器断点续跑。
//
// 槽位就是“某个场景的第几次、哪个实验组”。它先于模型任务保存。即使在创建
// 任务后丢了回复，再次运行也会拿回同一个输入和编号，不会再付一次钱。
//
// 主要内容：PrepareEvalResult 保存不可变输入；AttachEvalDiagnosis 关联幂等任务；
// FinishEvalResult 只保存一次终态；EvalResults 为导出和页面返回结果。
// 阅读建议：不覆盖失败结果；需要重新运行时使用新的批次或明确的新轮次。

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// EvalResult 包含运行位置和原始输入；Data 在诊断结束前为空。
type EvalResult struct {
	Key            string          `json:"key"`              // 全局唯一槽位编号。
	RunID          string          `json:"run_id"`           // 批次拥有自己的 token 额度。
	Scenario       string          `json:"scenario"`         // 仅评测器和结果页使用，不发给诊断模型。
	Iteration      int             `json:"iteration"`        // 从一开始的重复轮次。
	Group          string          `json:"group"`            // E0–E3。
	Position       int             `json:"position"`         // 同次故障中实际执行的先后位置。
	FaultStartedAt time.Time       `json:"fault_started_at"` // 配对组共用的开始时间。
	Input          json.RawMessage `json:"input"`            // 最初告警快照，续跑不重抓一份来替换。
	DiagnosisID    string          `json:"diagnosis_id"`     // 与诊断表的唯一关联。
	Data           json.RawMessage `json:"result"`           // 不可变终态摘要；完整步骤仍在诊断表。
}

// EvalDiagnosisState 的轮询只取一个状态字段，不反复加载可能很长的模型对话。
func (store *Store) EvalDiagnosisState(ctx context.Context, id string) (string, error) {
	var status string
	err := store.database.QueryRowContext(ctx, "SELECT status FROM diagnoses WHERE id=? AND `trigger`='eval'", id).Scan(&status)
	return status, err
}

// PrepareEvalResult 不覆盖旧告警。调用方必须使用返回的快照，而不是刚刚抓到的新快照。
func (store *Store) PrepareEvalResult(ctx context.Context, value EvalResult) (EvalResult, error) {
	if value.Key == "" || len(value.Key) > 128 || value.Iteration < 1 || value.Position < 1 || value.FaultStartedAt.IsZero() || len(value.Input) > 65536 || !json.Valid(value.Input) {
		return value, fmt.Errorf("实验槽位或输入不合法")
	}
	clean, err := store.config.Redactor.JSON(value.Input)
	if err != nil {
		return value, err
	}
	_, err = store.database.ExecContext(ctx, "INSERT INTO eval_results(result_key,run_id,scenario,iteration,experiment_group,position,fault_started_at,input) VALUES(?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE result_key=result_key", value.Key, value.RunID, value.Scenario, value.Iteration, value.Group, value.Position, value.FaultStartedAt.UTC(), clean)
	if err != nil {
		return value, err
	}
	saved, err := store.readEvalResult(ctx, value.Key)
	if err != nil {
		return value, err
	}
	// 同编号的配置不同是调用错误；相同编号的告警已变化则只是重试，沿用旧快照。
	if saved.RunID != value.RunID || saved.Scenario != value.Scenario || saved.Iteration != value.Iteration || saved.Group != value.Group || saved.Position != value.Position {
		return value, fmt.Errorf("实验槽位已有不同配置")
	}
	return saved, nil
}

// readEvalResult 在一个查询里取回完整槽位，避免分别读取时拼出不同版本的快照。
func (store *Store) readEvalResult(ctx context.Context, key string) (EvalResult, error) {
	var value EvalResult
	var data []byte
	err := store.database.QueryRowContext(ctx, "SELECT result_key,run_id,scenario,iteration,experiment_group,position,fault_started_at,input,COALESCE(diagnosis_id,''),result FROM eval_results WHERE result_key=?", key).Scan(&value.Key, &value.RunID, &value.Scenario, &value.Iteration, &value.Group, &value.Position, &value.FaultStartedAt, &value.Input, &value.DiagnosisID, &data)
	value.Data = data
	return value, err
}

// AttachEvalDiagnosis 只接受同批次、同幂等编号的任务，不能把另一次好结果移过来替换失败。
func (store *Store) AttachEvalDiagnosis(ctx context.Context, key, id string) error {
	changed, err := store.database.ExecContext(ctx, "UPDATE eval_results r JOIN diagnoses d ON d.id=? AND d.eval_key=r.result_key AND d.eval_run_id=r.run_id SET r.diagnosis_id=d.id WHERE r.result_key=? AND (r.diagnosis_id IS NULL OR r.diagnosis_id=d.id)", id, key)
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
	rows, err := store.database.QueryContext(ctx, "SELECT result_key FROM eval_results WHERE run_id=? ORDER BY scenario,iteration,position", runID)
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
