// 本文件：在专用医生测试库验证迁移、通知事务和分组去重。
//
// 不连接保存真实诊断任务的 doctor 库。每个用例用独立系统名和邮箱，只清理自己
// 创建的记录；不清空整张表，也不删除别人的验收数据。
//
// 主要内容：
//   - TestIntegrationAlertDeduplication：三次推送和并发推送都只创建一个任务。
//   - TestIntegrationAlertRollback：告警行写失败时，任务也必须回滚。
//   - TestIntegrationUserBootstrap：重复初始化不能重置已有密码。
//   - integrationStore、testNotification：准备专用库和虚构告警。
//
// 阅读建议：这里证明真实唯一约束和事务，HTTP 的令牌隔离另在 httpapi 测试。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// integrationStore 在专用库上重复应用迁移；清理按本次系统名，不影响同时运行的其他用例。
func integrationStore(t *testing.T) (*Store, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DOCTOR_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_DOCTOR_MYSQL_DSN，库名必须为 opsdoctor_doctor_test")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_doctor_test" {
		t.Fatal("拒绝在非专用医生测试库执行")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal("无法连接医生测试库")
	}
	if err := Migrate(t.Context(), database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	target := "test-" + newID()[:20]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// 先删子表，再删本次测试的任务，遵守外键关系。
		for _, table := range []string{"diagnosis_steps", "llm_attempts"} {
			if _, err := database.ExecContext(ctx, "DELETE FROM "+table+" WHERE diagnosis_id IN (SELECT id FROM diagnoses WHERE target IN (?,?))", target, target+"-b"); err != nil {
				t.Error(err)
			}
		}
		if _, err := database.ExecContext(ctx, "DELETE FROM alert_groups WHERE target IN (?,?)", target, target+"-b"); err != nil {
			t.Error(err)
		}
		if _, err := database.ExecContext(ctx, "DELETE FROM diagnosis_alerts WHERE diagnosis_id IN (SELECT id FROM diagnoses WHERE target IN (?,?))", target, target+"-b"); err != nil {
			t.Error(err)
		}
		if _, err := database.ExecContext(ctx, "DELETE FROM diagnoses WHERE target IN (?,?)", target, target+"-b"); err != nil {
			t.Error(err)
		}
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return NewConfigured(database, Config{RecoveryWindows: map[string]time.Duration{target: 3 * time.Minute, target + "-b": time.Minute}}), target
}

// testNotification 使用明显的验收名称，不能误看成业务故障证据。
func testNotification() alerts.Notification {
	return alerts.Notification{Status: "firing", GroupKey: "test-group", GroupLabels: map[string]string{"alertname": "TestOnly", "service": "fictional-api"}, Alerts: []alerts.Alert{{Status: "firing", Fingerprint: "0123456789abcdef", Labels: map[string]string{"alertname": "TestOnly"}, StartsAt: time.Now().UTC()}}}
}

// TestIntegrationAlertDeduplication 先按要求顺序推三次，再加并发推送检查竞态。
func TestIntegrationAlertDeduplication(t *testing.T) {
	store, target := integrationStore(t)
	value := testNotification()
	service := alerts.New(store)
	id := ""
	for index := 0; index < 3; index++ {
		receipt, err := service.Receive(t.Context(), target, value)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			id = receipt.DiagnosisID
		}
		if receipt.DiagnosisID != id || receipt.Created != (index == 0) {
			t.Fatalf("重复通知=%+v", receipt)
		}
	}
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			receipt, err := service.Receive(t.Context(), target, value)
			if err != nil || receipt.Created || receipt.DiagnosisID != id {
				t.Errorf("并发重发失败：%v %+v", err, receipt)
			}
		})
	}
	workers.Wait()
	list, err := store.List(t.Context(), target, "pending", 20, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("任务数=%d err=%v", len(list), err)
	}
	other, err := service.Receive(t.Context(), target+"-b", value)
	if err != nil || !other.Created || other.DiagnosisID == id {
		t.Fatal("两个系统共用了一个任务")
	}
	// 恢复只更新快照，不把 pending 任务删掉或自动执行。
	value.Status = "resolved"
	value.Alerts[0].Status = "resolved"
	receipt, err := service.Receive(t.Context(), target, value)
	if err != nil || receipt.Created {
		t.Fatal("恢复通知失败")
	}
	detail, err := store.Detail(t.Context(), id)
	if err != nil || detail.Status != "pending" || len(detail.Alerts) != 1 {
		t.Fatalf("详情=%+v err=%v", detail, err)
	}
	var snapshot alerts.Alert
	if json.Unmarshal(detail.Alerts[0], &snapshot) != nil || snapshot.Status != "resolved" {
		t.Fatal("恢复快照没有保存")
	}
	value.GroupKey = "never-seen"
	receipt, err = service.Receive(t.Context(), target, value)
	if err != nil || !receipt.Ignored {
		t.Fatal("单独恢复通知不应创建任务")
	}
	t.Log("同组顺序 3 次和并发 20 次仍只有 1 个 pending 任务；另一个系统创建独立任务")
}

// TestIntegrationAlertRollback 直接调用仓储制造子表失败，证明任务不会独自留在库里。
func TestIntegrationAlertRollback(t *testing.T) {
	store, target := integrationStore(t)
	value := testNotification()
	value.Alerts[0].Fingerprint = strings.Repeat("a", 17) // 超过 CHAR(16)；服务层本来就会提前拒绝。
	if _, err := store.ReceiveAlerts(t.Context(), target, value); err == nil {
		t.Fatal("过长指纹没有触发存储失败")
	}
	values, err := store.List(t.Context(), target, "", 20, 0)
	if err != nil || len(values) != 0 {
		t.Fatal("事务失败后留下了任务")
	}
}

// TestIntegrationUserBootstrap 第二次初始化不能悄悄改掉用户已经在用的密码摘要。
func TestIntegrationUserBootstrap(t *testing.T) {
	// 先保存原密码摘要，再用不同初始密码重复启动，检查既有账号不会被重置。
	store, _ := integrationStore(t)
	email := "test-" + newID() + "@example.invalid"
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = store.database.ExecContext(ctx, "DELETE FROM users WHERE email=?", email)
	})
	for _, hash := range []string{"first-demo-hash", "second-demo-hash"} {
		if err := store.EnsureUser(t.Context(), email, hash); err != nil {
			t.Fatal(err)
		}
	}
	user, err := store.UserByEmail(t.Context(), email)
	if err != nil || user.PasswordHash != "first-demo-hash" || user.Status != "active" {
		t.Fatal("已有账号被覆盖")
	}
	if _, err := store.UserByEmail(t.Context(), "missing-"+email); err != sql.ErrNoRows {
		t.Fatal("未知邮箱没有返回未找到")
	}
}
