// 本文件：默认模拟Docker输出，验证每份诊断的采集审计不会把缺证据当作正常。
//
// 单元测试不启动容器或模型。固定窗口检查最终丢弃、嵌套错误、轮转、重启与不可变证据。
// 只有显式设置TEST_PHASE09_COLLECTOR_DIR时，集成用例才只读现有Collector并保留真实原件；
// 它不访问数据库、模型或故障开关，CI未设置该变量时跳过。

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
)

// collectorFixture固定输出与实际Collector相同的Docker前缀、制表符和JSON附加字段。
func collectorFixture(at time.Time, message, signal, component, problem string, dropped *int64) string {
	fields := map[string]any{"otelcol.signal": signal, "otelcol.component.id": component}
	if problem != "" {
		fields["error"] = problem
	}
	if dropped != nil {
		fields["dropped_items"] = *dropped
	}
	raw, _ := json.Marshal(fields)
	return at.Format(time.RFC3339Nano) + " " + at.Format(time.RFC3339Nano) + "\terror\tinternal/queue_sender.go:62\t" + message + "\t" + string(raw) + "\n"
}

// TestCollectorCounts将最终丢弃和Will retry放在同一窗口，批次计数绝不与重试相加。
func TestCollectorCounts(t *testing.T) {
	start := time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)
	items := int64(7)
	metricItems := int64(20)
	raw := collectorFixture(start.Add(time.Second), "Exporting failed. Dropping data.", "logs", "opensearch", "mapper_parsing_exception; mapper_parsing_exception", &items)
	raw += collectorFixture(start.Add(2*time.Second), "Exporting failed. Dropping data.", "metrics", "otlp_http/prometheus", "permanent error", &metricItems)
	raw += collectorFixture(start.Add(3*time.Second), "Exporting failed. Will retry the request after interval.", "traces", "otlp/jaeger", "retryable", nil)
	// error字段里的文字不是第二个真实事件；栈帧也不重复增加事件数。
	raw += collectorFixture(start.Add(4*time.Second), "Exporting failed. Will retry the request after interval.", "logs", "opensearch", `nested: Exporting failed. Dropping data. {"dropped_items":999}`, nil)
	raw += start.Add(4*time.Second).Format(time.RFC3339Nano) + " go.opentelemetry.io/collector/exporter/exporterhelper.New.func1\n"
	raw += start.Add(4*time.Second).Format(time.RFC3339Nano) + " \tgo.opentelemetry.io/collector/exporter@v0.159.0/exporter.go:42\n"
	audit := report.CollectorAudit{WindowStart: start, WindowEnd: start.Add(time.Minute), SignalDroppedItems: map[string]int64{}, SignalDroppingDataEvents: map[string]int64{}}
	if err := parseCollectorWindow([]byte(raw), &audit); err != nil {
		t.Fatal(err)
	}
	if audit.DroppingDataEvents != 2 || audit.LogDroppedItems != 7 || audit.SignalDroppedItems["metrics"] != 20 || audit.SignalDroppingDataEvents["logs"] != 1 || audit.SignalDroppedItems["traces"] != 0 || audit.MapperErrorLines != 1 || audit.MapperErrorOccurrences != 2 {
		t.Fatalf("最终事件与重试或错误次数混算：%+v", audit)
	}
	// 在固定版真实原件上再对照既有6209口径，防止夹具与真实格式一起写错。
	path := filepath.Join("..", "..", "..", "docs", "reports", "phase-09-collector-final-before-fix.log")
	if actual, err := os.ReadFile(path); err == nil {
		audit = report.CollectorAudit{WindowStart: time.Date(2026, 11, 3, 4, 40, 0, 0, time.UTC), WindowEnd: time.Date(2026, 11, 3, 5, 5, 0, 0, time.UTC), SignalDroppedItems: map[string]int64{}, SignalDroppingDataEvents: map[string]int64{}}
		if err = parseCollectorWindow(actual, &audit); err != nil {
			t.Fatal("真实格式无法解析", err)
		}
		if audit.LogDroppedItems != 6209 || audit.DroppingDataEvents != 320 || audit.MapperErrorOccurrences != 2293 {
			t.Fatalf("真实原件计数不符：%+v", audit)
		}
	}
}

