// 本文件：用阻塞的假模型验证配对任务选对模型，并且不会跨账户停用。
//
// 主要内容：TestExecutionSelection 检查白名单和模型一致；TestExecutionBudgets
// 检查默认账户用完后另一配置仍可运行；TestMockCannotEnablePaid 检查免费总开关。
// 阅读建议：provider=openai 只用于测试预算分支，实际客户端仍是内存里的 blockingModel。

package run

import (
	"context"
	"testing"
	"testing/synctest"
)

// TestExecutionSelection 不接受模型自行改名、未知配置或给普通诊断附带评测配置。
func TestExecutionSelection(t *testing.T) {
	scheduler := testScheduler(t, &memoryTasks{}, "mock", false)
	scheduler.config.Executions = map[string]Execution{"second": {Model: "mock-second"}}
	for _, test := range []struct {
		name, profile, model, group string
		valid                       bool
	}{
		{"默认任务", "", "", "", true},                      // 原有手动和告警入口不变。
		{"第二模型", "second", "mock-second", "E1", true},   // 任务与白名单一致。
		{"换模型", "second", "mock-other", "E1", false},    // 不能把结果归到另一模型名。
		{"未知配置", "missing", "mock-second", "E1", false}, // 不回退到默认配置。
		{"普通诊断冒用", "second", "mock-second", "", false},  // 配对入口仅用于评测。
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := scheduler.executionFor(Task{ExecutionProfile: test.profile, ExpectedModel: test.model, EvaluationGroup: test.group})
			if (err == nil) != test.valid {
				t.Fatalf("允许=%t err=%v", test.valid, err)
			}
		})
	}
}

// TestExecutionBudgets 使用虚拟时间等到实际调度完成，不能只检查选择函数返回了什么。
func TestExecutionBudgets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first, second := taskForTest("first", "A"), taskForTest("second", "B")
		second.task.ExecutionProfile, second.task.ExpectedModel, second.task.EvaluationGroup = "second", "mock-test", "E1"
		memory := &memoryTasks{items: []*queued{first, second}}
		scheduler := testScheduler(t, memory, "openai", true)
		// 默认账户没有额度，第二账户有额度；两者都调用同一个不联网的假模型。
		scheduler.config.Executions = map[string]Execution{"second": {Model: "mock-test", Provider: "openai", Agent: scheduler.agent, Repository: memory, Budget: func(context.Context) (BudgetStatus, error) {
			return BudgetStatus{Currency: "CNY", LimitMicros: 10_000_000}, nil
		}}}
		if err := scheduler.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		scheduler.Close()
		if first.status != Pending || second.status != Running {
			t.Fatalf("默认=%s，第二配置=%s", first.status, second.status)
		}
	})
}

// TestMockCannotEnablePaid 阻止未来装配错误把真实客户端藏在 Mock 页面后面。
func TestMockCannotEnablePaid(t *testing.T) {
	scheduler := testScheduler(t, &memoryTasks{}, "mock", false)
	configuration := scheduler.config
	configuration.Executions = map[string]Execution{"paid": {Model: "paid-model", Provider: "openai", Agent: scheduler.agent, Repository: scheduler.repository, Budget: configuration.Budget}}
	if _, err := New(configuration, scheduler.repository, scheduler.agent, scheduler.logger); err == nil {
		t.Fatal("Mock 总开关允许了真实执行配置")
	}
}
