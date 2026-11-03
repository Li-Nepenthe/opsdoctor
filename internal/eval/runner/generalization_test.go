// 本文件：用本机假接口检查阶段 9 的注入、账户、异常暂停与恢复依据。
//
// 测试只使用 httptest 和临时文件，既不启动 Docker，也不读取真实模型 Key。
// 重点是“重试不能换实验、改targeting不能扩大故障、恢复不能丢掉原文件”。
//
// 主要内容：TestDemoFlagMutation 验证变更范围；TestDemoHTTP 检查真实协议形状；
// TestGeneralizationBoundaries 检查账户和隔离；TestDemoRestoreProof 验证快照不变。
// 阅读建议：先看标注了上游 targeting 结构的测试输入。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestDemoFlagMutation 特定商品的故障只改变 if 命中分支，条件与其他开关完整保留。
func TestDemoFlagMutation(t *testing.T) {
	original := json.RawMessage(`{"flags":{"adFailure":{"defaultVariant":"off","variants":{"on":true,"off":false}},"productCatalogFailure":{"defaultVariant":"off","variants":{"on":true,"off":false},"targeting":{"if":[{"==":[{"var":"productId"},"OLJCESPC7Z"]},"off","off"]}}}}`)
	want := json.RawMessage(`{"flags":{"adFailure":{"defaultVariant":"off","variants":{"on":true,"off":false}},"productCatalogFailure":{"defaultVariant":"off","variants":{"on":true,"off":false},"targeting":{"if":[{"==":[{"var":"productId"},"OLJCESPC7Z"]},"on","off"]}}}}`)
	value := demoScenario{Flag: "productCatalogFailure", Variant: "on", OverrideTargeting: true}
	changed, err := changeDemoFlag(original, value)
	if err != nil || !equalJSON(changed, want) {
		t.Fatalf("扩大了 targeting 范围：%s %v", changed, err)
	}
	if strings.Contains(string(original), `"on","off"]`) {
		t.Fatal("修改了原始恢复快照")
	}
	// 普通开关只修改 defaultVariant；未知变体和不存在的 targeting 都需要停下。
	changed, err = changeDemoFlag(original, demoScenario{Flag: "adFailure", Variant: "on"})
	if err != nil || equalJSON(changed, original) {
		t.Fatal("没有修改普通开关", err)
	}
	for _, invalid := range []demoScenario{
		{Flag: "missing", Variant: "on"},
		{Flag: "adFailure", Variant: "unknown"},
		{Flag: "adFailure", Variant: "on", OverrideTargeting: true},
	} {
		if _, err = changeDemoFlag(original, invalid); err == nil {
			t.Fatal("无效注入被接受", invalid)
		}
	}
}

// TestDemoHTTP 对照官方 GET /api/read 与 POST {data:{flags:...}}，写后必须读回一致。
func TestDemoHTTP(t *testing.T) {
	current := json.RawMessage(`{"flags":{"adFailure":{"defaultVariant":"off","variants":{"on":true,"off":false}}}}`)
	var mutex sync.Mutex // httptest 处理器在另一个 goroutine 读写快照。
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Demo 请求携带了医生令牌")
		}
		mutex.Lock()
		defer mutex.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/read":
			_, _ = w.Write(current)
		case "POST /api/write":
			var payload struct {
				Data json.RawMessage `json:"data"`
			}
			if json.NewDecoder(r.Body).Decode(&payload) != nil || !json.Valid(payload.Data) {
				t.Error("写入没有 data 包装")
			}
			current = slices.Clone(payload.Data)
			writes++
			fmt.Fprint(w, `{}`)
		default:
			t.Error("访问了未约定路径", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original, err := readDemoFlags(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := changeDemoFlag(original, demoScenario{Flag: "adFailure", Variant: "on"})
	if err != nil || writeDemoFlags(t.Context(), server.URL, changed) != nil {
		t.Fatal("写后读回失败", err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if writes != 1 || !equalJSON(current, changed) || equalJSON(current, original) {
		t.Fatal("没有使用完整配置写入")
	}
}

// TestDemoQuery 不把无流量、错误响应或无效数值当成已验证的健康状态。
func TestDemoQuery(t *testing.T) {
	// 含样本只证明比较条件成立，空结果与无效数值都不能偷换成成功。
	for _, test := range []struct {
		body          string
		passed, valid bool
	}{
		{`{"status":"success","data":{"result":[{"value":[1,"2"]}]}}`, true, true},
		{`{"status":"success","data":{"result":[]}}`, false, true},
		{`{"status":"success","data":{"result":[{"value":[1,"NaN"]}]}}`, false, true},
		{`{"status":"error"}`, false, false},
		{`{"status":"success","data":{"result":[{"value":[]}]}}`, false, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/query" || r.URL.Query().Get("query") != "x > 0" {
				t.Error("表达式未作为查询值编码")
			}
			fmt.Fprint(w, test.body)
		}))
		passed, err := demoQuery(t.Context(), server.URL, "x > 0")
		server.Close()
		if passed != test.passed || (err == nil) != test.valid {
			t.Fatalf("passed=%t err=%v body=%s", passed, err, test.body)
		}
	}
}

