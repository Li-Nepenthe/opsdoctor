// 本文件：用固定 HTTP 响应检查 Prometheus 适配器的契约。
//
// 时间和数值都来自测试夹具，不把它们记成真实运行数据。
//
// 主要内容：
//   - TestQueries：即时、范围、标量、坏响应和告警状态。
//
// 阅读建议：看请求断言和返回值断言两边，不能只验证 JSON 能解析。

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// TestQueries 确认工具给出的范围真的被发送到后端，pending 也不会混入 firing 列表。
func TestQueries(t *testing.T) {
	for _, testcase := range []struct {
		name     string // 说明本次检查哪一种后端格式。
		response string // 夹具正文。
		kind     string // instant、range 或 alerts。
		bad      bool   // 是否应该被当成坏响应拒绝。
	}{
		{"即时", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"service":"a"},"value":[100,"3"]}]}}`, "instant", false},                                                // vector 只有一个值。
		{"范围", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"service":"a"},"values":[[100,"3"],[110,"5"]]}]}}`, "range", false},                                     // matrix 是多个采样点。
		{"标量", `{"status":"success","data":{"resultType":"scalar","result":[100,"3"]}}`, "instant", false},                                                                                     // scalar 没有标签也不能丢。
		{"短采样点", `{"status":"success","data":{"resultType":"vector","result":[{"value":[100]}]}}`, "instant", true},                                                                            // 防止数组下标越界。
		{"业务错误", `{"status":"error"}`, "instant", true},                                                                                                                                        // HTTP 200 仍可能是查询错误。
		{"告警", `{"status":"success","data":{"alerts":[{"state":"pending"},{"state":"firing","labels":{"alertname":"Slow"},"activeAt":"2026-10-01T00:00:00Z","value":"2"}]}}`, "alerts", false}, // 只留下 firing。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("不是只读 GET")
				}
				if testcase.kind == "range" && (r.URL.Path != "/api/v1/query_range" || r.URL.Query().Get("step") != "10" || r.URL.Query().Get("start") == "") {
					t.Error("范围条件丢失")
				}
				_, _ = w.Write([]byte(testcase.response))
			}))
			t.Cleanup(server.Close)
			client, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			if testcase.kind == "alerts" {
				alerts, err := client.Alerts(t.Context())
				if err != nil || len(alerts) != 1 || alerts[0].Labels["alertname"] != "Slow" {
					t.Fatalf("告警错误：%v %+v", err, alerts)
				}
				return
			}
			query := source.MetricQuery{Expression: "up"}
			if testcase.kind == "range" {
				query.Start = time.Unix(100, 0)
				query.End = time.Unix(110, 0)
				query.Step = 10 * time.Second
			}
			result, err := client.Query(t.Context(), query)
			if (err != nil) != testcase.bad {
				t.Fatalf("错误分类：%v", err)
			}
			if !testcase.bad && (len(result.Series) != 1 || result.Series[0].Points[0].Value != "3") {
				t.Fatalf("指标转换错误：%+v", result)
			}
		})
	}
}
