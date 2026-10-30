// 本文件：在评测动作开始前保存执行器版本，续跑也留下自己的记录。
//
// 主要内容：RecordEvalExecution 追加命令、轮次或槽位的版本快照。
// 阅读建议：commit 指向代码提交，source_hash 同时覆盖未提交文件。这里不接收模型输入。

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

// RecordEvalExecution 只追加，不能给过去的报告换一个版本或抹掉先前的失败尝试。
func (store *Store) RecordEvalExecution(ctx context.Context, runID, scenario string, iteration int, operation, key string, versions json.RawMessage) error {
	var value struct {
		Commit string `json:"commit"`
		Hash   string `json:"source_hash"`
	}
	if json.Unmarshal(versions, &value) != nil || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(value.Commit) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value.Hash) {
		return fmt.Errorf("执行审计缺少完整提交号与源码哈希")
	}
	if !slices.Contains([]string{"run", "matrix", "slot", "pending", "injecting", "verifying", "diagnosing", "restoring", "cooling"}, operation) || len(key) > 128 {
		return fmt.Errorf("执行审计操作不合法")
	}
	if operation != "run" && (!regexp.MustCompile(`^S(0[1-9]|1[0-3])$`).MatchString(scenario) || iteration < 1 || iteration > 3) {
		return fmt.Errorf("执行审计轮次不合法")
	}
	_, err := store.database.ExecContext(ctx, "INSERT INTO eval_execution_audits(run_id,scenario,iteration,operation,result_key,versions) VALUES(?,?,?,?,?,?)", runID, scenario, iteration, operation, key, versions)
	return err
}
