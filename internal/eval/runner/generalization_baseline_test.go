// 本文件：只用假依赖检查阶段9的基线等待，不接触真实Docker、医生或模型。
//
// 每轮重新取得全部健康结果，查询错误和配置错误不重试；等待失败也留下原证据。
// 测试的等待替身会确认生产仍要求五秒，短父上下文用于验证超时而不真的等五分钟。
//
// 主要内容：TestDemoBaselineHealthy检查同轮通过；TestDemoBaselineTimeout检查超时；
// TestDemoBaselineQueryError和TestDemoBaselineFlags检查失败顺序，其他用例检查证据硬停。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// baselineFixture只提供本地函数，生产依赖若被错误使用会使测试立即失败。
func baselineFixture(t *testing.T) (generalizationCommand, map[string]demoScenario, demoBaselineSources) {
	t.Helper()
	options := generalizationCommand{command: command{runID: "phase09-baseline-test", target: "otel-demo", scenario: "OT01", iteration: 1, cases: []string{"OT01", "OT02", "OT03", "OT04", "OT05"}}, baseline: json.RawMessage(`{"flags":{"test":"original"}}`)}
	cases := map[string]demoScenario{}
	for _, name := range options.cases {
		cases[name] = demoScenario{HealthyPromQL: "healthy-" + name}
	}
	sources := demoBaselineSources{root: t.TempDir(),
		memory: func(context.Context) error { return nil },
		flags:  func(context.Context) (json.RawMessage, error) { return options.baseline, nil },
		fault:  func(context.Context) (bool, error) { return false, nil },
		query:  func(context.Context, string) (bool, error) { return true, nil },
		wait:   func(context.Context, time.Duration) error { t.Fatal("不应重复等待"); return nil },
	}
	return options, cases, sources
}

// readBaselineObservation核对落盘格式，不把内存返回值当作独占证据。
func readBaselineObservation(t *testing.T, root, runID, phase string) demoBaselineObservation {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, runID, "baseline-"+phase+"-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatal("没有唯一基线证据", paths, err)
	}
	raw, err := os.ReadFile(paths[0])
	var value demoBaselineObservation
	if err != nil || json.Unmarshal(raw, &value) != nil {
		t.Fatal("基线证据不可读", err)
	}
	if !value.Known || value.StartUTC.IsZero() || value.FinishedUTC.Before(value.StartUTC) || value.ElapsedNS < 0 || value.ElapsedSeconds < 0 {
		t.Fatal("单调用时证据不完整", value)
	}
	return value
}

// TestDemoBaselineHealthy先出现0/0式无健康样本，后一轮五场景同时通过才继续。
func TestDemoBaselineHealthy(t *testing.T) {
	options, cases, sources := baselineFixture(t)
	order := []string{}
	queries, waits, faults := 0, 0, 0
	sources.memory = func(context.Context) error { order = append(order, "memory"); return nil }
	sources.flags = func(context.Context) (json.RawMessage, error) {
		order = append(order, "flags")
		return options.baseline, nil
	}
	sources.query = func(ctx context.Context, query string) (bool, error) {
		queries++
		order = append(order, query)
		deadline, ok := ctx.Deadline()
		if remaining := time.Until(deadline); !ok || remaining > demoObservationLimit || remaining < demoObservationLimit-5*time.Second {
			t.Error("健康查询没有五分钟总期限")
		}
		return queries > 5 || query != "healthy-OT03" && query != "healthy-OT04", nil
	}
	// 生产要求五秒一次；测试只替换等待动作，不改健康谓词或轮次结果。
	sources.wait = func(_ context.Context, d time.Duration) error {
		waits++
		if d != 5*time.Second {
			t.Error("轮询间隔被改动", d)
		}
		return nil
	}
	sources.fault = func(context.Context) (bool, error) { faults++; order = append(order, "fault"); return false, nil }
	if err := runDemoBaseline(t.Context(), options, cases, "pre-inject", sources); err != nil {
		t.Fatal(err)
	}
	value := readBaselineObservation(t, sources.root, options.runID, "pre-inject")
	if queries != 10 || waits != 1 || faults != 1 || value.Polls != 2 || value.Queries != 10 || value.Status != "passed" || value.PassedUTC == nil || !reflect.DeepEqual(value.Cases, options.cases) {
		t.Fatal("未等同轮全部通过", queries, waits, faults, value)
	}
	if !value.HealthKnown || value.HealthStartUTC == nil || value.HealthFinishedUTC == nil || value.HealthElapsedNS == nil || *value.HealthElapsedNS < 0 || *value.HealthElapsedNS > value.ElapsedNS {
		t.Fatal("健康轮询与整次调用的耗时未分开", value)
	}
	if order[0] != "memory" || order[1] != "flags" || order[len(order)-1] != "fault" {
		t.Fatal("改变原资源/开关/健康/开放期顺序", order)
	}
}

