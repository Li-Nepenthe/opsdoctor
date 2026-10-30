// 本文件：在专用医生库验证基线中断的重启边界，不运行真实模型。
//
// 主要内容：TestIntegrationEvalBaselineAmendment 检查旧记录保留、版本链与批准幂等；
// TestIntegrationEvalBaselineRejectStarted 验证任何已创建的槽位都会阻止重启。
// 阅读建议：测试只制造检查点，不执行场景注入，更不清理业务数据。

package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// baselineFixture 模拟“已验证症状、尚未创建模型”的那一个暂停点。
func baselineFixture(t *testing.T) (*Store, EvaluationInput, EvalTrial, int64, json.RawMessage, json.RawMessage) {
	t.Helper()
	repository, input := evaluationFixture(t)
	// 先删本测试的批准记录，再由公共清理删除它引用的中断和批次。
	t.Cleanup(func() {
		if _, err := repository.database.ExecContext(context.Background(), "DELETE FROM eval_baseline_amendments WHERE run_id=?", input.RunID); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	before := json.RawMessage(`{"commit":"` + strings.Repeat("a", 40) + `","source_hash":"` + strings.Repeat("b", 64) + `"}`)
	after := json.RawMessage(`{"commit":"` + strings.Repeat("c", 40) + `","source_hash":"` + strings.Repeat("d", 64) + `"}`)
	configuration, _ := json.Marshal(map[string]any{"kind": "paired", "versions": before})
	if _, err := repository.database.ExecContext(ctx, "UPDATE eval_runs SET config=? WHERE id=?", configuration, input.RunID); err != nil {
		t.Fatal(err)
	}
	trial, err := repository.EvalTrial(ctx, input.RunID, "S08", 1)
	if err != nil {
		t.Fatal(err)
	}
	fault := time.Now().UTC().Truncate(time.Microsecond)
	for _, next := range []string{"injecting", "verifying", "diagnosing"} {
		if err := repository.AdvanceEvalTrial(ctx, trial, next, &fault); err != nil {
			t.Fatal(err)
		}
		trial, err = repository.EvalTrial(ctx, input.RunID, "S08", 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.InterruptEval(ctx, trial, "评测基线要求200，实际400"); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := repository.database.QueryRowContext(ctx, "SELECT id FROM eval_interruptions WHERE run_id=? ORDER BY id DESC LIMIT 1", input.RunID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return repository, input, trial, id, before, after
}

// TestIntegrationEvalBaselineAmendment 同一批准只能重置一次，新注入的开始时间不能被第二次命令清空。
func TestIntegrationEvalBaselineAmendment(t *testing.T) {
	repository, input, trial, id, before, after := baselineFixture(t)
	ctx := t.Context()
	reason := "用户明确批准场景流量精确匹配与零模型轮次重启"
	if err := repository.AmendEvalBaseline(ctx, trial, id, reason, json.RawMessage(`{"wrong":true}`), after); err == nil {
		t.Fatal("接受错误父版本")
	}
	if err := repository.AmendEvalBaseline(ctx, trial, id, reason, before, after); err != nil {
		t.Fatal(err)
	}
	current, err := repository.EvalTrial(ctx, input.RunID, "S08", 1)
	if err != nil || current.State != "pending" || current.FaultStartedAt != nil {
		t.Fatal(current, err)
	}
	var raw []byte
	if err := repository.database.QueryRowContext(ctx, "SELECT original_trial FROM eval_baseline_amendments WHERE run_id=?", input.RunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var original EvalTrial
	if json.Unmarshal(raw, &original) != nil || original.State != "diagnosing" || !original.FaultStartedAt.Equal(*trial.FaultStartedAt) {
		t.Fatal("原注入时间丢失")
	}
	if err := repository.AdvanceEvalTrial(ctx, current, "injecting", nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.AmendEvalBaseline(ctx, trial, id, reason, before, after); err != nil {
		t.Fatal(err)
	}
	current, err = repository.EvalTrial(ctx, input.RunID, "S08", 1)
	if err != nil || current.State != "injecting" {
		t.Fatal("重复批准重置了新进度", err)
	}
	if err := repository.AmendEvalBaseline(ctx, trial, id, reason, before, json.RawMessage(`{"different":true}`)); err == nil {
		t.Fatal("允许覆盖批准")
	}
	versions, err := repository.EvalVersions(ctx, input.RunID, before)
	if err != nil || !sameJSON(versions, after) {
		t.Fatal("有效版本错误", err)
	}
	// 每次运行追加自己的版本。两次操作不能合并成一条、也不能让版本缺失。
	for _, operation := range []string{"run", "injecting"} {
		if err := repository.RecordEvalExecution(ctx, input.RunID, "S08", 1, operation, "", after); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.RecordEvalExecution(ctx, input.RunID, "S08", 1, "slot", "key", json.RawMessage(`{}`)); err == nil {
		t.Fatal("版本缺失仍执行")
	}
	var count int
	if err := repository.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM eval_execution_audits WHERE run_id=?", input.RunID).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if err := repository.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM eval_interruptions WHERE id=?", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("原中断丢失", err)
	}
}

// TestIntegrationEvalBaselineRejectStarted 连没有关联诊断的占位槽位也拒绝重启，不能挑一次新故障重做。
func TestIntegrationEvalBaselineRejectStarted(t *testing.T) {
	repository, input, trial, id, before, after := baselineFixture(t)
	slot := EvalResult{Key: input.Key, RunID: input.RunID, Scenario: "S08", Iteration: 1, Group: "E1", Position: 1, FaultStartedAt: *trial.FaultStartedAt, Input: json.RawMessage(`{}`)}
	if _, err := repository.PrepareEvalResult(t.Context(), slot); err != nil {
		t.Fatal(err)
	}
	if err := repository.AmendEvalBaseline(t.Context(), trial, id, "用户批准但此轮已经有模型槽位", before, after); err == nil {
		t.Fatal("已有槽位仍允许重启")
	}
	current, err := repository.EvalTrial(t.Context(), input.RunID, "S08", 1)
	if err != nil || current.State != "diagnosing" {
		t.Fatal("拒绝时改写检查点", err)
	}
}
