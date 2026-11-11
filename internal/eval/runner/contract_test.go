// 本文件：通过 eval 的解析入口校验两组子命令的帮助，在建库或发请求之前退出。

package runner

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// captureEvalRequest 是评测器的内存传输层，默认地址和路径在请求构造完成后才做断言。
type captureEvalRequest func(*http.Request) (*http.Response, error)

// RoundTrip 把请求交给测试，不尝试建立网络连接。
func (capture captureEvalRequest) RoundTrip(request *http.Request) (*http.Response, error) {
	return capture(request)
}

// TestEvalDefaultURLs 从真实客户端取得最终请求的 URL；提前终止传输便不需要任何账户或服务。
func TestEvalDefaultURLs(t *testing.T) {
	// 记录 Eval 组装好的完整地址，让测试不依赖本机端口或网络。
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	var requests []string
	http.DefaultTransport = captureEvalRequest(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.String())
		return nil, errors.New("测试在发送前结束")
	})
	reader := config.NewReader(func(name string) (string, bool) {
		return "fixture", name == "DOCTOR_ADMIN_EMAIL" || name == "DOCTOR_ADMIN_PASSWORD"
	})
	_, loginErr := newDoctorClient(t.Context(), reader)
	_, loadErr := readLoad(t.Context(), reader)
	want := []string{"POST http://127.0.0.1:8090/api/v1/auth/login", "GET http://127.0.0.1:6070/control"}
	if loginErr == nil || loadErr == nil || !slices.Equal(requests, want) {
		t.Fatalf("默认请求：%v", requests)
	}
}

// TestEvalEnvironmentContract 为两个评测入口和登录分别留下一项非法配置，观察在连接数据库或发送登录请求之前已读到的变量。
func TestEvalEnvironmentContract(t *testing.T) {
	for _, args := range [][]string{{"export", "--run", "fixture"}, {"generalization", "export", "--run", "phase09-fixture"}} {
		for _, value := range []string{"", "0", "1", "1000000000", "1000000001", "invalid"} {
			var keys []string
			reader := config.NewReader(func(name string) (string, bool) {
				keys = append(keys, name)
				return value, value != "" && name == "EVAL_TOKEN_BUDGET"
			})
			wantKeys := "EVAL_TOKEN_BUDGET DOCTOR_MYSQL_DSN"
			problem := "环境变量 DOCTOR_MYSQL_DSN 不能为空"
			if value == "invalid" || value == "0" || value == "1000000001" {
				wantKeys = "EVAL_TOKEN_BUDGET"
				problem = "环境变量 EVAL_TOKEN_BUDGET 必须是 1 到 1000000000 之间的整数"
			}
			err := RunCLI(t.Context(), args, reader, io.Discard)
			if err == nil || err.Error() != problem || !slices.Equal(keys, strings.Fields(wantKeys)) {
				t.Fatalf("%v budget=%q：%v %v", args, value, keys, err)
			}
		}
	}
	// 提供邮箱但省略密码，登录地址已经过校验，HTTP 仍然无法发出。
	var keys []string
	reader := config.NewReader(func(name string) (string, bool) {
		keys = append(keys, name)
		return "reader@example.invalid", name == "DOCTOR_ADMIN_EMAIL"
	})
	if _, err := newDoctorClient(t.Context(), reader); err == nil || err.Error() != "环境变量 DOCTOR_ADMIN_PASSWORD 不能为空" || !slices.Equal(keys, strings.Fields("DOCTOR_URL DOCTOR_ADMIN_EMAIL DOCTOR_ADMIN_PASSWORD")) {
		t.Fatalf("登录配置：%v %v", keys, err)
	}
	keys = nil
	reader = config.NewReader(func(name string) (string, bool) {
		keys = append(keys, name)
		return "", true
	})
	if _, err := readLoad(t.Context(), reader); err == nil || !slices.Equal(keys, []string{"LOADGEN_CONTROL_URL"}) {
		t.Fatalf("压测控制地址：%v %v", keys, err)
	}
}

