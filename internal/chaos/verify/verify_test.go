// 本文件：检查监控查询失败、空数据和告警状态的解释。
//
// httptest 在本进程提供假的 Prometheus，不依赖正在运行的容器。
// 测试重点是不能把采集断开或 pending 告警误判成验收通过。
//
// 主要内容：
//   - TestQueryPresence：比较表达式成立时允许零值，空列表和 NaN 不通过。
//   - TestSymptomRequiresRelatedFiringAlert：症状必须配合真正触发的相关告警。
//   - TestHealthyRequiresContinuousSamples：坏样本会打断连续健康时间。
//
// 阅读建议：注意 PromQL 的过滤比较与带 bool 的比较返回形式不同。

package verify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// TestQueryPresence 不用网络时钟假设，逐行给出后端可能返回的内容。
func TestQueryPresence(t *testing.T) {
	for _, testcase := range []struct {
		name      string // 描述 Prometheus 返回的情形。
		body      string // 完整 JSON，真实走解码路径。
		want      bool   // 是否有满足比较条件的有限样本。
		wantError bool   // 协议错误与尚未满足条件要区分。
	}{
		{"零错误率成立", `{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"0"]}]}}`, true, false},   // 过滤比较保留零值也表示成立。
		{"没有样本", `{"status":"success","data":{"resultType":"vector","result":[]}}`, false, false},                     // 不能把无流量当成健康。
		{"无效分位数", `{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"NaN"]}]}}`, false, false}, // 没有足够数据计算延迟。
		{"后端错误", `{"status":"error"}`, false, true},                                                                   // 查询失败不能变成空结果成功。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, testcase.body) }))
			defer server.Close()
			got, err := New(server.URL).Query(t.Context(), "metric < 1")
			if got != testcase.want || (err != nil) != testcase.wantError {
				t.Fatalf("got=%t err=%v", got, err)
			}
		})
	}
}

// TestSymptomRequiresRelatedFiringAlert 先返回 pending，再返回不相关告警，最后才允许成功。
func TestSymptomRequiresRelatedFiringAlert(t *testing.T) {
	// 三次告警响应分别验证“还没触发”“不相关”和“真正满足”三条分支。
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/query" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"2"]}]}}`)
			return
		}
		calls++
		state, name := "firing", "Expected"
		if calls == 1 {
			state = "pending"
		}
		if calls == 2 {
			name = "Unrelated"
		}
		fmt.Fprintf(w, `{"status":"success","data":{"alerts":[{"state":%q,"labels":{"alertname":%q}}]}}`, state, name)
	}))
	defer server.Close()
	client := New(server.URL)
	client.poll = time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := client.Symptom(ctx, scenario.Symptom{PromQL: "metric > 1", Alerts: []string{"Expected"}})
	if err != nil || calls != 3 || !result.SymptomSeen || len(result.Alerts) != 1 {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}

// TestHealthyRequiresContinuousSamples 在第一次健康后制造一个坏样本，确认不能立即通过。
func TestHealthyRequiresContinuousSamples(t *testing.T) {
	// 第一轮所有查询正常，第二轮第一项为空，之后才允许重新连续计时。
	queries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/alerts" {
			fmt.Fprint(w, `{"status":"success","data":{"alerts":[]}}`)
			return
		}
		queries++
		values := `[{"value":[1,"1"]}]`
		if queries == len(healthyQueries())+1 {
			values = `[]`
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, values)
	}))
	defer server.Close()
	client := New(server.URL)
	client.poll = 2 * time.Millisecond
	client.stable = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := client.Healthy(ctx)
	if err != nil || result.LastProblem == "" || result.Elapsed < client.stable {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