// TestCollectorMalformed明确把空输出、截断JSON和坏计数归入不可验证，不能得出零丢弃结论。
func TestCollectorMalformed(t *testing.T) {
	start := time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)
	negative := int64(-1)
	valid := collectorFixture(start.Add(time.Second), "Exporting failed. Dropping data.", "logs", "opensearch", "x", &negative)
	for _, raw := range []string{"", "no timestamp\n", start.Format(time.RFC3339Nano) + " not-json\n", valid, strings.Replace(valid, `"dropped_items":-1`, `"dropped_items":1.5`, 1), strings.Replace(valid, `"dropped_items":-1`, `"dropped_items":9223372036854775808`, 1), collectorFixture(start.Add(-time.Second), "ok", "logs", "opensearch", "", nil)} {
		audit := report.CollectorAudit{WindowStart: start, WindowEnd: start.Add(time.Minute), SignalDroppedItems: map[string]int64{}, SignalDroppingDataEvents: map[string]int64{}}
		if err := parseCollectorWindow([]byte(raw), &audit); err == nil {
			t.Fatalf("坏证据被接受：%q", raw)
		}
	}
}

// collectorFake模拟固定容器，按请求区分锚点和窗口，不调用本机Docker。
type collectorFake struct {
	start       time.Time // 每个测试都用固定边界，不受当前时钟影响。
	anchor      string    // 起点前的完整日志行。
	raw         string    // 真正审计的诊断窗口输出。
	mode        string    // 单独破坏一项证明。
	inspections int       // 第二次inspect才模拟诊断中途重启。
}

// run只接受实现承诺的inspect与logs参数，测试也验证容器名固定。
func (fake *collectorFake) run(_ context.Context, args ...string) ([]byte, error) {
	// 前后两次身份查询使用同一假容器，单独的模式才引入进程变化。
	if args[len(args)-1] != collectorContainer {
		return nil, errors.New("wrong container")
	}
	if args[0] == "inspect" {
		fake.inspections++
		value := collectorIdentity{ID: strings.Repeat("a", 64), Started: fake.start.Add(-time.Hour), Running: true}
		if fake.mode == "restart" && fake.inspections > 1 {
			value.Restarts = 1
		}
		if fake.mode == "late-start" {
			value.Started = fake.start.Add(time.Second)
		}
		return json.Marshal(value)
	}
	if args[0] != "logs" || args[1] != "--timestamps" {
		return nil, errors.New("unexpected command")
	}
	if slices.Contains(args, "--tail") {
		return []byte(fake.anchor + "\n"), nil
	}
	if args[3] == fake.start.Format(time.RFC3339Nano) {
		if fake.mode == "command-error" {
			return []byte(fake.raw), errors.New("simulated docker failure")
		}
		return []byte(fake.raw), nil
	}
	if fake.mode == "rotation" {
		return []byte(""), nil
	}
	return []byte(fake.anchor + "\n"), nil
}

