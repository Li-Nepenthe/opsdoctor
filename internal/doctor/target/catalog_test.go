// 本文件：证明两个系统的工具和告警令牌不会串用。
//
// 两个 HTTP 后端分别返回 11 和 22。除了检查结果，还统计后端收到的请求数，
// 防止某个工具返回了正确外形的数据，却实际访问了错误系统。
//
// 主要内容：
//   - TestIsolation：用虚构系统与 shop 验证固定绑定、令牌隔离和缺数据源降级。
//   - TestConfigFailures：检查重名、共用凭据、路径逃逸和未知配置。
//   - writeConfig、testLookup：准备不含真实凭据的配置与环境。
//
// 阅读建议：从两个后端的计数断言看隔离，再读启动失败的表格用例。

package target

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestIsolation 使用真正的 HTTP 适配器与工具注册表，不用假 Registry 代替接线。
func TestIsolation(t *testing.T) {
	directory := t.TempDir()
	var shopCalls, warehouseCalls atomic.Int64
	shop := metricsServer(t, 11, &shopCalls)
	warehouse := metricsServer(t, 22, &warehouseCalls)
	writeConfig(t, directory, "shop", "prometheus: "+shop.URL+"\n")
	fixture, err := os.ReadFile("testdata/warehouse.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, "warehouse.yaml", strings.ReplaceAll(string(fixture), "http://127.0.0.1:1", warehouse.URL))
	profile, err := os.ReadFile("testdata/warehouse.profile.md")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, "warehouse.profile.md", string(profile))
	catalog, err := Load(directory, testLookup)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	if strings.Join(catalog.Names(), ",") != "shop,warehouse" {
		t.Fatal(catalog.Names())
	}
	a, _ := catalog.Get("shop")
	b, _ := catalog.Get("warehouse")
	if !a.Authenticate(strings.Repeat("s", 32)) || b.Authenticate(strings.Repeat("s", 32)) {
		t.Fatal("令牌串用")
	}
	if _, ok := catalog.Get("missing"); ok {
		t.Fatal("未知系统被默认接到 shop")
	}
	for _, testcase := range []struct {
		system *System
		number string
	}{
		{a, "11"}, // shop 工具只能得到 shop 后端的值。
		{b, "22"}, // 第二个系统必须有自己的客户端。
	} {
		result := testcase.system.Tools().Run(t.Context(), "query_metrics", json.RawMessage(`{"query":"up","lookback_seconds":0}`))
		if result.Status != "ok" || !strings.Contains(string(result.Data), testcase.number) {
			t.Fatalf("结果=%+v", result)
		}
		if len(testcase.system.Tools().List()) != 5 {
			t.Fatal("缺少数据源时仍暴露了无法运行的工具")
		}
	}
	if shopCalls.Load() != 1 || warehouseCalls.Load() != 1 {
		t.Fatal("工具访问了另一个系统")
	}
}

// TestConfigFailures 在创建连接之前报错，而且错误不会带出密码或完整连接串。
func TestConfigFailures(t *testing.T) {
	for _, testcase := range []struct{ name, extra, second, want string }{
		{"重名", "", "name: shop\nprofile: warehouse.profile.md\nalert_token_env: WAREHOUSE_ALERT_TOKEN\n", "重复"},                                                                                                  // 名称决定任务归属，不能含糊。
		{"共用变量", "mysql_readonly_dsn_env: SHOP_MYSQL_RO_DSN\n", "name: warehouse\nprofile: warehouse.profile.md\nalert_token_env: WAREHOUSE_ALERT_TOKEN\nmysql_readonly_dsn_env: SHOP_MYSQL_RO_DSN\n", "共用凭据变量"}, // 换文件不代表换账号。
		{"错误前缀", "mysql_readonly_dsn_env: OTHER_DSN\n", "", "SHOP_"},                                                                                                                                             // 一眼能看出账号属于谁。
		{"未知地址", "arbitrary_url: http://example.invalid\n", "", "未知字段"},                                                                                                                                          // 不能偷偷增加通用网络入口。
		{"路径逃逸", "", "name: warehouse\nprofile: ../warehouse.profile.md\nalert_token_env: WAREHOUSE_ALERT_TOKEN\n", "TARGETS_DIR"},                                                                               // 说明书不能越过挂载目录。
		{"重复字段", "prometheus: http://a\nprometheus: http://b\n", "", "格式错误"},                                                                                                                                     // 不允许悄悄采用最后一项。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			directory := t.TempDir()
			writeConfig(t, directory, "shop", testcase.extra)
			if testcase.second != "" {
				writeTestFile(t, directory, "warehouse.yaml", testcase.second)
			}
			_, err := Load(directory, testLookup)
			if err == nil || !strings.Contains(err.Error(), testcase.want) {
				t.Fatalf("错误=%v，需要包含 %s", err, testcase.want)
			}
		})
	}
}

