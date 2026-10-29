// 本文件：用专用医生测试库验证评测不会重复创建付费任务，也不会执行写操作。
//
// 主要内容：TestIntegrationEvaluation 检查暂停、重试、模型约束和恢复；
// TestIntegrationEvaluationApproval 检查提议留档、自动拒绝和执行器的最后一道拦截。
// 阅读建议：这里只操作本测试创建的 doctor 记录，不访问业务表，也不调用真实模型。

package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// evaluationFixture 为每个测试准备独占批次，并在关闭连接池前清理模式行。
func evaluationFixture(t *testing.T) (*Store, EvaluationInput) {
	t.Helper()
	repository, target := integrationStore(t)
	value := EvaluationInput{Target: target, RunID: target, Key: target + "/trial/E1", Group: "E1", ExpectedModel: "mock-eval", FaultStartedAt: time.Now().UTC().Truncate(time.Microsecond), Symptoms: json.RawMessage(`{"alerts":[],"symptoms":"虚构例行检查"}`)}
	if err := repository.CreateEvalRun(t.Context(), EvalRun{ID: value.RunID, Target: target, Config: json.RawMessage(`{"mode":"mock"}`), TokenLimit: 100}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := repository.database.ExecContext(context.Background(), "DELETE FROM evaluation_targets WHERE target=?", target)
		if err != nil {
			t.Error(err)
		}
	})
	return repository, value
}

// TestIntegrationEvaluation 同一配置重试只拿回旧 ID；切换模型不能把这份实验交给别的客户端。
func TestIntegrationEvaluation(t *testing.T) {
	repository, value := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "eval-owner")
	before, err := repository.ReceiveAlerts(ctx, value.Target, testNotification())
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.SetEvaluationMode(ctx, value.Target, value.RunID, true); err != nil {
		t.Fatal(err)
	}
	if detail, err := repository.Detail(ctx, before.DiagnosisID); err != nil || detail.Status != "cancelled" {
		t.Fatalf("旧告警仍可执行：%+v %v", detail, err)
	}
	id, err := repository.Evaluation(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		repeated, err := repository.Evaluation(ctx, value)
		if err != nil || repeated != id {
			t.Fatalf("重试创建新任务：%s %v", repeated, err)
		}
	}
	changed := value
	changed.Symptoms = json.RawMessage(`{"symptoms":"改了输入"}`)
	if _, err := repository.Evaluation(ctx, changed); err == nil {
		t.Fatal("同编号覆盖了输入")
	}
	if ok, err := repository.StartTask(ctx, id, "mock-other", "test", "mock"); err == nil || ok {
		t.Fatal("评测跨过模型切换")
	}
	if ok, err := repository.StartTask(ctx, id, value.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	if err = repository.SetEvaluationMode(ctx, value.Target, value.RunID, false); err == nil {
		t.Fatal("诊断运行中退出评测")
	}
	if err = repository.EndTask(ctx, id, "succeeded", agent.Result{}, ""); err != nil {
		t.Fatal(err)
	}
	// 换一个告警组，证明进入模式之后的新通知也只留档，不只是取消了之前的队列。
	notification := testNotification()
	notification.GroupKey = "second-test-group"
	receipt, err := repository.ReceiveAlerts(ctx, value.Target, notification)
	if err != nil || !receipt.Suppressed {
		t.Fatalf("新告警未暂停：%+v %v", receipt, err)
	}
	if detail, err := repository.Detail(ctx, receipt.DiagnosisID); err != nil || detail.Status != "cancelled" {
		t.Fatal("捕获告警进入执行队列")
	}
	if err = repository.SetEvaluationMode(ctx, value.Target, value.RunID, false); err == nil {
		t.Fatal("开放故障期间恢复自动付费")
	}
	repository.config.Now = func() time.Time { return time.Now().UTC().Add(20 * time.Minute) }
	if err = repository.SetEvaluationMode(ctx, value.Target, value.RunID, false); err != nil {
		t.Fatal(err)
	}
	mode, err := repository.EvaluationMode(ctx, value.Target)
	if err != nil || mode.Enabled {
		t.Fatal("模式没有恢复")
	}
}

// TestIntegrationEvaluationApproval 即使有人误改审批状态，执行器也要根据诊断来源拒绝领取。
func TestIntegrationEvaluationApproval(t *testing.T) {
	repository, value := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "eval-owner")
	if err := repository.SetEvaluationMode(ctx, value.Target, value.RunID, true); err != nil {
		t.Fatal(err)
	}
	id, err := repository.Evaluation(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.StartTask(ctx, id, value.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	proposal, err := repository.CreateApproval(ctx, approval.Record{ID: newID(), DiagnosisID: id, Target: value.Target, ParentSeq: 1, CallID: "proposal", Tool: "set_db_pool_size", Args: json.RawMessage(`{"size":30}`), Plan: approval.Plan{Tool: "set_db_pool_size", Size: 30}, Evidence: []int{1}})
	if err != nil || proposal.Status != "rejected" || proposal.DecidedBy != "eval-policy" || proposal.Plan.Size != 30 {
		t.Fatalf("未保留拒绝和参数：%+v %v", proposal, err)
	}
	if ok, err := repository.DecideApproval(ctx, proposal.ID, "test-user", true); err == nil || ok {
		t.Fatal("页面能批准评测操作")
	}
	// 这是专用测试库中的故意破坏，用来证明执行入口不只依赖按钮或前一道状态检查。
	if _, err = repository.database.ExecContext(ctx, "UPDATE approvals SET status='approved' WHERE id=?", proposal.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.ClaimApproval(ctx, id, proposal.ID); err == nil {
		t.Fatal("执行器放行了评测写操作")
	}
	if err = repository.EndTask(ctx, id, "succeeded", agent.Result{}, ""); err != nil {
		t.Fatal(err)
	}
}