// TestJudgeEnvironmentContract 只走配置校验，假日格式故意不合法，因而无法到达账本或客户端构造。
func TestJudgeEnvironmentContract(t *testing.T) {
	status := runtimeStatus{Provider: "openai", Executions: map[string]run.ExecutionStatus{"comparison": {Model: "deepseek-flash", Account: "phase-07-08", Budget: run.BudgetStatus{Currency: "CNY"}}}}
	values := map[string]string{"LLM_PROVIDER": "openai", "DEEPSEEK_API_KEY": "invalid-fixture", "EVAL_DEEPSEEK_HOLIDAYS": "invalid-date"}
	var keys []string
	reader := config.NewReader(func(name string) (string, bool) {
		keys = append(keys, name)
		v, ok := values[name]
		return v, ok
	})
	// 假账户上限为零，构造器即使越过日期校验也无法获得可调用模型的账本。
	_, err := newGrader(t.Context(), reader, store.New(nil), status, true, io.Discard)
	want := strings.Fields("LLM_PROVIDER DEEPSEEK_API_KEY EVAL_COMPARISON_INPUT_PRICE EVAL_COMPARISON_CACHE_HIT_PRICE EVAL_COMPARISON_OUTPUT_PRICE EVAL_DEEPSEEK_HOLIDAYS")
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") || !slices.Equal(keys, want) {
		t.Fatalf("评委配置：%v %v", keys, err)
	}
}

// TestEvalCLIContract 所有普通命令共享一组参数，泛化命令使用另一组。每个动词都要确实走进自己的解析器。
func TestEvalCLIContract(t *testing.T) {
	reader := config.NewReader(func(name string) (string, bool) {
		t.Fatalf("帮助不该读取 %s", name)
		return "", false
	})
	if err := RunCLI(t.Context(), nil, reader, &bytes.Buffer{}); err == nil || err.Error() != "用法：eval check-clock；或 eval prepare|pilot|finish|export|freeze|matrix|run|calibrate|amend-clock|amend-baseline --run 批次 [--model 模型 --scenario 场景 --allow-paid]" {
		t.Fatalf("公开用法变化：%v", err)
	}
	if err := RunCLI(t.Context(), []string{"generalization"}, reader, &bytes.Buffer{}); err == nil || err.Error() != "用法：eval generalization freeze|run|export|finish|restore --run phase09-批次 [--scenarios OT01,OT02 --groups E1 --allow-paid]" {
		t.Fatalf("公开用法变化：%v", err)
	}
	for _, group := range []struct{ prefix, verbs, fixture string }{
		{"", "prepare pilot finish export freeze matrix run calibrate amend-clock amend-baseline", "commands.golden.txt"},
		{"generalization ", "freeze run export finish restore", "generalization_commands.golden.txt"},
	} {
		for _, verb := range strings.Fields(group.verbs) {
			command := group.prefix + verb
			var out bytes.Buffer
			err := RunCLI(t.Context(), append(strings.Fields(command), "--help"), reader, &out)
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("%s 没有进入帮助：%v", command, err)
			}
			commandHelp(t, "eval "+command, out.String(), group.fixture)
		}
	}
	// 校时分支没有 FlagSet，额外参数须在检查时钟之前被拒绝。
	if err := RunCLI(t.Context(), []string{"check-clock", "--help"}, reader, &bytes.Buffer{}); err == nil || err.Error() != "用法：eval check-clock（不接受其他参数）" {
		t.Fatal(err)
	}
}

// commandHelp 核对当前动词注册的参数清单；对齐帮助文本时去掉制表符。
func commandHelp(t *testing.T, command string, output string, fixture string) {
	t.Helper()
	header, options, ok := strings.Cut(output, "\n")
	if !ok || header != "Usage of "+command+":" {
		t.Fatalf("%q 的帮助标题错误：%s", command, header)
	}
	want, err := os.ReadFile("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	options = strings.ReplaceAll(options, "\n    \t", "\n    ")
	if options != string(want) {
		t.Fatalf("%s 的参数清单变化：\n%s", command, options)
	}
}
