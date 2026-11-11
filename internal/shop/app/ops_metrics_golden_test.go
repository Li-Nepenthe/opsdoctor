// 本文件：从业务进程的运维入口启动注册表，固定连接池修改标志的指标名称、说明和服务标签。

package app

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestOpsPoolMetricGolden 在回环地址上启动服务，核对刚启动时尚未修改过连接池的指标。
func TestOpsPoolMetricGolden(t *testing.T) {
	t.Setenv("OPS_ADDR", "127.0.0.1:0")
	t.Setenv("OPS_TOKEN", strings.Repeat("fixture-", 5))
	common := telemetry.NewMetrics("inventory-service")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// sql.Open 只构造连接池；StartOps 只读池统计，测试不会向 MySQL 发起连接。
	database, err := sql.Open("mysql", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	shutdown, err := StartOps(ctx, cancel, database, nil, common)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := shutdown(); err != nil {
			t.Error(err)
		}
	})
	want, err := os.ReadFile("testdata/ops_pool_metric.golden.prom")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GatherAndCompare(common.Registry, bytes.NewReader(want), "db_pool_runtime_changed"); err != nil {
		t.Fatal(err)
	}
}
