// 本文件：证明整批 token 上限和供应商费用账户在同一事务里生效。
//
// 主要内容：TestIntegrationEvalTokenBudget 检查拒绝不扣费、缓存只计一次、未知量保留和续跑不重置。
// 阅读建议：测试只调用本机仓储，不连接供应商；这里的 token 和费用全是明确的测试样例。

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalTokenBudget 让费用余额充足而 token 不足，单独验证评测这一道限制。
func TestIntegrationEvalTokenBudget(t *testing.T) {
	repository, value := evaluationFixture(t)
	ctx := t.Context()
	if err := repository.SetEvaluationMode(ctx, value.Target, value.RunID, true); err != nil {
		t.Fatal(err)
	}
	id, err := repository.Evaluation(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	account := "test-" + newID()
	ledger, err := repository.NewLedger(ctx, account, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repository.database.ExecContext(context.Background(), "DELETE FROM llm_attempts WHERE account=?", account)
		_, _ = repository.database.ExecContext(context.Background(), "DELETE FROM llm_budget WHERE account=?", account)
	})
	request := llm.Reservation{DiagnosisID: id, Model: "mock-eval", Request: json.RawMessage(`{}`), MaximumMicros: 10, MaximumTokens: 60}
	first, err := ledger.Reserve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ledger.Reserve(ctx, request); !errors.Is(err, llm.ErrEvaluationBudget) {
		t.Fatalf("120 超过 100 没有拒绝：%v", err)
	}
	status, err := ledger.Status(ctx)
	if err != nil || status.SpentMicros != 10 {
		t.Fatal("token 拒绝仍然扣了费用")
	}
	// Input 已经包含 CacheHit，所以 20 输入和 10 输出只能结算 30，不能算成 45。
	if err = ledger.Settle(ctx, first, llm.Settlement{Usage: llm.Usage{Input: 20, Output: 10, CacheHit: 15}, CostMicros: 3, Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Reserve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err = ledger.Settle(ctx, second, llm.Settlement{Unknown: true, Status: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	run, err := repository.EvalRun(ctx, value.RunID)
	if err != nil || run.TokensUsed != 90 {
		t.Fatalf("未知量或缓存计数错误：%+v %v", run, err)
	}
	if err = repository.CreateEvalRun(ctx, EvalRun{ID: value.RunID, Target: value.Target, Config: json.RawMessage(`{"mode":"mock"}`), TokenLimit: 100}); err != nil {
		t.Fatal(err)
	}
	reopened, _ := repository.NewLedger(ctx, account, 1000)
	request.MaximumTokens = 11
	if _, err = reopened.Reserve(ctx, request); !errors.Is(err, llm.ErrEvaluationBudget) {
		t.Fatal("续跑清除了未确认预留")
	}
}