// TestGeneralizationBoundaries 旧账户、两套靶场同时运行和中途换模型都必须被拦住。
func TestGeneralizationBoundaries(t *testing.T) {
	// 固定两个预算账户，再逐项破坏边界，确认失败不会回退到旧账户。
	status := runtimeStatus{Provider: "openai", Executions: map[string]run.ExecutionStatus{
		"primary":    {Model: "gpt-6.1-terra", Provider: "openai", Account: "phase-09-openai", Budget: run.BudgetStatus{Currency: "USD", LimitMicros: 10000000}},
		"comparison": {Model: "deepseek-flash", Provider: "openai", Account: "phase-09-deepseek", Budget: run.BudgetStatus{Currency: "CNY", LimitMicros: 10000000}},
	}}
	values, err := generalizationConfigurations(status, "gpt-6.1-terra", []string{"E1", "E2"})
	if err != nil || len(values) != 2 || values[0].Profile != "primary" {
		t.Fatal(values, err)
	}
	for _, account := range []string{"phase-07-08-openai", "arbitrary"} {
		primary := status.Executions["primary"]
		primary.Account = account
		status.Executions["primary"] = primary
		if _, err = generalizationConfigurations(status, "gpt-6.1-terra", []string{"E1"}); err == nil {
			t.Fatal("接受旧账户或任意新账户")
		}
	}
	allowed := "opsdoctor/doctor-mysql\nopsdoctor-phase09-doctor/doctor\nopsdoctor-otel-demo/ad\nqqbot/astrbot"
	if err = checkDemoIsolation(allowed); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"opsdoctor/order-service", "opsdoctor/loadgen", "opsdoctor/grafana", "opsdoctor/doctor", "opsdoctor-cold-repro-20261102/shop-mysql"} {
		if checkDemoIsolation(allowed+"\n"+extra) == nil {
			t.Fatal("两套系统同时运行仍被接受", extra)
		}
	}
	ctx := context.WithValue(t.Context(), generalizationGuardKey{}, true)
	for _, summary := range []report.Summary{{Status: "failed", ReliabilityFailure: true}, {Status: "budget_exceeded"}, {Status: "succeeded", MixedModel: true}} {
		if checkGeneralizationSummary(ctx, summary, "slot") == nil {
			t.Fatal("阶段 9 异常没有拦截下一次收费")
		}
		if checkGeneralizationSummary(t.Context(), summary, "stage8") != nil {
			t.Fatal("改变了阶段 8 的旧补跑行为")
		}
	}
}

