// 本文件：从审批服务入口固定三种写工具回复给模型的结果，使用内存审批单、假元数据和假运维响应。

package approval

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestApprovalToolResultsGolden 同一份待审申请可以被拒绝、过期或批准，等待期间没有工具结果。
func TestApprovalToolResultsGolden(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &Backend{
			ddl: &sql.DB{}, metadata: &fakeMetadata{exists: true}, tables: map[string]bool{"stock": true},
			ops:  OpsConfig{Services: map[string]string{"orders": "http://ops.invalid"}, PoolMin: 5, PoolMax: 200, WarmService: "orders"},
			http: &http.Client{Transport: goldenOpsTransport{}}, timeout: time.Minute,
		}
		fields := map[string]string{
			"create_index":     `"table":"stock","columns":["sku_id"],"unique":true`,
			"set_db_pool_size": `"service":"orders","size":30`,
			"warm_cache":       `"ids":[1,2]`,
		}
		var got bytes.Buffer
		for _, definition := range backend.definitions() {
			name := definition.Function.Name
			arguments := "{" + fields[name] + `,"reason":"观察到延迟","expected_effect":"降低延迟","risk":"额外开销","evidence_steps":[7]}`
			call := llm.ToolCall{ID: name, Type: "function", Function: llm.Function{Name: name, Arguments: arguments}}
			// 只记录返回模型的结果；待审时 Agent 会暂停，不能伪造一条成功响应。
			recordApprovalDecisions(t, &got, backend, call)
		}
		want, err := os.ReadFile("testdata/approval_tool_results.golden.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("审批工具结果与基线不同，实际为\n%s", got.Bytes())
		}
	})
}

// recordApprovalDecisions 比较每种审批决定。三个分支都从同一份最初保存的待审单开始。
func recordApprovalDecisions(t *testing.T, out *bytes.Buffer, backend *Backend, call llm.ToolCall) {
	t.Helper()
	repo := memoryApprovals{}
	service := New("shop", backend, repo, llm.NewRedactor())
	read := func(steps map[int]bool, status string) {
		t.Helper()
		waiting, result, err := service.Handle(t.Context(), "diagnosis", 8, call, steps)
		if err != nil {
			t.Fatal(err)
		}
		if status == "pending" {
			if !waiting || result.Tool != "" {
				t.Fatalf("%s 待审时提前给出了结果", call.ID)
			}
			return
		}
		if waiting || result.Status != status {
			t.Fatalf("%s：期望 %s，等待=%t，实际结果=%+v", call.ID, status, waiting, result)
		}
		if err := json.NewEncoder(out).Encode(result); err != nil {
			t.Fatal(err)
		}
	}
	// 先提交一份没有有效证据的申请，再补全证据，只有后者会创建待审单。
	read(nil, "invalid_argument")
	if len(repo) != 0 {
		t.Fatal("无效申请被保存")
	}
	steps := map[int]bool{7: true}
	read(steps, "pending")
	if len(repo) != 1 {
		t.Fatal("没有保存唯一待审单")
	}
	for id, pending := range repo {
		for _, decision := range []struct{ status, result string }{{"rejected", "rejected"}, {"expired", "expired"}, {"approved", "ok"}} {
			branch := pending
			branch.Status = decision.status
			repo[id] = branch
			read(steps, decision.result)
		}
	}
}

// goldenOpsTransport 在内存中应答，只接受样例预期用到的三种请求。
type goldenOpsTransport struct{}

// RoundTrip 把申请前读到的池上限与批准后的响应分开，任何意外路径都直接失败。
func (goldenOpsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string
	switch {
	case req.Method == "GET" && req.URL.Path == "/ops/pool":
		body = `{"instance":"fixture","initial":20,"current":20,"revision":0}`
	case req.Method == "POST" && req.URL.Path == "/ops/pool":
		body = `{"instance":"fixture","initial":20,"current":30,"revision":1,"last_operation":"fixture-operation"}`
	case req.Method == "POST" && req.URL.Path == "/ops/cache/warm":
		body = `{"loaded":1,"already_present":1}`
	default:
		return nil, fmt.Errorf("样例没有 %s %s", req.Method, req.URL.Path)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}
