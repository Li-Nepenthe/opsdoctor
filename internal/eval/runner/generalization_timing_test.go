// 本文件：用临时时间和文件验证外部场景的首次观察证据。
//
// 测试不访问 Docker、HTTP、数据库或模型。重点是单调时钟缺失时不造耗时、
// 症状与恢复各记各的，以及重复写入不能覆盖原件。

package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// timingFixture 把本次测试的证据放进独立临时目录，不接触真实批次。
func timingFixture(t *testing.T) (context.Context, generalizationCommand, *demoTimingState) {
	t.Helper()
	ctx := withDemoTiming(t.Context())
	state := ctx.Value(demoTimingKey{}).(*demoTimingState)
	state.root = t.TempDir()
	options := generalizationCommand{command: command{runID: "phase09-timing-test", scenario: "OT01", iteration: 1}}
	return ctx, options, state
}

// readTimingObservation 读取最终公开格式，断言针对落盘结果而不只看内存状态。
func readTimingObservation(t *testing.T, state *demoTimingState, options generalizationCommand, kind string) (demoTimingObservation, []byte) {
	t.Helper()
	path := filepath.Join(state.root, options.runID, "OT01-1."+kind+"-observed.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value demoTimingObservation
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value, raw
}

// TestDemoTimingKnown 两个阶段各自使用原始单调锚点，恢复不混入故障或冷却耗时。
func TestDemoTimingKnown(t *testing.T) {
	ctx, options, state := timingFixture(t)
	start := time.Now()
	if err := rememberDemoInjection(ctx, options, start); err != nil {
		t.Fatal(err)
	}
	if err := observeDemoSymptom(ctx, options, start.Add(1250*time.Millisecond), 3); err != nil {
		t.Fatal(err)
	}
	// 恢复开始比注入晚很多；正确结果仍只计算恢复后的两秒。
	restored := start.Add(10 * time.Minute)
	if err := rememberDemoRestore(ctx, options, restored); err != nil {
		t.Fatal(err)
	}
	if err := observeDemoRecovery(ctx, options, restored.Add(2*time.Second), 1); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind    string
		start   time.Time
		elapsed time.Duration
		polls   int
	}{{"symptom", start, 1250 * time.Millisecond, 3}, {"recovery", restored, 2 * time.Second, 1}} {
		value, _ := readTimingObservation(t, state, options, test.kind)
		if value.RunID != options.runID || value.Scenario != options.scenario || value.Iteration != options.iteration || value.Kind != test.kind || value.Polls != test.polls || value.StartUTC == nil || !value.StartUTC.Equal(test.start) || !value.PassedUTC.Equal(test.start.Add(test.elapsed)) || !value.Known || value.ElapsedNS == nil || *value.ElapsedNS != test.elapsed.Nanoseconds() || value.ElapsedSeconds == nil || *value.ElapsedSeconds != test.elapsed.Seconds() {
			t.Fatalf("计时身份或单调耗时不符：%+v", value)
		}
	}
}

// TestDemoTimingUnknown 续跑或调用方剥离单调部分时，只写 UTC 和明确的未知标记。
func TestDemoTimingUnknown(t *testing.T) {
	// 既覆盖整个原锚点遗失，也覆盖任一端只剩可序列化的 UTC 时间。
	for _, name := range []string{"没有原锚点", "起点只有UTC", "终点只有UTC"} {
		t.Run(name, func(t *testing.T) {
			ctx, options, state := timingFixture(t)
			start, passed := time.Now(), time.Now().Add(time.Second)
			if name == "起点只有UTC" {
				start = start.UTC()
			}
			if name == "终点只有UTC" {
				passed = passed.UTC()
			}
			if name != "没有原锚点" {
				if err := rememberDemoRestore(ctx, options, start); err != nil {
					t.Fatal(err)
				}
			}
			if err := observeDemoRecovery(ctx, options, passed, 1); err != nil {
				t.Fatal(err)
			}
			value, raw := readTimingObservation(t, state, options, "recovery")
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil || value.Known || value.ElapsedNS != nil || value.ElapsedSeconds != nil || fields["elapsed_ns"] != nil || fields["elapsed_seconds"] != nil {
				t.Fatalf("缺失单调时钟却制造了耗时：%s %v", raw, err)
			}
			if (value.StartUTC == nil) != (name == "没有原锚点") {
				t.Fatalf("UTC 起点可见性不符：%s", raw)
			}
		})
	}
}

// TestDemoTimingPreservesEvidence 重复记起点或观察都不能使原时间缩短、原文件被覆盖。
func TestDemoTimingPreservesEvidence(t *testing.T) {
	ctx, options, state := timingFixture(t)
	start := time.Now()
	if err := rememberDemoInjection(ctx, options, start); err != nil {
		t.Fatal(err)
	}
	if err := rememberDemoInjection(ctx, options, start.Add(time.Second)); err == nil {
		t.Fatal("接受刷新原计时起点")
	}
	if err := observeDemoSymptom(ctx, options, start.Add(2*time.Second), 1); err != nil {
		t.Fatal(err)
	}
	_, before := readTimingObservation(t, state, options, "symptom")
	// 第二次观察更晚且查询次数不同，仍必须保留第一次的原始字节。
	if err := observeDemoSymptom(ctx, options, start.Add(3*time.Second), 2); err == nil {
		t.Fatal("重复写入没有报告证据已存在")
	}
	value, after := readTimingObservation(t, state, options, "symptom")
	if string(before) != string(after) || value.ElapsedNS == nil || *value.ElapsedNS != (2*time.Second).Nanoseconds() {
		t.Fatal("首次观察证据被后来的调用修改")
	}
}

// TestDemoTimingErrors 非法身份、负耗时和文件错误都向上传递，不静默当成观察成功。
func TestDemoTimingErrors(t *testing.T) {
	ctx, options, state := timingFixture(t)
	now := time.Now()
	if err := observeDemoSymptom(t.Context(), options, now, 1); err == nil {
		t.Fatal("没有安装计时状态却继续")
	}
	if err := rememberDemoInjection(ctx, options, time.Time{}); err == nil {
		t.Fatal("接受空起点")
	}
	for _, polls := range []int{0, -1} {
		if err := observeDemoSymptom(ctx, options, now, polls); err == nil {
			t.Fatal("接受无效查询次数")
		}
	}
	invalid := options
	invalid.runID = "../../old-batch"
	if err := observeDemoRecovery(ctx, invalid, now, 1); err == nil {
		t.Fatal("接受恢复目录以外的身份")
	}
	if err := rememberDemoInjection(ctx, options, now); err != nil {
		t.Fatal(err)
	}
	if err := observeDemoSymptom(ctx, options, now.Add(-time.Second), 1); err == nil {
		t.Fatal("接受早于起点的单调观察")
	}
	blocked := filepath.Join(state.root, "blocked")
	// 用普通文件阻断目录创建，确定真实文件系统错误会让调用方停止。
	if err := os.WriteFile(blocked, []byte("保留原件"), 0600); err != nil {
		t.Fatal(err)
	}
	state.root = blocked
	if err := observeDemoRecovery(ctx, options, now, 1); err == nil {
		t.Fatal("证据目录写入失败却继续")
	}
}
