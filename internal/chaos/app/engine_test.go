// 本文件：用假的外部执行器验证实验失败时的恢复顺序。
//
// 测试不会改数据库或容器，只用临时目录保存恢复记录。
// 它专门覆盖取消、部分失败与第二个实验企图覆盖记录的情况。
//
// 主要内容：
//   - fakeExecutor、fakeObserver：按测试安排返回结果并记录恢复情况。
//   - TestTrialRestoresAfterCancellation：取消实验后仍有独立的恢复预算。
//   - TestRestoreContinuesAndRetainsRecord：失败后继续清理，并保留恢复依据。
//   - TestInjectDoesNotOverwrite：同一时刻只允许一个活动场景。
//
// 阅读建议：先看各测试想保护的顺序，再看假执行器如何记录调用。

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/action"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/verify"
)

// fakeExecutor 保留操作次数，测试通过钩子安排某一步取消或失败。
type fakeExecutor struct {
	onExecute       func() error // 可选地模拟注入过程中取消。
	restores        int          // 恢复失败也要计数，便于判断后续清理是否执行。
	restoreError    error        // 非空时每次恢复都返回这个错误。
	restoreCanceled bool         // 记录恢复是否误用了已经取消的父 context。
}

// Capture 返回无秘密的假快照，测试不需要实际部署数据。
func (fake *fakeExecutor) Capture(context.Context, scenario.Scenario) (action.Baseline, error) {
	return action.Baseline{QPS: 200}, nil
}

// Execute 按测试安排失败；未设置钩子时表示动作成功。
func (fake *fakeExecutor) Execute(context.Context, scenario.Step) error {
	if fake.onExecute != nil {
		return fake.onExecute()
	}
	return nil
}

// Restore 即使返回错误也记录自己执行过，用来验证不能提前放弃其他清理。
func (fake *fakeExecutor) Restore(ctx context.Context, _ scenario.Step, _ action.Baseline) error {
	fake.restores++
	fake.restoreCanceled = fake.restoreCanceled || ctx.Err() != nil
	return fake.restoreError
}

// fakeObserver 省去真实的一分钟等待，只验证流程如何使用监控结果。
type fakeObserver struct{}

// Healthy 在流程测试中立即通过，具体健康判断由 verify 包的测试覆盖。
func (fakeObserver) Healthy(context.Context) (verify.Result, error) { return verify.Result{}, nil }

// Symptom 返回一条假观察，避免把流程测试变成 Prometheus 协议测试。
func (fakeObserver) Symptom(context.Context, scenario.Symptom) (verify.Result, error) {
	return verify.Result{SymptomSeen: true, Alerts: []string{"Observed"}}, nil
}

// testEngine 为每个测试分配独立状态路径，测试间不会抢同一份恢复记录。
func testEngine(t *testing.T, fake *fakeExecutor) (*Engine, scenario.Scenario) {
	t.Helper()
	value := scenario.Scenario{ID: "S06", Inject: []scenario.Step{{Action: "stop", Service: "order-worker"}}, Restore: []scenario.Step{{Action: "start", Service: "order-worker"}}}
	return &Engine{Executor: fake, Observer: fakeObserver{}, StatePath: filepath.Join(t.TempDir(), "state.json")}, value
}

// TestTrialRestoresAfterCancellation 确认用户中断不会让已经生效的外部动作无人恢复。
func TestTrialRestoresAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fake := &fakeExecutor{onExecute: func() error { cancel(); return context.Canceled }}
	engine, value := testEngine(t, fake)
	row := engine.Trial(ctx, value)
	if !errors.Is(row.Problem, context.Canceled) || !row.Restored || fake.restores != 1 || fake.restoreCanceled {
		t.Fatalf("row=%+v fake=%+v", row, fake)
	}
	if _, err := os.Stat(engine.StatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("成功恢复后仍有记录：%v", err)
	}
}

// TestRestoreContinuesAndRetainsRecord 一个恢复失败不能挡住后续动作，也不能删除重试依据。
func TestRestoreContinuesAndRetainsRecord(t *testing.T) {
	fake := &fakeExecutor{restoreError: errors.New("模拟外部恢复失败")}
	engine, value := testEngine(t, fake)
	value.Restore = append(value.Restore, scenario.Step{Action: "clear_latency", Proxy: "order-inventory"})
	record, err := engine.Inject(t.Context(), value)
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.Restore(t.Context(), record); err == nil || fake.restores != 2 {
		t.Fatalf("restores=%d err=%v", fake.restores, err)
	}
	if _, err = os.Stat(engine.StatePath); err != nil {
		t.Fatal("失败后丢了恢复记录")
	}
	// 外部系统恢复后可以重跑相同恢复动作，全部成功才删除记录。
	fake.restoreError = nil
	if err = engine.Restore(t.Context(), record); err != nil {
		t.Fatal(err)
	}
}

// TestInjectDoesNotOverwrite 第二次注入必须失败，原场景和起始时间都不能改变。
func TestInjectDoesNotOverwrite(t *testing.T) {
	fake := &fakeExecutor{}
	engine, value := testEngine(t, fake)
	first, err := engine.Inject(t.Context(), value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Inject(t.Context(), value); err == nil {
		t.Fatal("覆盖了活动实验")
	}
	actual, err := engine.Read()
	if err != nil || !actual.Started.Equal(first.Started) {
		t.Fatalf("record=%+v err=%v", actual, err)
	}
}
