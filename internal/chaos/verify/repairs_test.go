// 本文件：验证实验基线能发现医生留下的修改，而不会替人自动撤销。
//
// 索引读取用测试函数替换，Prometheus 用本机假 HTTP 服务替换。测试不连业务库，
// 不发任何 DDL，也不改变当前运行的实验。
//
// 主要内容：TestRepairBaseline 覆盖索引残留、池已修改和指标缺失。
// 阅读建议：健康要求是“能证明原样”，缺证据不能当作通过。

package verify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRepairBaseline 每个用例分别控制索引与池证据，不靠告警恰好是否正在 firing。
func TestRepairBaseline(t *testing.T) {
	// 分别模拟索引残留、池修改、缺采集，以及完整干净的基线。
	for _, test := range []struct {
		name       string
		indexes    []string
		sample, ok bool
	}{
		{"医生索引尚未撤销", []string{"stock.idx_doctor_test"}, true, false},
		{"池已变更或指标缺失", nil, false, false},
		{"索引与池均干净", nil, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Query().Get("query"), "db_pool_runtime_changed") {
					t.Error("没查池修改证据")
				}
				rows := "[]"
				if test.sample {
					rows = `[{"value":[1,"3"]}]`
				}
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, rows)
			}))
			defer server.Close()
			baseline := WithBaseline(New(server.URL), "", 10000, 64)
			baseline.indexes = func(context.Context) ([]string, error) { return test.indexes, nil }
			if err := baseline.checkRepairs(t.Context()); (err == nil) != test.ok {
				t.Fatalf("基线检查=%v", err)
			}
		})
	}
}
