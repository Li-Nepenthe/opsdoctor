// 本文件：通过库存进程的配置入口记录数据库参数与两个监听地址，测试止于配置加载。

package inventoryservice

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestInventoryEnvironmentContract 核对库存服务实际读取的变量，以及该服务与共用连接池最终使用的默认值。
func TestInventoryEnvironmentContract(t *testing.T) {
	keys := map[string]bool{}
	settings, err := loadConfig(config.NewReader(func(name string) (string, bool) {
		keys[name] = true
		return "test-only", name == "MYSQL_DSN"
	}))
	want := strings.Fields("ADMIN_ADDR DB_CONN_MAX_LIFETIME DB_MAX_IDLE_CONNS DB_MAX_OPEN_CONNS GRPC_ADDR MYSQL_DSN")
	if err != nil || !slices.Equal(slices.Sorted(maps.Keys(keys)), want) {
		t.Fatalf("环境查找：%v %v", keys, err)
	}
	if settings.AdminAddress != ":6062" || settings.GRPCAddress != ":9001" ||
		settings.Database.MaxOpenConns != 20 || settings.Database.MaxIdleConns != 10 ||
		settings.Database.ConnMaxLifetime != 5*time.Minute {
		t.Fatalf("库存配置：%+v", settings)
	}
}
