// 本文件：固定人工校准抽中的二十份报告，不替用户填写分数。
//
// 主要内容：EvalCalibration 读取原抽样；SaveEvalCalibration 只保存第一次抽样。
// 阅读建议：这里保存的是编号，不包含评委分数，导出表由评测器另行生成。

package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// EvalCalibration 没有抽过时保留 sql.ErrNoRows，让调用方明确区分读取错误。
func (store *Store) EvalCalibration(ctx context.Context, id string) ([]string, error) {
	var raw []byte
	err := store.database.QueryRowContext(ctx, "SELECT result_keys FROM eval_calibration WHERE run_id=?", id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var keys []string
	err = json.Unmarshal(raw, &keys)
	return keys, err
}

// SaveEvalCalibration 不覆盖先前的随机样本，也不允许未结束批次提前挑出好报告。
func (store *Store) SaveEvalCalibration(ctx context.Context, id string, keys []string) ([]string, error) {
	if len(keys) != 20 {
		return nil, fmt.Errorf("校准需要恰好二十份报告")
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			return nil, fmt.Errorf("校准样本重复")
		}
		seen[key] = true
		// 核对归属，调用者不能把其他批次的报告移进这一轮校准。
		var count int
		if err := store.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM eval_results r JOIN eval_runs e ON e.id=r.run_id WHERE r.result_key=? AND r.run_id=? AND r.result IS NOT NULL AND r.judgement IS NOT NULL AND e.status='completed'", key, id).Scan(&count); err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, fmt.Errorf("校准样本不属于已完成批次")
		}
	}
	raw, _ := json.Marshal(keys)
	_, err := store.database.ExecContext(ctx, "INSERT INTO eval_calibration(run_id,result_keys) VALUES(?,?) ON DUPLICATE KEY UPDATE run_id=run_id", id, raw)
	if err != nil {
		return nil, err
	}
	return store.EvalCalibration(ctx, id)
}
