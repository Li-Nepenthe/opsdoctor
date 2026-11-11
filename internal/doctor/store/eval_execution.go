// 本文件：每次执行评测动作前都追加一条版本审计。提交号标识 Git 版本，源码哈希还涵盖未提交文件。
// 同一轮断点续跑可能换了执行器，必须单独留记录。审计只接收版本和动作，不接收模型输入。

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

// EvalExecution 记录一个评测动作实际运行的版本。同一轮续跑可以产生多条记录。
type EvalExecution struct {
	RunID     string
	Scenario  string
	Iteration int
	Operation string
	ResultKey string
	Versions  json.RawMessage
}

// RecordEvalExecution 只追加，不能给过去的报告换一个版本或抹掉先前的失败尝试。
func (store *Store) RecordEvalExecution(ctx context.Context, audit EvalExecution) error {
	var value struct {
		Commit string `json:"commit"`
		Hash   string `json:"source_hash"`
	}
	if json.Unmarshal(audit.Versions, &value) != nil || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(value.Commit) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value.Hash) {
		return fmt.Errorf("执行审计缺少完整提交号与源码哈希")
	}
	if !slices.Contains([]string{"run", "matrix", "slot", "pending", "injecting", "verifying", "diagnosing", "restoring", "cooling"}, audit.Operation) || len(audit.ResultKey) > 128 {
		return fmt.Errorf("执行审计操作不合法")
	}
	if audit.Operation != "run" && !validEvalTrial(audit.RunID, audit.Scenario, audit.Iteration) {
		return fmt.Errorf("执行审计轮次不合法")
	}
	_, err := store.database.ExecContext(ctx, insertExecutionAuditSQL, audit.RunID, audit.Scenario, audit.Iteration, audit.Operation, audit.ResultKey, audit.Versions)
	return err
}

const insertExecutionAuditSQL = `INSERT INTO eval_execution_audits (
    run_id,
    scenario,
    iteration,
    operation,
    result_key,
    versions
)
VALUES (?, ?, ?, ?, ?, ?)`
