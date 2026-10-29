// 本文件：给进程崩溃验收准备独立的医生测试库记录。
//
// 终止进程是验收程序的能力，医生本身仍不能导入 os/exec。这里通过医生公开的
// 仓储接口保存步骤，只有核对租约和清理本次记录时直接查询专用测试库。
//
// 主要内容：crashRepository 同时保留生产仓储和测试连接；integrationStore 限定专用库并清理本次记录。
// 阅读建议：清理条件始终带本次随机系统名，不能清空共享测试库。

package recoverycheck

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// crashRepository 嵌入生产 Store，测试仍调用同一套事务和租约实现。
type crashRepository struct {
	*store.Store
	database *sql.DB
}

// integrationStore 只连接明确命名的测试库，缺配置时说明跳过原因。
func integrationStore(t *testing.T) (*crashRepository, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DOCTOR_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_DOCTOR_MYSQL_DSN，库名必须为 opsdoctor_doctor_test")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName != "opsdoctor_doctor_test" {
		t.Fatal("拒绝使用非专用医生测试库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal("无法连接医生测试库")
	}
	if err = store.Migrate(t.Context(), database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	target := "crash-" + strings.ToLower(rand.Text())[:20]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// 子记录跟随本次诊断删除；不修改其他包正在使用的测试任务。
		for _, table := range []string{"diagnosis_steps", "llm_attempts", "diagnosis_alerts"} {
			if _, err := database.ExecContext(ctx, "DELETE FROM "+table+" WHERE diagnosis_id IN (SELECT id FROM diagnoses WHERE target=?)", target); err != nil {
				t.Error(err)
			}
		}
		if _, err := database.ExecContext(ctx, "DELETE FROM diagnoses WHERE target=?", target); err != nil {
			t.Error(err)
		}
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return &crashRepository{store.NewConfigured(database, store.Config{LeaseTTL: 2 * time.Second}), database}, target
}
