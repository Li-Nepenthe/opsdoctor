// 本文件：读取仓库内随医生部署的目标清单，固定两套目标实际要求的凭据变量。

package target

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestTargetEnvironmentContract 只执行目标凭据的解析，配置来自实际部署文件，返回的用户名和令牌都是测试样例。
func TestTargetEnvironmentContract(t *testing.T) {
	configs, err := readConfigs(filepath.Join("..", "..", "..", "deploy", "doctor", "targets"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"OTEL_DEMO_ALERT_TOKEN": strings.Repeat("a", 32), "SHOP_ALERT_TOKEN": strings.Repeat("b", 32),
		"SHOP_MCP_TOKEN": strings.Repeat("c", 32), "SHOP_OPS_TOKEN": strings.Repeat("d", 32),
		"SHOP_MYSQL_RO_DSN": "reader:test@tcp(localhost:3306)/shop", "SHOP_MYSQL_DDL_DSN": "ddl:test@tcp(localhost:3306)/shop",
		"SHOP_REDIS_RO_URL": "redis://reader:test@localhost:6379/0",
	}
	// 未知变量返回缺失；若有目标不再读原有变量，结果中也必须能发现少项。
	got, err := credentials(configs, func(name string) (string, bool) {
		v, ok := fixtures[name]
		return v, ok
	})
	if err != nil || !slices.Equal(slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(fixtures))) {
		t.Fatalf("部署变量改变：%v %v", slices.Sorted(maps.Keys(got)), err)
	}
}