// TestCredentialReuse 覆盖“变量名不同，但实际值仍共用”的部署错误。
func TestCredentialReuse(t *testing.T) {
	for _, testcase := range []struct{ name, extra, value string }{
		{"令牌", "", strings.Repeat("same-demo-token-", 3)},                                                      // 相同令牌会让跨系统推送通过。
		{"MySQL账号", "mysql_readonly_dsn_env: %s_MYSQL_RO_DSN\n", "diag_ro:demo-only@tcp(localhost:3306)/shop"}, // 同主机同用户名仍是同一个数据库账号。
		{"Redis账号", "redis_readonly_url_env: %s_REDIS_RO_URL\n", "redis://diag_ro:demo-only@localhost:6379/0"}, // 两个变量不能掩盖同一个 ACL 账号。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			directory := t.TempDir()
			for _, name := range []string{"shop", "warehouse"} {
				extra := ""
				if testcase.extra != "" {
					extra = fmt.Sprintf(testcase.extra, strings.ToUpper(name))
				}
				writeConfig(t, directory, name, extra)
			}
			lookup := func(name string) (string, bool) {
				if testcase.extra != "" && strings.HasSuffix(name, "ALERT_TOKEN") {
					return testLookup(name)
				}
				return testcase.value, true
			}
			_, err := Load(directory, lookup)
			if err == nil || !strings.Contains(err.Error(), "共用") || strings.Contains(err.Error(), "demo-only") {
				t.Fatalf("共用凭据错误=%v", err)
			}
		})
	}
}

// metricsServer 只接受固定 Prometheus 查询路径，未知路径不能被测试悄悄放过。
func metricsServer(t *testing.T, value int, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"service":"api"},"value":[100,"%d"]}]}}`, value)
	}))
	t.Cleanup(server.Close)
	return server
}

// writeConfig 给每个系统独立的说明书和令牌变量，测试只覆盖它关心的差异。
func writeConfig(t *testing.T, directory, name, extra string) {
	t.Helper()
	writeTestFile(t, directory, name+".yaml", fmt.Sprintf("name: %s\nprofile: %s.profile.md\nalert_token_env: %s_ALERT_TOKEN\n%s", name, name, strings.ToUpper(name), extra))
	writeTestFile(t, directory, name+".profile.md", "# 架构\n一个 HTTP 服务及其指标后端。\n")
}

// writeTestFile 使用临时目录，不改变仓库里的示例配置。
func writeTestFile(t *testing.T, directory, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// testLookup 只提供虚构凭据，错误路径也不会打印真实密码。
func testLookup(name string) (string, bool) {
	switch name {
	case "SHOP_ALERT_TOKEN":
		return strings.Repeat("s", 32), true
	case "WAREHOUSE_ALERT_TOKEN":
		return strings.Repeat("w", 32), true
	case "SHOP_MYSQL_RO_DSN":
		return "diag_ro:demo-only@tcp(localhost:3306)/shop", true
	}
	return "", false
}