// TestCollectorWindow验证正常、丢弃与不可验证均保留原始字节，原件和起点不能被续跑覆盖。
func TestCollectorWindow(t *testing.T) {
	start := time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)
	for _, mode := range []string{"normal", "drop", "mapper-only", "metrics-drop", "empty", "restart", "rotation", "command-error"} {
		t.Run(mode, func(t *testing.T) {
			fake := collectorFake{start: start, anchor: start.Add(-time.Second).Format(time.RFC3339Nano) + " retained anchor", mode: mode}
			fake.raw = collectorFixture(start.Add(time.Second), "Exporting failed. Will retry the request after interval.", "traces", "otlp/jaeger", "temporary", nil)
			items := int64(3)
			if mode == "drop" {
				fake.raw += collectorFixture(start.Add(2*time.Second), "Exporting failed. Dropping data.", "logs", "opensearch", "permanent", &items)
			}
			if mode == "metrics-drop" {
				fake.raw += collectorFixture(start.Add(2*time.Second), "Exporting failed. Dropping data.", "metrics", "otlp_http/prometheus", "permanent", &items)
			}
			if mode == "mapper-only" {
				fake.raw += collectorFixture(start.Add(2*time.Second), "Exporting failed. Will retry the request after interval.", "logs", "opensearch", "mapper_parsing_exception", nil)
			}
			if mode == "empty" {
				fake.raw = ""
			}
			root := t.TempDir()
			window, err := beginCollectorWindow(t.Context(), "phase09-test/OT01/1/primary-E1", "", root, start, fake.run)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(window.Prefix + ".start.json")
			audit := window.finish(t.Context(), start.Add(time.Minute))
			if err = verifyCollectorRaw(audit); err != nil {
				t.Fatal(err)
			}
			got := report.CollectionQuality(report.Summary{DataQuality: report.QualityNormal, CollectorAudit: audit})
			want := report.QualityUnknown
			if mode == "normal" {
				want = report.QualityNormal
			}
			if mode == "drop" || mode == "mapper-only" || mode == "metrics-drop" {
				want = report.QualityAffected
			}
			if got != want {
				t.Fatalf("mode=%s quality=%s want=%s audit=%+v", mode, got, want, audit)
			}
			// 第二次启动不能刷新时间，第二次结束也不能用正常日志洗掉原结果。
			fake.raw, fake.mode = collectorFixture(start.Add(3*time.Second), "ok", "logs", "opensearch", "", nil), "normal"
			resumed, err := beginCollectorWindow(t.Context(), window.Key, "diagnosis-existing", root, start.Add(time.Hour), fake.run)
			if err != nil || !resumed.Start.Equal(start) {
				t.Fatal("续跑刷新了原始起点", err)
			}
			again := resumed.finish(t.Context(), start.Add(2*time.Minute))
			if again.SHA256 != audit.SHA256 || !again.WindowEnd.Equal(audit.WindowEnd) || again.Status != audit.Status {
				t.Fatal("续跑覆盖了原始窗口", again)
			}
			after, _ := os.ReadFile(window.Prefix + ".start.json")
			if string(before) != string(after) {
				t.Fatal("原始起点被改写")
			}
		})
	}
}

// TestCollectorGate确保数据质量在评分前拦截，原始日志被改写后也不能继续花钱。
func TestCollectorGate(t *testing.T) {
	// 先取得可验证原件，再分别破坏质量标记和原始字节，不把两类失败混在一起。
	start := time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)
	fake := collectorFake{start: start, anchor: start.Add(-time.Second).Format(time.RFC3339Nano) + " retained anchor", raw: collectorFixture(start.Add(time.Second), "ok", "logs", "opensearch", "", nil)}
	window, err := beginCollectorWindow(t.Context(), "phase09-test/OT01/1/primary-E1", "", t.TempDir(), start, fake.run)
	if err != nil {
		t.Fatal(err)
	}
	value := report.Summary{DataQuality: report.QualityNormal, CollectorAudit: window.finish(t.Context(), start.Add(time.Minute))}
	ctx := context.WithValue(t.Context(), generalizationGuardKey{}, true)
	if err = checkCollectorSummary(ctx, value); err != nil {
		t.Fatal(err)
	}
	if checkCollectorSummary(ctx, report.Summary{}) == nil {
		t.Fatal("无证据阶段9结果进入评分")
	}
	if checkCollectorSummary(t.Context(), report.Summary{}) != nil {
		t.Fatal("阶段8旧结果被加上新门槛")
	}
	value.DataQuality = report.QualityAffected
	if checkCollectorSummary(ctx, value) == nil {
		t.Fatal("受影响结果进入评分或补跑")
	}
	value.DataQuality = report.QualityNormal
	if err = os.WriteFile(value.CollectorAudit.RawPath, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if checkCollectorSummary(ctx, value) == nil {
		t.Fatal("改写证据后仍允许评分")
	}
	if _, err = beginCollectorWindow(t.Context(), "missing-proof", "already-running", t.TempDir(), start, fake.run); err == nil {
		t.Fatal("已有诊断被补造正常起点")
	}
	fake.mode = "late-start"
	if _, err = beginCollectorWindow(t.Context(), "late-collector", "", t.TempDir(), start, fake.run); err == nil {
		t.Fatal("窗口内才启动的Collector未在付费前拒绝")
	}
	// 已保存的unknown是不可变决定；后来诊断成功也只能恢复原摘要，不能生成正常替代品。
	value.DataQuality = report.QualityUnknown
	value.DiagnosisID = "saved-diagnosis"
	raw, _ := json.Marshal(value)
	path := filepath.Join(t.TempDir(), "slot.summary.json")
	if err = writeCollectorEvidence(path, raw); err != nil {
		t.Fatal(err)
	}
	saved, err := readCollectorSummary(path)
	if err != nil || string(saved) != string(raw) {
		t.Fatal("续跑未读回第一次质量决定", err)
	}
	if writeCollectorEvidence(path, []byte(`{}`)) == nil {
		t.Fatal("原质量摘要被覆盖")
	}
}