// TestDemoRestoreProof 确认恢复只追加证明，不截断或覆盖唯一的原始恢复快照。
func TestDemoRestoreProof(t *testing.T) {
	// 原快照在第一次与重复确认之后都要保持逐字节相同。
	path := filepath.Join(t.TempDir(), "recovery.json")
	original := json.RawMessage(`{"flags":{"example":"original"}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkDemoUnrestored(path); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := writeDemoRestoreProof(path, original); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(original) {
		t.Fatal("恢复确认覆盖了原快照", err)
	}
	if checkDemoUnrestored(path) == nil {
		t.Fatal("恢复后重新打开开关仍可冒充同一次注入")
	}
	if err = writeDemoRestoreProof(path, json.RawMessage(`{"different":true}`)); err == nil {
		t.Fatal("已有恢复证明被另一份原始配置覆盖")
	}
}

// TestGeneralizationCLI 在连接数据库前拒绝别的系统、旧批次和哈希覆盖范围外的场景目录。
func TestGeneralizationCLI(t *testing.T) {
	// 环境读取器故意失败，证明这些错误会在接触凭据之前出现。
	reader := config.NewReader(func(string) (string, bool) { t.Fatal("无效命令不应读取凭据"); return "", false })
	for _, args := range [][]string{
		{"generalization"},
		{"generalization", "freeze", "--run", "phase08-old"},
		{"generalization", "freeze", "--run", "phase09-test", "--target", "shop"},
		{"generalization", "freeze", "--run", "phase09-test", "--groups", "E0"},
		{"generalization", "freeze", "--run", "phase09-test", "--scenario-dir", "../../outside"},
		{"generalization", "freeze", "--run", "phase09-test", "--scenarios", "OT01,OT01"},
		{"generalization", "freeze", "--run", "phase09-test", "--flagd-url", "http://example.com"},
	} {
		if err := RunCLI(t.Context(), args, reader, io.Discard); err == nil {
			t.Fatal("无效命令未拒绝", args)
		}
	}
	old := clockSampleArguments(t.Context())
	stage9 := clockSampleArguments(context.WithValue(t.Context(), demoClockKey{}, true))
	if old[0] != "compose" || !slices.Equal(stage9, []string{"exec", "opsdoctor-phase09-doctor", "/app/server", "--clock-samples"}) {
		t.Fatal("混用了两次阶段的时钟采样容器")
	}
}

// TestGeneralizationScenarioFiles 让仓库中的五份正式候选文件也通过严格字段校验。
func TestGeneralizationScenarioFiles(t *testing.T) {
	directory := filepath.Join("..", "..", "..", "eval", "scenarios", "otel-demo")
	values, err := loadGeneralizationCases(directory, []string{"OT01", "OT02", "OT03", "OT04", "OT05"})
	if err != nil || len(values) != 5 {
		t.Fatal("候选场景不能被当前评测器读取", err)
	}
	if !values["OT05"].OverrideTargeting || values["OT05"].Flag != "productCatalogFailure" {
		t.Fatal("特定商品场景缺少精确 targeting 处理")
	}
	// 不完整映射和未实现编号不能靠默认值变成一份可运行实验。
	if _, err = loadGeneralizationCases(directory, []string{"OT09"}); err == nil {
		t.Fatal("接受未授权场景")
	}
	if _, err = newGraderForAccount(t.Context(), config.NewReader(func(string) (string, bool) { t.Fatal("Mock 读取付费配置"); return "", false }), store.New(nil), runtimeStatus{Provider: "mock"}, true, io.Discard, "phase-09-deepseek"); err != nil {
		t.Fatal("阶段 9 Mock 评委不应联网", err)
	}
}

// TestGeneralizationProfileHash 证明第二系统的说明书进入行为哈希，普通导航文字仍不影响它。
func TestGeneralizationProfileHash(t *testing.T) {
	// 在临时仓库模拟说明书变化，不触碰用户工作区或任何学习材料。
	t.Chdir(t.TempDir())
	for _, root := range []string{"cmd", "internal", "tools", "gen", "api", "deploy", "eval"} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{"go.mod": "module test", "go.sum": "", "deploy/otel-demo.profile.md": "original topology", "deploy/README.md": "navigation"} {
		if err := os.WriteFile(filepath.FromSlash(path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := sourceHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.FromSlash("deploy/README.md"), []byte("new navigation"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := sourceHash()
	if err != nil || after != before {
		t.Fatal("普通导航不应改变行为哈希", err)
	}
	if err = os.WriteFile(filepath.FromSlash("deploy/otel-demo.profile.md"), []byte("changed topology"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err = sourceHash()
	if err != nil || after == before {
		t.Fatal("第二系统说明书变化没有被哈希检测", err)
	}
}
