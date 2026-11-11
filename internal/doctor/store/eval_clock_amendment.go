// 本文件：这是历史批次的一次性校时修订。批准记录只允许指定的中断诊断补跑一次，原冻结清单仍保留。
// 新批次不会产生修订记录，EvalVersions 直接返回冻结版本。第一次阅读评测流程时可以跳过本文件。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// AmendEvalClock 由宿主机命令显式调用。这里没有 HTTP 入口，也不会让模型自行批准。
func (store *Store) AmendEvalClock(ctx context.Context, runID, key, reason string, original, replacement json.RawMessage) error {
	if len(reason) < 10 || len(reason) > 2000 || !json.Valid(original) || !json.Valid(replacement) || sameJSON(original, replacement) {
		return fmt.Errorf("需要具体的用户授权原因和新旧版本")
	}
	var interruption int64
	// 只接受原槽位中没有报告的取消任务。还须有同轮、诊断期间发生的校时中断。
	err := store.database.QueryRowContext(ctx, `SELECT i.id FROM eval_results r JOIN diagnoses d ON d.id=r.diagnosis_id JOIN eval_interruptions i ON i.run_id=r.run_id AND i.scenario=r.scenario AND i.iteration=r.iteration WHERE r.run_id=? AND r.result_key=? AND r.attempt=0 AND d.status='cancelled' AND (d.report IS NULL OR JSON_TYPE(d.report)='NULL') AND i.state='diagnosing' AND i.reason LIKE '容器时钟%' AND i.created_at>=d.started_at AND i.created_at<=DATE_ADD(d.finished_at,INTERVAL 5 SECOND) ORDER BY i.id DESC LIMIT 1`, runID, key).Scan(&interruption)
	if err != nil {
		return fmt.Errorf("未找到对应的时钟取消记录：%w", err)
	}
	_, err = store.database.ExecContext(ctx, `INSERT INTO eval_clock_amendments(run_id,original_versions,replacement_versions,makeup_key,reason,interruption_id) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE run_id=run_id`, runID, original, replacement, key, store.config.Redactor.Text(reason), interruption)
	if err != nil {
		return err
	}
	var before, after []byte
	var savedKey, savedReason string
	err = store.database.QueryRowContext(ctx, "SELECT original_versions,replacement_versions,makeup_key,reason FROM eval_clock_amendments WHERE run_id=?", runID).Scan(&before, &after, &savedKey, &savedReason)
	if err == nil && (!sameJSON(before, original) || !sameJSON(after, replacement) || savedKey != key || savedReason != store.config.Redactor.Text(reason)) {
		return fmt.Errorf("校时批准记录已存在，拒绝改写")
	}
	return err
}

// EvalVersions 原版本仍是事实；只有匹配它的独立批准记录才能指定续跑版本。
func (store *Store) EvalVersions(ctx context.Context, runID string, original json.RawMessage) (json.RawMessage, error) {
	var before, after []byte
	err := store.database.QueryRowContext(ctx, "SELECT original_versions,replacement_versions FROM eval_clock_amendments WHERE run_id=?", runID).Scan(&before, &after)
	if errors.Is(err, sql.ErrNoRows) {
		return store.baselineVersions(ctx, runID, original)
	}
	if err != nil {
		return nil, err
	}
	if !sameJSON(before, original) {
		return nil, fmt.Errorf("校时批准与原冻结版本不符")
	}
	return store.baselineVersions(ctx, runID, after)
}

// clockMakeupAllowed 不把时钟异常推广为所有取消都可重跑；只认明确批准的那一条。
func (store *Store) clockMakeupAllowed(ctx context.Context, id string) (bool, error) {
	var count int
	err := store.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM eval_clock_amendments a JOIN eval_results r ON r.result_key=a.makeup_key WHERE r.diagnosis_id=? AND r.attempt=0`, id).Scan(&count)
	return count == 1, err
}
