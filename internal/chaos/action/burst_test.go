// 本文件：检查商品突发动作只接受自己这一批的真实完成结果。
//
// 本地 HTTP 测试服务返回几种控制状态，不连接业务，也不发送真正的商品请求。
//
// 主要内容：
//   - TestBurstResult：拒绝错批次、未完整执行或存在失败的结果。
//   - TestBurstRequest：检查发出的商品编号与数量，以及接收后确实查询完成状态。
//
// 阅读建议：completed 包含失败尝试，所以不能只核对完成数。

package action

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// TestBurstResult 避免把上一批成功，或这一批的部分成功，误写成整个注入动作已完成。
func TestBurstResult(t *testing.T) {
	for _, testcase := range []struct {
		name string // 说明这份进度为什么可接受或必须拒绝。
		body string // 只有控制状态，不伪造业务返回值。
		ok   bool   // true 才能继续下一批。
	}{
		{"全部成功", `{"id":7,"completed":4}`, true},               // 四次都完成，且没有失败或跳过。
		{"不同批次", `{"id":6,"completed":4}`, false},              // 不能拿旧成功顶替当前批次。
		{"完成数不足", `{"id":7,"completed":3}`, false},             // 停止运行不代表数量完整。
		{"存在失败", `{"id":7,"completed":4,"failed":1}`, false},   // 失败也计入 completed，须单独检查。
		{"存在未发出", `{"id":7,"completed":4,"skipped":1}`, false}, // 自相矛盾的统计也不能接受。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"data":{"burst":%s}}`, testcase.body)
			}))
			defer server.Close()
			runner := &Runner{LoadURL: server.URL, HTTP: server.Client()}
			if err := runner.waitBurst(t.Context(), 7, 4); (err == nil) != testcase.ok {
				t.Fatalf("结果判断不正确：%v", err)
			}
		})
	}
}

// TestBurstRequest 使用一次真实本机 HTTP 往返，检查控制契约和接收、完成两个步骤。
func TestBurstRequest(t *testing.T) {
	var posted, checked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/burst":
			var input struct {
				SKUID    int64 `json:"sku_id"`   // 要查询的商品，不是下单 SKU 配置。
				Requests int   `json:"requests"` // 只发送四次，不自动扩大到工作者上限。
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.SKUID != 42 || input.Requests != 4 || request.Method != "POST" {
				t.Error("突发请求参数不正确")
			}
			posted = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"data":{"id":7}}`))
		case "/control":
			checked = true
			_, _ = w.Write([]byte(`{"data":{"burst":{"id":7,"completed":4}}}`))
		default:
			t.Error("调用了意外的控制路径")
		}
	}))
	defer server.Close()
	runner := &Runner{LoadURL: server.URL, HTTP: server.Client()}
	// 通过正式分派入口执行，白名单校验不能被测试直接调用私有方法而绕开。
	if err := runner.Execute(t.Context(), scenario.Step{Action: "burst", FirstSKUID: 42, Count: 1, Requests: 4}); err != nil || !posted || !checked {
		t.Fatalf("未完整调用突发控制：%v posted=%t checked=%t", err, posted, checked)
	}
}
