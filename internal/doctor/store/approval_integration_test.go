// 本文件：在专用医生测试库验证审批的事务边界和等待计时。
//
// 不执行任何 shop 写操作。这里检查的是两个批准请求争抢同一张单时谁能成功，
// 以及进程退出后是否还能读到同一份参数和执行线索。
//
// 主要内容：TestIntegrationApproval 验证等待、决定和执行领取；TestIntegrationApprovalExpiry 验证过期与取消。
// 阅读建议：等待时间通过移动专用任务的时间字段模拟，不真的等待半小时。

package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// approvalFixture 建立一个正在执行的诊断和一张明确参数的单子，不调用模型。
func approvalFixture(t *testing.T, repository *Store, target string) (context.Context, approval.Record) {
	t.Helper()
	ctx := llm.WithLease(t.Context(), "first-owner")
	id, err := repository.Manual(ctx, target, "审批测试")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.StartTask(ctx, id, "mock-a", "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	value, err := repository.CreateApproval(ctx, approval.Record{ID: newID(), DiagnosisID: id, Target: target, ParentSeq: 1, CallID: "call-one", Tool: "set_db_pool_size", Args: json.RawMessage(`{"size":30}`), Plan: approval.Plan{Tool: "set_db_pool_size", Size: 30, Preview: "测试计划", Rollback: "恢复旧值"}, Reason: "测试", Evidence: []int{1}, Expected: "测试", Risk: "测试"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, value
}

// TestIntegrationApproval 人不决定时不会重跑；两个请求同时决定也只有一个成功。
func TestIntegrationApproval(t *testing.T) {
	repository, target := integrationStore(t)
	ctx, value := approvalFixture(t, repository, target)
	if _, err := repository.ClaimApproval(ctx, value.DiagnosisID, value.ID); err == nil {
		t.Fatal("未经批准可执行")
	}
	if err := repository.EndTask(ctx, value.DiagnosisID, run.WaitingApproval, agent.Result{}, "等待"); err != nil {
		t.Fatal(err)
	}
	// 模拟等待十分钟。新进程恢复不会把主动等待当成崩溃，也不把等待算入活动时间。
	if _, err := repository.database.ExecContext(t.Context(), "UPDATE diagnoses SET lease_updated_at=UTC_TIMESTAMP(6)-INTERVAL 10 MINUTE WHERE id=?", value.DiagnosisID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tasks, err := repository.Pending(t.Context()); err != nil || len(tasks) != 0 {
		t.Fatal("待审批任务被重新执行")
	}
	another, err := repository.Manual(t.Context(), target, "同系统排队")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.StartTask(llm.WithLease(t.Context(), "other"), another, "mock", "test", "mock"); err != nil || ok {
		t.Fatal("待审批没有占住系统名额")
	}
	var group sync.WaitGroup
	out := make(chan bool, 2)
	for range 2 {
		group.Go(func() {
			ok, err := repository.DecideApproval(t.Context(), value.ID, "test-user", true)
			if err != nil {
				t.Error(err)
			}
			out <- ok
		})
	}
	group.Wait()
	close(out)
	count := 0
	for ok := range out {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("批准成功次数=%d", count)
	}
	next := llm.WithLease(t.Context(), "resumed-owner")
	if ok, err := repository.StartTask(next, value.DiagnosisID, "mock-b", "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	_, active, _, err := repository.Resume(next, value.DiagnosisID)
	if err != nil || active > time.Minute {
		t.Fatalf("审批等待被计时：%s %v", active, err)
	}
	claimed, err := repository.ClaimApproval(next, value.DiagnosisID, value.ID)
	if err != nil || claimed.Plan.Size != 30 || claimed.Status != "executing" {
		t.Fatal("审批原参数丢失")
	}
	if _, err = repository.ClaimApproval(next, value.DiagnosisID, value.ID); err == nil {
		t.Fatal("同执行者重复领取")
	}
	if err = repository.ApprovalConnection(next, value.DiagnosisID, value.ID, 123); err != nil {
		t.Fatal(err)
	}
	// 模拟执行到一半崩溃。新租约接到的是同一张 executing 单和之前的连接编号。
	if _, err = repository.database.ExecContext(t.Context(), "UPDATE diagnoses SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", value.DiagnosisID); err != nil {
		t.Fatal(err)
	}
	third := llm.WithLease(t.Context(), "third-owner")
	if ok, err := repository.StartTask(third, value.DiagnosisID, "mock-c", "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	recovered, err := repository.ClaimApproval(third, value.DiagnosisID, value.ID)
	if err != nil || recovered.ConnectionID != 123 || recovered.Plan.Size != 30 {
		t.Fatal("执行线索未恢复")
	}
	result := tools.Result{Tool: value.Tool, Status: "ok", Data: json.RawMessage(`{}`)}
	if err = repository.FinishApproval(next, value.DiagnosisID, value.ID, result); err == nil {
		t.Fatal("旧执行者覆盖了结果")
	}
	if err = repository.FinishApproval(third, value.DiagnosisID, value.ID, result); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.ClaimApproval(third, value.DiagnosisID, value.ID); err == nil {
		t.Fatal("已执行单再次执行")
	}
}

// TestIntegrationApprovalExpiry 过期审批无法批准，取消后的审批也不能重新激活任务。
func TestIntegrationApprovalExpiry(t *testing.T) {
	for _, test := range []string{"过期", "取消"} {
		t.Run(test, func(t *testing.T) {
			repository, target := integrationStore(t)
			ctx, value := approvalFixture(t, repository, target)
			if err := repository.EndTask(ctx, value.DiagnosisID, run.WaitingApproval, agent.Result{}, "等待"); err != nil {
				t.Fatal(err)
			}
			if test == "过期" {
				// 只移动这一张测试单；到期判定仍由真实条件更新负责。
				if _, err := repository.database.ExecContext(t.Context(), "UPDATE approvals SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", value.ID); err != nil {
					t.Fatal(err)
				}
				if ok, err := repository.DecideApproval(t.Context(), value.ID, "test-user", true); err != nil || ok {
					t.Fatal("过期仍能批准")
				}
				if err := repository.Recover(t.Context()); err != nil {
					t.Fatal(err)
				}
				saved, err := repository.FindApproval(t.Context(), value.DiagnosisID, value.ID)
				if err != nil || saved.Status != "expired" {
					t.Fatal("没有标记过期")
				}
				if tasks, err := repository.Pending(t.Context()); err != nil || len(tasks) != 1 {
					t.Fatal("过期结果没有机会交回模型")
				}
			} else {
				if ok, err := repository.CancelPending(t.Context(), value.DiagnosisID, "用户取消"); err != nil || !ok {
					t.Fatal(err)
				}
				if ok, err := repository.DecideApproval(t.Context(), value.ID, "test-user", true); err != nil || ok {
					t.Fatal("取消后仍可批准")
				}
			}
		})
	}
}