// TestCollectorTerminal证明外层取消后仍能有界保存终态，超时则保留最后的非终态而不编造完成。
func TestCollectorTerminal(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer stop()
	calls := 0
	value, err := terminalCollectorDetail(cleanup, func(ctx context.Context) (store.Detail, error) {
		if ctx.Err() != nil {
			return store.Detail{}, ctx.Err()
		}
		calls++
		status := "running"
		if calls > 1 {
			status = "cancelled"
		}
		return store.Detail{Diagnosis: store.Diagnosis{ID: "original", Status: status}}, nil
	})
	if err != nil || value.Status != "cancelled" || calls != 2 {
		t.Fatal("取消父上下文丢失终态", value, err, calls)
	}
	// 不响应取消的远端不能无限占用收尾，也不能冒充已经结束。
	short, stopShort := context.WithTimeout(context.WithoutCancel(parent), time.Millisecond)
	defer stopShort()
	value, err = terminalCollectorDetail(short, func(context.Context) (store.Detail, error) {
		return store.Detail{Diagnosis: store.Diagnosis{Status: "running"}}, nil
	})
	if err == nil || value.Status != "running" {
		t.Fatal("收尾超时被当成正常终态", value, err)
	}
}

// TestCollectorAttachFailure证明关联失败时先用独立预算取消，之后才读取供持久化的原任务摘要。
func TestCollectorAttachFailure(t *testing.T) {
	for _, cancelFails := range []bool{false, true} {
		parent, stop := context.WithCancel(t.Context())
		stop()
		var operations []string
		cancelProblem := errors.New("simulated cancel transport error")
		detail, err := cancelUnattachedCollector(parent, func(ctx context.Context) error {
			// 即使故障守卫已取消父上下文，HTTP取消请求仍有自己的至多三秒预算。
			deadline, ok := ctx.Deadline()
			if ctx.Err() != nil || !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 3*time.Second {
				t.Error("主动取消丢失独立预算")
			}
			operations = append(operations, "cancel")
			if cancelFails {
				return cancelProblem
			}
			return nil
		}, func(ctx context.Context) (store.Detail, error) {
			if !slices.Equal(operations, []string{"cancel"}) || ctx.Err() != nil {
				t.Error("尚未取消就读取或沿用了取消上下文")
			}
			operations = append(operations, "read-original")
			return store.Detail{Diagnosis: store.Diagnosis{ID: "original", Status: "cancelled"}, CostMicros: 123}, nil
		})
		if errors.Is(err, cancelProblem) != cancelFails || detail.ID != "original" || detail.CostMicros != 123 || !slices.Equal(operations, []string{"cancel", "read-original"}) {
			t.Fatal("取消失败丢失原错误或原收费摘要", detail, err, operations)
		}
	}
}

