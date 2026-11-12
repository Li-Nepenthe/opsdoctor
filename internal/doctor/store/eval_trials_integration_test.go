// 本文件：重读每个故障检查点模拟命令重启。恢复时间、旧中断和状态推进顺序都应保留，避免把一次重试算成新注入。

package store

import (
	"testing"
	"time"
)

// TestTrialTransition 完成和未知状态没有后继，错误不会变成“已完成”。
func TestTrialTransition(t *testing.T) {
	for _, test := range []struct {
		from, to string
		valid    bool
	}{
		{"pending", "injecting", true},    // 注入前先落盘。
		{"injecting", "verifying", true},  // 有动作记录仍需验证症状。
		{"diagnosing", "restoring", true}, // 全部组结束才恢复。
		{"restoring", "cooling", true},    // 恢复后等待证据窗口过去。
		{"pending", "done", false},        // 不能跳过整轮。
		{"diagnosing", "pending", false},  // 不能重新注入挑结果。
		{"done", "injecting", false},      // 已完成不会再花钱。
		{"unknown", "", false},            // 未识别状态必须拒绝。
	} {
		if trialTransition(test.from, test.to) != test.valid {
			t.Fatalf("%s -> %s", test.from, test.to)
		}
	}
}

// TestIntegrationEvalTrial 在每一步重新读取，模拟执行器退出后重建进程。
func TestIntegrationEvalTrial(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := t.Context()
	// 恢复文件使用本地时区，纳秒尾数会被 MySQL 向上取整；这里用同样的时间核对续跑。
	fault := time.Date(2026, 10, 29, 5, 20, 2, 15277600, time.FixedZone("test", -7*3600))
	for _, next := range []string{"injecting", "verifying", "diagnosing", "restoring", "cooling", "done"} {
		value, err := repository.EvalTrial(ctx, input.RunID, "S01", 1)
		if err != nil {
			t.Fatal(err)
		}
		var started *time.Time
		if next == "verifying" {
			started = &fault
		}
		if err = repository.AdvanceEvalTrial(ctx, value, next, started); err != nil {
			t.Fatal(err)
		}
		if err = repository.AdvanceEvalTrial(ctx, value, next, started); err == nil {
			t.Fatal("旧进度重复推进")
		}
		current, err := repository.EvalTrial(ctx, input.RunID, "S01", 1)
		if err != nil || current.State != next {
			t.Fatal(current, err)
		}
		if next != "injecting" && (current.FaultStartedAt == nil || !current.FaultStartedAt.Equal(fault.UTC().Truncate(time.Microsecond))) {
			t.Fatal("重启丢失原故障时间")
		}
		if next == "cooling" && (current.RestoredAt == nil || current.CooldownSeconds < 299) {
			t.Fatal("恢复后没有五分钟冷却")
		}
	}
	value, err := repository.EvalTrial(ctx, input.RunID, "S01", 1)
	if err != nil {
		t.Fatal(err)
	}
	// 两次失败各保留一行；原因会脱敏，不能把凭据带进公开结果。
	for range 2 {
		if err = repository.InterruptEval(ctx, value, "network password=private"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = repository.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM eval_interruptions WHERE run_id=? AND reason NOT LIKE '%private%'", input.RunID).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
}
