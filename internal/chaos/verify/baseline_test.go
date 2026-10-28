// 本文件：证明实验参数错误时立即停止，而不是等待告警碰巧消失。
//
// 控制响应由本机测试服务器提供，不依赖 Docker 或真实业务流量。
// 主要内容：TestBaseline 核对正常配置、两种不匹配和旧版本缺字段的行为。
// 阅读建议：失败用例还检查 Prometheus 没有被查询，说明判断发生在等待之前。

package verify

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBaseline 不靠等待时间断言“立即失败”，而是确认流程没有进入指标观察。
func TestBaseline(t *testing.T) {
	for _, testcase := range []struct {
		name              string
		products, workers int
		valid             bool
	}{
		{"一万商品基线", 10000, 64, true},    // 正常配置可以继续检查指标。
		{"重建回到五十万", 500000, 64, false}, // 工作集变化必须被发现。
		{"工作者不同", 10000, 128, false},   // 即使商品数正确，也不能改变并发条件。
		{"旧控制接口", 0, 0, false},         // 缺字段不能悄悄采用预期值。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			queries := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/control" {
					fmt.Fprintf(writer, `{"code":"OK","data":{"settings":{"product_count":%d,"workers":%d}}}`, testcase.products, testcase.workers)
					return
				}
				queries++
				t.Error("参数核对不应请求指标")
			}))
			defer server.Close()
			baseline := WithBaseline(New(server.URL), server.URL, 10000, 64)
			err := baseline.check(t.Context())
			if (err == nil) != testcase.valid {
				t.Fatalf("err=%v", err)
			}
			if !testcase.valid {
				_, err = baseline.Healthy(t.Context())
				if err == nil || !strings.Contains(err.Error(), "LOADGEN_PRODUCT_COUNT=10000") || !strings.Contains(err.Error(), "LOADGEN_WORKERS=64") || queries != 0 {
					t.Fatalf("未立即失败或缺少恢复办法：%v", err)
				}
			}
		})
	}
}
