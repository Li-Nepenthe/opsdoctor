// 本文件：检查恢复文件不能把另一场实验接到本轮。
//
// 主要内容：TestMatchRecord 验证场景、注入时刻与缺失记录；TestNestedClockGuard
// 验证长跑、矩阵和单条诊断共用一个采样器。
// 阅读建议：这些测试只用临时文件，不启动 Docker，也不调用模型。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	chaos "github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMatchRecord 同一个 S01 的另一次注入也必须拒绝，不能仅凭场景名续跑。
func TestMatchRecord(t *testing.T) {
	started := time.Now().UTC().Truncate(time.Microsecond)
	engine := chaos.Engine{StatePath: filepath.Join(t.TempDir(), "state.json")}
	raw, _ := json.Marshal(chaos.Record{Scenario: scenario.Scenario{ID: "S01"}, Started: started})
	if err := os.WriteFile(engine.StatePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	later := started.Add(time.Minute)
	for _, test := range []struct {
		name  string
		start *time.Time
		valid bool
	}{
		{"S01", &started, true},  // 原轮次。
		{"S01", nil, true},       // 注入成功但数据库尚未收到确认。
		{"S02", &started, false}, // 另一场景。
		{"S01", &later, false},   // 同场景重新注入过。
	} {
		if _, err := matchRecord(engine, store.EvalTrial{Scenario: test.name, FaultStartedAt: test.start}); (err == nil) != test.valid {
			t.Fatal(test, err)
		}
	}
	if err := os.Remove(engine.StatePath); err != nil {
		t.Fatal(err)
	}
	if _, err := matchRecord(engine, store.EvalTrial{Scenario: "S01"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

// TestNestedClockGuard 子流程结束不能把外层守卫停掉，外层仍持续保护下一组。
func TestNestedClockGuard(t *testing.T) {
	ctx, stop := guardClock(t.Context(), func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	defer stop()
	nested, finish := guardClock(ctx, func(context.Context) error { t.Error("重复采样"); return errors.New("duplicate") })
	finish()
	if nested != ctx || ctx.Err() != nil {
		t.Fatal("子流程终止了外层守卫")
	}
}
