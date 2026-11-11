// 本文件：通过医生的真实路由记录页面与 doctorctl 收到的状态码和响应正文。

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// responseStore 让列表、详情、事件和审批指向同一份已结束的诊断记录。
type responseStore struct {
	*replayStore
	fakeApprovals
	diagnosis store.Diagnosis
}

// List 返回一条非空记录，协议增加、删除或改名的字段才会反映到样本中。
func (s *responseStore) List(context.Context, string, string, int, int) ([]store.Diagnosis, error) {
	return []store.Diagnosis{s.diagnosis}, nil
}

// Detail 给页面和事件流提供同一份已经提交的步骤。
func (s *responseStore) Detail(context.Context, string) (store.Detail, error) {
	return store.Detail{Diagnosis: s.diagnosis, Steps: s.steps, LastSeq: len(s.steps)}, nil
}

// Approvals 返回一张包含实际参数的演示审批单，供页面呈现。
func (s *responseStore) Approvals(context.Context, string) ([]approval.Record, error) {
	return []approval.Record{{
		ID: strings.Repeat("b", 32), DiagnosisID: s.diagnosis.ID, Target: s.diagnosis.Target,
		Status: "pending", Tool: "test_tool", Args: json.RawMessage(`{"size":8}`), Evidence: []int{1},
		ExpiresAt: s.diagnosis.CreatedAt.Add(time.Hour),
	}}, nil
}

// responseRunner 只提供页面需要的预算快照，批准之后不会启动后台任务。
type responseRunner struct{}

// Status 让页面的预算字段有可核对的币种和金额。
func (responseRunner) Status(context.Context) (run.BudgetStatus, error) {
	return run.BudgetStatus{Currency: "USD", SpentMicros: 1200, LimitMicros: 10000}, nil
}

// Wake 在内存演示中没有等待领取任务的调度器。
func (responseRunner) Wake() {}

// Cancel 不是这里的测试场景；意外调用时应让测试失败。
func (responseRunner) Cancel(context.Context, string) error { panic("意外调用 Cancel") }

// setupResponseAPI 给真实路由接上固定预算和已结束的诊断，事件流可以发完最后一帧自然退出。
func setupResponseAPI(t *testing.T, id string) http.Handler {
	t.Helper()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	router, _, _ := setupWithOptions(t, func(options *Options) {
		// 同一份步骤流同时出现在详情 JSON 和 SSE 帧里。
		steps := []agent.Step{
			{Seq: 1, Kind: "tool", ToolName: "service_topology", Status: "ok", Args: json.RawMessage(`{}`), Result: "alpha", CreatedAt: created},
			{Seq: 2, Kind: "system", Status: "succeeded", CreatedAt: created.Add(time.Second)},
		}
		fixture := &responseStore{
			replayStore: &replayStore{memoryStore: options.Store.(*memoryStore), steps: steps},
			diagnosis:   store.Diagnosis{ID: id, Target: "alpha", Trigger: "manual", Status: "succeeded", Title: "示例", CreatedAt: created, Currency: "USD", Provider: "mock", Model: "script"},
		}
		options.Store, options.Approvals, options.Evaluations = fixture, fixture, evalView{}
		options.Provider, options.Model, options.Runner, options.Events = "mock", "script", responseRunner{}, events.New()
	})
	return router
}

// TestDoctorHTTPResponsesGolden 保留 JSON 封装和 SSE 帧的原始字节。
func TestDoctorHTTPResponsesGolden(t *testing.T) {
	synctest.Test(t, checkDoctorHTTPResponses)
}

// checkDoctorHTTPResponses 在虚拟时钟内登录，签发时间可复现，后续请求继续使用刚签出的身份。
func checkDoctorHTTPResponses(t *testing.T) {
	id := strings.Repeat("a", 32)
	router := setupResponseAPI(t, id)
	var transcript strings.Builder
	var token string
	diagnosis := "/api/v1/diagnoses/" + id
	decision := "/api/v1/approvals/" + strings.Repeat("b", 32)
	for _, call := range []struct{ method, path, body string }{
		{"GET", "/api/v1/status", ""},
		{"POST", "/api/v1/auth/login", `{"email":"learner@example.invalid","password":"wrong-demo-password"}`},
		{"POST", "/api/v1/auth/login", `{"email":"learner@example.invalid","password":"demo-password-only"}`},
		{"GET", "/api/v1/status", ""},
		{"GET", "/api/v1/diagnoses?target=alpha&page=1&page_size=2", ""},
		{"GET", diagnosis, ""},
		{"GET", diagnosis + "/events", ""},
		{"GET", diagnosis + "/approvals", ""},
		{"POST", decision + "/approve", "{}"},
		{"POST", decision + "/reject", "{}"},
		{"GET", "/api/v1/eval/runs", ""},
	} {
		// 终态帧若意外丢失，事件流也应在测试期限内返回，方便看出哪条响应变了。
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		req := httptest.NewRequestWithContext(ctx, call.method, call.path, strings.NewReader(call.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-request")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		cancel()
		body := res.Body.String()
		// Cookie 保存本次签出的 JWT；同一个值必须能从登录正文拿到并用于后续请求。
		if call.path == "/api/v1/auth/login" && res.Code == 200 {
			// 整个未替换的响应也要固定，但不能把可用格式的令牌写进公开仓库。
			wantHash, err := os.ReadFile("testdata/login_body.golden.sha256")
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x\n", sha256.Sum256(res.Body.Bytes())); got != string(wantHash) {
				t.Fatalf("登录响应的原始字节已变化，SHA-256 为 %s", got)
			}
			cookies := res.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Value == "" {
				t.Fatal("缺少登录 Cookie")
			}
			token = cookies[0].Value
			body = strings.NewReplacer(token, "{issued-jwt}", csrfToken(token), "{session-csrf}").Replace(body)
		}
		fmt.Fprintf(&transcript, "%s %s\n%d %s\n%s\n", call.method, call.path, res.Code, res.Header().Get("Content-Type"), body)
	}

	golden := "testdata/http_responses.golden.txt"
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if transcript.String() != string(want) {
		t.Fatalf("%s 与实际 HTTP 响应不一致：\n%s", golden, transcript.String())
	}
}