// TestCollectorCommitUncertainty覆盖提交确认丢失：已知ID必须取消，无ID失败和正常创建不能乱取消。
func TestCollectorCommitUncertainty(t *testing.T) {
	for _, testcase := range []struct {
		name, id string // 只有已知ID才能确定取消哪一份原任务。
		failed   bool   // 模拟Commit返回错误，但服务器可能已经完成提交。
	}{
		{"确认丢失但已知ID", "original", true},
		{"创建前失败没有ID", "", true},
		{"创建确认正常", "original", false},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			parent, stop := context.WithCancel(t.Context())
			var operations []string
			commitErr := errors.New("simulated commit acknowledgement lost")
			id, detail, err := createCollectorTask(parent, func(context.Context) (string, error) {
				operations = append(operations, "create")
				if testcase.failed {
					stop()
					return testcase.id, commitErr
				}
				return testcase.id, nil
			}, func(ctx context.Context, knownID string) error {
				// 提交阶段已取消父上下文，主动取消仍必须用独立预算且指向同一个ID。
				if ctx.Err() != nil || knownID != "original" {
					t.Error("未用独立取消或取消了其他任务")
				}
				operations = append(operations, "cancel")
				return nil
			}, func(ctx context.Context, knownID string) (store.Detail, error) {
				if ctx.Err() != nil || knownID != "original" || !slices.Equal(operations, []string{"create", "cancel"}) {
					t.Error("取消原任务之前读取了结果")
				}
				operations = append(operations, "read-original")
				return store.Detail{Diagnosis: store.Diagnosis{ID: knownID, Status: "cancelled"}, CostMicros: 123}, nil
			})
			stop()
			want := []string{"create"}
			if testcase.failed && testcase.id != "" {
				want = append(want, "cancel", "read-original")
				if detail.ID != "original" || detail.CostMicros != 123 {
					t.Fatal("提交不确定时丢失原收费摘要", detail)
				}
			}
			if id != testcase.id || errors.Is(err, commitErr) != testcase.failed || !slices.Equal(operations, want) {
				t.Fatal("创建异常分流不符", id, err, operations)
			}
		})
	}
}

// TestCollectorRetainedAnchor复现Docker先tail后until，保证旧行仍在时不会因最新行较新而误停。
func TestCollectorRetainedAnchor(t *testing.T) {
	start := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
	newestBefore := start.Add(-time.Second).Format(time.RFC3339Nano) + " retained latest before start"
	retained := newestBefore + "\n" + start.Add(-5*time.Second).Format(time.RFC3339Nano) + " retained older line\n"
	fake := collectorFake{start: start}
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "inspect" {
			return fake.run(ctx, args...)
		}
		// 新的末行晚于until，因此旧实现tail1为空；all仍能读到边界前的真实保留行。
		if slices.Contains(args, "1") {
			return []byte{}, nil
		}
		if !slices.Contains(args, "all") {
			return nil, errors.New("unexpected retained log query")
		}
		return []byte(retained), nil
	}
	legacy, err := run(t.Context(), "logs", "--timestamps", "--until", start.Format(time.RFC3339Nano), "--tail", "1", collectorContainer)
	if err != nil || len(legacy) != 0 {
		t.Fatal("未复现tail先截断的空成功结果", err)
	}
	window, err := beginCollectorWindow(t.Context(), "retained-before-start", "", t.TempDir(), start, run)
	if err != nil || window.Anchor != newestBefore {
		t.Fatal("未从全部保留行选择最近的合法锚点", window, err)
	}
	for _, bad := range []string{"", " \r\n\t", retained + start.Add(time.Nanosecond).Format(time.RFC3339Nano) + " future line\n"} {
		badRun := func(ctx context.Context, args ...string) ([]byte, error) {
			if args[0] == "inspect" {
				return fake.run(ctx, args...)
			}
			return []byte(bad), nil
		}
		if _, err = beginCollectorWindow(t.Context(), "bad-retained", "", t.TempDir(), start, badRun); err == nil {
			t.Fatal("真正空输出或未来日志被当作合法锚点")
		}
	}
	// all仍必须经过生产查看器的64MiB保护，不能用改选择方式放开无界输出。
	output := collectorOutput{Buffer: *bytes.NewBuffer(make([]byte, collectorMaximumBytes))}
	if n, err := output.Write([]byte("x")); err == nil || n != 0 || output.Len() != collectorMaximumBytes {
		t.Fatal("all查询绕过输出硬上限", n, err)
	}
}

