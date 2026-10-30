// 本文件：验证试跑的基线检查和内存暂停线不会被无效读数绕过。
//
// 主要内容：TestBaseline 用本机假服务模拟控制接口；TestMemorySize 检查单位和异常数。
// TestCLIValidation 证明错误命令在连接数据库之前就失败。
// 阅读建议：测试没有 Docker 和模型依赖，也不会发起外部网络请求。

package runner

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestBaseline 区分配置文件里的期望值与控制接口返回的实际值。
func TestBaseline(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		// 正确基线允许继续；五十万商品与少工作者都改变了实验条件。
		{"符合基线", `{"data":{"settings":{"qps":200,"hot_sku_id":0,"product_count":10000,"workers":64}}}`, true},
		{"商品数回到默认", `{"data":{"settings":{"qps":200,"hot_sku_id":0,"product_count":500000,"workers":64}}}`, false},
		{"工作者变化", `{"data":{"settings":{"qps":200,"hot_sku_id":0,"product_count":10000,"workers":32}}}`, false},
		// 缺字段不应按默认值补齐，否则无法发现旧版本控制接口。
		{"缺字段", `{"data":{"settings":{"qps":200}}}`, false},
		{"缺热点不能当零", `{"data":{"settings":{"qps":200,"product_count":10000,"workers":64}}}`, false}, // 新约定要求接口明确给出热点。
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/control" || r.Header.Get("Authorization") != "" {
					t.Error("控制请求路径或凭据错误")
				}
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			reader := config.NewReader(func(name string) (string, bool) { return server.URL, name == "LOADGEN_CONTROL_URL" })
			if err := checkBaseline(t.Context(), reader); (err == nil) != test.valid {
				t.Fatalf("有效=%t err=%v", test.valid, err)
			}
		})
	}
}

// TestMemorySize 拒绝 NaN、无穷大和溢出；这些值转成整数可能错误地通过六 GiB 检查。
func TestMemorySize(t *testing.T) {
	for _, test := range []struct {
		text string
		want int64
	}{
		// 二进制与十进制单位不同；小数也是 docker stats 的正常输出。
		{"1.5GiB", 1610612736}, {"12MiB", 12582912}, {"10kB", 10000}, {"0B", 0},
	} {
		if got, err := memorySize(test.text); err != nil || got != test.want {
			t.Fatalf("%s: %d %v", test.text, got, err)
		}
	}
	for _, value := range []string{"NaNGiB", "+InfMiB", "-1B", "1e99GiB", "16unknown", ""} {
		if _, err := memorySize(value); err == nil {
			t.Errorf("接受无效内存 %q", value)
		}
	}
}

// TestCLIValidation 不提供任何数据库配置，仍应先得到具体的命令校验错误。
func TestCLIValidation(t *testing.T) {
	reader := config.NewReader(func(string) (string, bool) { t.Fatal("非法命令不应读环境"); return "", false })
	for _, test := range []struct {
		args []string
		want string
	}{
		// 缺命令、重复场景和白名单以外场景分别检查三个入口分支。
		{nil, "用法"}, {[]string{"prepare", "--run", "test", "--scenarios", "S01,S01"}, "不重复"},
		{[]string{"check-clock", "--run", "test"}, "不接受"}, // 独立校时不接受模型或批次选项。
		{[]string{"prepare", "--run", "test", "--scenarios", "S01,S14"}, "已实现"},
	} {
		if err := RunCLI(t.Context(), test.args, reader, io.Discard); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("args=%v err=%v", test.args, err)
		}
	}
}
