// 本文件：用独立测试库检查时间来源和恢复后的未知耗时。
//
// 主要内容：TestIntegrationDiagnosisTiming 故意把进程墙钟拨到另一天，验证诊断
// 时间戳仍来自数据库；恢复用例检查最后一段时长不会冒充完整时长。
// 阅读建议：测试只移动本次任务的租约，不修改系统时钟或历史诊断。

package store

import (
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
)

// TestIntegrationDiagnosisTiming 单段执行有独立耗时，跨进程接手则明确缺少完整测量。
func TestIntegrationDiagnosisTiming(t *testing.T) {
	for _, resumed := range []bool{false, true} {
		name := "单段执行"
		if resumed {
			name = "接手旧租约"
		}
		t.Run(name, func(t *testing.T) {
			repository, target := integrationStore(t)
			repository.config.Now = func() time.Time { return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) }
			id, err := repository.Manual(t.Context(), target, "时间测试")
			if err != nil {
				t.Fatal(err)
			}
			ctx := llm.WithLease(t.Context(), "first")
			if ok, err := repository.StartTask(ctx, id, "mock-clock", "test", "mock"); err != nil || !ok {
				t.Fatalf("start=%v %v", ok, err)
			}
			if resumed {
				// 只让本次任务的租约过期，随后用新编号接手，模拟另一个进程。
				if _, err := repository.database.ExecContext(t.Context(), "UPDATE diagnoses SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
				ctx = llm.WithLease(t.Context(), "second")
				if ok, err := repository.StartTask(ctx, id, "mock-clock", "test", "mock"); err != nil || !ok {
					t.Fatalf("resume=%v %v", ok, err)
				}
			}
			if err := repository.EndTask(ctx, id, run.Succeeded, agent.Result{Elapsed: 1250 * time.Millisecond}, ""); err != nil {
				t.Fatal(err)
			}
			detail, err := repository.Detail(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			if detail.FinishedAt == nil || detail.FinishedAt.Year() == 2000 {
				t.Fatal("结束时间仍取了进程墙钟")
			}
			for _, step := range detail.Steps {
				if step.CreatedAt.Year() == 2000 {
					t.Fatal("步骤仍取了进程墙钟")
				}
			}
			if resumed {
				if detail.DurationMS != nil {
					t.Fatal("把恢复后的末段冒充全部耗时")
				}
			} else if detail.DurationMS == nil || *detail.DurationMS != 1250 {
				t.Fatal("未原样保存独立耗时")
			}
		})
	}
}