// TestDemoBaselineTimeout不同轮各有失败，不能把先前的健康缓存拼成全部通过。
func TestDemoBaselineTimeout(t *testing.T) {
	options, cases, sources := baselineFixture(t)
	queries := 0
	sources.query = func(_ context.Context, query string) (bool, error) {
		round := queries / 5
		queries++
		return !(round == 0 && query == "healthy-OT01" || round == 1 && query == "healthy-OT02"), nil
	}
	// 先确定执行两轮，再由短父上下文中止真实等待，避免测试真的花五分钟。
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	sources.wait = func(ctx context.Context, d time.Duration) error {
		if queries < 10 {
			return nil
		}
		return waitTick(ctx, d)
	}
	sources.fault = func(context.Context) (bool, error) { t.Fatal("健康未通过却检查开放期"); return false, nil }
	err := runDemoBaseline(ctx, options, cases, "pre-inject", sources)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("持续不健康没有超时", err)
	}
	value := readBaselineObservation(t, sources.root, options.runID, "pre-inject")
	if queries != 10 || value.Polls != 2 || value.Queries != 10 || value.Status != "failed" || value.PassedUTC != nil {
		t.Fatal("拼接不同轮健康或丢失失败观察", queries, value)
	}
}

// TestDemoBaselineQueryError查询错误立即停止，不等下一轮，也不继续本轮剩余查询。
func TestDemoBaselineQueryError(t *testing.T) {
	// 第二条查询失败以后，剩余场景、轮询等待和故障开放期都不能继续。
	options, cases, sources := baselineFixture(t)
	queries := 0
	problem := errors.New("模拟查询失败")
	sources.query = func(context.Context, string) (bool, error) {
		queries++
		if queries == 2 {
			return false, problem
		}
		return false, nil
	}
	sources.fault = func(context.Context) (bool, error) { t.Fatal("查询失败仍查开放期"); return false, nil }
	if err := runDemoBaseline(t.Context(), options, cases, "pre-inject", sources); !errors.Is(err, problem) {
		t.Fatal("查询错误被重试或吞掉", err)
	}
	value := readBaselineObservation(t, sources.root, options.runID, "pre-inject")
	if queries != 2 || value.Polls != 1 || value.Queries != 2 || value.Status != "failed" {
		t.Fatal("没有立即停止", queries, value)
	}
}

// TestDemoBaselineFlags开关不一致在任何健康查询之前拒绝，freeze无需已有计时上下文。
func TestDemoBaselineFlags(t *testing.T) {
	options, cases, sources := baselineFixture(t)
	parsed, err := parseGeneralization([]string{"freeze", "--run", options.runID}, io.Discard)
	if err != nil || parsed.iteration != 1 {
		t.Fatal("未覆盖真实CLI的freeze缺省轮次", parsed.iteration, err)
	}
	parsed.baseline = options.baseline
	options = parsed
	sources.flags = func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"flags":{"test":"different"}}`), nil
	}
	sources.query = func(context.Context, string) (bool, error) { t.Fatal("开关不一致仍查健康"); return false, nil }
	sources.fault = func(context.Context) (bool, error) { t.Fatal("开关不一致仍查开放期"); return false, nil }
	// 使用没有withDemoTiming的上下文，仍应保存这次失败的单调计时。
	if err := runDemoBaseline(t.Context(), options, cases, "freeze", sources); err == nil {
		t.Fatal("错误配置被放行")
	}
	value := readBaselineObservation(t, sources.root, options.runID, "freeze")
	if value.Scenario != "" || value.Iteration != 0 || value.Status != "failed" || value.Polls != 0 || value.Queries != 0 || value.HealthKnown || value.HealthStartUTC != nil || value.HealthElapsedNS != nil {
		t.Fatal("freeze身份或零查询不符", value)
	}
}

// TestDemoBaselineQueryDeadline即使查询晚返回true，也不能越过整段健康等待的期限。
func TestDemoBaselineQueryDeadline(t *testing.T) {
	options, cases, sources := baselineFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	queries := 0
	sources.query = func(ctx context.Context, _ string) (bool, error) { queries++; <-ctx.Done(); return true, nil }
	err := runDemoBaseline(ctx, options, cases, "pre-inject", sources)
	if !errors.Is(err, context.DeadlineExceeded) || queries != 1 {
		t.Fatal("查询耗时未受总期限约束", err, queries)
	}
	value := readBaselineObservation(t, sources.root, options.runID, "pre-inject")
	if value.Status != "failed" || value.Polls != 1 || value.Queries != 1 {
		t.Fatal(value)
	}
}

// TestDemoBaselineEvidence健康后仍只查一次开放期；证据目录失败也必须阻止推进。
func TestDemoBaselineEvidence(t *testing.T) {
	options, cases, sources := baselineFixture(t)
	faults := 0
	sources.fault = func(context.Context) (bool, error) { faults++; return true, nil }
	if err := runDemoBaseline(t.Context(), options, cases, "post-cool", sources); err == nil {
		t.Fatal("开放故障被忽略")
	}
	value := readBaselineObservation(t, sources.root, options.runID, "post-cool")
	if faults != 1 || value.Polls != 1 || value.Queries != 5 || value.Status != "failed" {
		t.Fatal("开放期检查被重复或提前", faults, value)
	}
	// 后一次调用必须新增独占文件，不能覆盖前次失败的字节。
	sources.fault = func(context.Context) (bool, error) { return false, nil }
	if err := runDemoBaseline(t.Context(), options, cases, "post-cool", sources); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(sources.root, options.runID, "baseline-post-cool-*.json"))
	if len(paths) != 2 {
		t.Fatal("重复基线覆盖了旧原件", paths)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("不能覆盖"), 0600); err != nil {
		t.Fatal(err)
	}
	sources.root = blocked
	if err := runDemoBaseline(t.Context(), options, cases, "freeze", sources); err == nil || !strings.Contains(err.Error(), "证据未能独占保存") {
		t.Fatal("健康但证据无法保存仍被放行", err)
	}
}