// TestIntegrationCollectorWindow只在显式指定保留目录时查看现有Collector，不清理真实证据。
func TestIntegrationCollectorWindow(t *testing.T) {
	directory := os.Getenv("TEST_PHASE09_COLLECTOR_DIR")
	if directory == "" {
		t.Skip("只读Collector集成需显式设置TEST_PHASE09_COLLECTOR_DIR，原始日志与审计JSON会保留")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("TEST_PHASE09_COLLECTOR_DIR必须是宿主机绝对路径")
	}
	// 不用t.TempDir或Cleanup：真实原件在测试结束后仍需供人工和台账复核。
	observed := time.Now().UTC()
	start := observed.Add(-time.Minute)
	key := "phase09-readonly-preflight/" + observed.Format("20060102T150405.000000000Z")
	proof := filepath.Join(directory, "retained-"+observed.Format("20060102T150405.000000000Z"))
	latest, err := runCollectorCommand(t.Context(), "logs", "--timestamps", "--tail", "1", collectorContainer)
	if err != nil {
		t.Fatal(err)
	}
	newer := false
	for _, line := range strings.Split(strings.TrimSpace(string(latest)), "\n") {
		stamp, _, err := collectorLineTime(strings.TrimSuffix(line, "\r"))
		if err != nil {
			t.Fatal(err)
		}
		newer = newer || stamp.After(start)
	}
	if !newer {
		t.Fatal("本次没有晚于旧60秒边界的新日志，不能验证tail/until缺陷")
	}
	if err = writeCollectorEvidence(proof+".latest.log", latest); err != nil {
		t.Fatal(err)
	}
	legacy, err := runCollectorCommand(t.Context(), "logs", "--timestamps", "--until", start.Format(time.RFC3339Nano), "--tail", "1", collectorContainer)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeCollectorEvidence(proof+".legacy-tail1.log", legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 0 {
		t.Fatal("当前Docker未复现旧tail1空成功，保留原件后停止本次对照")
	}
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		raw, err := runCollectorCommand(ctx, args...)
		if slices.Contains(args, "--tail") && slices.Contains(args, "all") {
			err = errors.Join(err, writeCollectorEvidence(proof+".anchor-all.log", raw))
		}
		return raw, err
	}
	window, err := beginCollectorWindow(t.Context(), key, "", directory, start, run)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("旧边界=%s；最新原件=%s；旧tail1原件=%s；修复all原件=%s；所选锚点=%s", start.Format(time.RFC3339Nano), proof+".latest.log", proof+".legacy-tail1.log", proof+".anchor-all.log", window.Anchor)
	if err = waitTick(t.Context(), 2*time.Second); err != nil {
		t.Fatal(err)
	}
	audit := window.finish(t.Context(), time.Now().UTC())
	t.Logf("Collector原始日志保留：%s；JSON审计保留：%s；SHA256=%s", audit.RawPath, window.Prefix+".audit.json", audit.SHA256)
	if quality := report.CollectionQuality(report.Summary{DataQuality: report.QualityNormal, CollectorAudit: audit}); quality != report.QualityNormal {
		t.Fatalf("只读窗口无法验证为normal：quality=%s audit=%+v", quality, audit)
	}
	if audit.RawBytes <= 0 {
		t.Fatal("空日志不能作为正常采集窗口")
	}
	if err = verifyCollectorRaw(audit); err != nil {
		t.Fatal(err)
	}
	// 核对已经独占保存的JSON原件，而不重新生成一份替代审计。
	raw, err := os.ReadFile(window.Prefix + ".audit.json")
	var saved report.CollectorAudit
	if err != nil || json.Unmarshal(raw, &saved) != nil || saved.SHA256 != audit.SHA256 || saved.RawPath != audit.RawPath || saved.RawBytes != audit.RawBytes || !saved.WindowStart.Equal(audit.WindowStart) || !saved.WindowEnd.Equal(audit.WindowEnd) {
		t.Fatalf("保留的JSON审计与本次原件不符：%v", err)
	}
}
