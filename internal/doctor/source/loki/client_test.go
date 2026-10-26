// 本文件：验证日志查询中的文字不会被解释成新的 LogQL。
//
// 所有 HTTP 响应都由本机测试服务器提供，不需要 Loki 进程。
//
// 主要内容：
//   - TestSearchBoundary：检查转义、时间条件、响应排序和服务不匹配。
//
// 阅读建议：特别看带引号和管道符的关键词，后端收到的仍然是一个字符串。

package loki

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// TestSearchBoundary 同时检查请求和返回，避免工具把其他服务的日志混进结论。
func TestSearchBoundary(t *testing.T) {
	for _, testcase := range []struct {
		name string // 检查的边界。
		body string // Loki 夹具。
		bad  bool   // 是否应拒绝。
	}{
		{"合法日志", `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service":"orders"},"values":[["100","old"],["200","new"]]}]}}`, false}, // 合并后按时间倒序。
		{"别的服务", `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service":"other"},"values":[["100","x"]]}]}}`, true},                   // 后端也不能混入错误来源。
		{"坏行", `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service":"orders"},"values":[["100"]]}]}}`, true},                        // 不足两个元素时不能越界。
		{"非日志结果", `{"status":"success","data":{"resultType":"vector","result":[]}}`, true},                                                                        // 不把指标响应误当日志。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			keyword := `" |~ ".*`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if r.URL.Path != "/loki/api/v1/query_range" || !strings.Contains(query.Get("query"), " |= "+strconv.Quote(keyword)) || query.Get("limit") != "2" || query.Get("start") != "1000000000" {
					t.Errorf("查询条件错误：%v", query)
				}
				_, _ = w.Write([]byte(testcase.body))
			}))
			t.Cleanup(server.Close)
			client, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			rows, err := client.Search(t.Context(), source.LogQuery{Service: "orders", Keyword: keyword, Start: time.Unix(1, 0), End: time.Unix(2, 0), Limit: 2})
			if (err != nil) != testcase.bad {
				t.Fatalf("错误分类：%v", err)
			}
			if !testcase.bad && (len(rows) != 2 || rows[0].Line != "new") {
				t.Fatalf("日志结果：%+v", rows)
			}
		})
	}
}
