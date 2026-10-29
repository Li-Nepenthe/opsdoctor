// 本文件：用真实事务验证恢复观察期既防止重复诊断，也不会永远吞掉下一场故障。
//
// 时间由测试推进，不真的等三分钟。模型不参与，原报告、步骤数和任务数都从数据库核对。
// 两个系统使用不同观察期，证明这不是隐藏的全局配置。
//
// 主要内容：TestIntegrationRecoveryWindow 覆盖晚到告警、重复恢复、期限边界与页面快照。
// 阅读建议：每次推进 now 都代表通知到达时间，不修改告警规则或放宽实际验收时间。

package store

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
)

// TestIntegrationRecoveryWindow 先结束报告，再接收恢复与后续症状，证明不会新增付费任务。
func TestIntegrationRecoveryWindow(t *testing.T) {
	repository, target := integrationStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	repository.config.Now = func() time.Time { return now }
	notification := testNotification()
	first, err := repository.ReceiveAlerts(t.Context(), target, notification)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StartTask(llm.WithLease(t.Context(), "test-owner"), first.DiagnosisID, "mock-test", "v-test", "mock"); err != nil {
		t.Fatal(err)
	}
	if err := repository.EndTask(t.Context(), first.DiagnosisID, run.Succeeded, agent.Result{}, ""); err != nil {
		t.Fatal(err)
	}
	before, err := repository.Detail(t.Context(), first.DiagnosisID)
	if err != nil {
		t.Fatal(err)
	}
	// A 的窗口为三分钟；B 为一分钟，同样时刻恢复也应在不同时间关闭。
	other, err := repository.ReceiveAlerts(t.Context(), target+"-b", notification)
	if err != nil || !other.Created {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	notification.Status, notification.Alerts[0].Status = "resolved", "resolved"
	notification.Alerts[0].EndsAt = now
	for _, name := range []string{target, target + "-b"} {
		if _, err := repository.ReceiveAlerts(t.Context(), name, notification); err != nil {
			t.Fatal(err)
		}
	}
	deadline := now.Add(3 * time.Minute)
	now = now.Add(time.Minute)
	for _, testcase := range []struct {
		target string
		open   bool
	}{
		{target, true},         // A 仍在观察，报告已经成功也不影响它。
		{target + "-b", false}, // B 恰好达到自己的期限，不能借用 A 的时长。
	} {
		state, err := repository.FaultState(t.Context(), testcase.target)
		if err != nil || state.Open != testcase.open {
			t.Fatalf("%s: %+v %v", testcase.target, state, err)
		}
	}
	// 重复的 resolved 不能从这一刻重新计算三分钟。
	if _, err := repository.ReceiveAlerts(t.Context(), target, notification); err != nil {
		t.Fatal(err)
	}
	state, err := repository.FaultState(t.Context(), target)
	if err != nil || state.RecoveryUntil == nil || !state.RecoveryUntil.Equal(deadline) {
		t.Fatalf("期限被重置: %+v %v", state, err)
	}
	notification.Status = "firing"
	notification.Alerts = append(notification.Alerts, alerts.Alert{Status: "firing", Fingerprint: "fedcba9876543210", Labels: map[string]string{"alertname": "LateSymptom"}, StartsAt: now})
	lateAt := now
	late, err := repository.ReceiveAlerts(t.Context(), target, notification)
	if err != nil || late.Created || !late.RecoveryFollowup || late.DiagnosisID != first.DiagnosisID {
		t.Fatalf("后续通知=%+v %v", late, err)
	}
	state, err = repository.FaultState(t.Context(), target)
	if err != nil || !state.Open || state.RecoveryUntil != nil {
		t.Fatalf("再次 firing 应等待整组恢复: %+v %v", state, err)
	}
	// 重发已并入的 firing 不能重复标记到达时间，也不能把已恢复的旧告警标成后续告警。
	now = now.Add(time.Second)
	if _, err := repository.ReceiveAlerts(t.Context(), target, notification); err != nil {
		t.Fatal(err)
	}
	after, err := repository.Detail(t.Context(), first.DiagnosisID)
	if err != nil || after.Status != before.Status || after.LastSeq != before.LastSeq || len(after.RecoveryAlerts) != 1 || !after.RecoveryAlerts[0].ReceivedAt.Equal(lateAt) {
		t.Fatalf("后续快照或原诊断被改写: %v", err)
	}
	var followup alerts.Alert
	if err := json.Unmarshal(after.RecoveryAlerts[0].Alert, &followup); err != nil || followup.Labels["alertname"] != "LateSymptom" {
		t.Fatal("未保存晚到症状")
	}
	items, err := repository.List(t.Context(), target, "", 20, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("同一故障创建 %d 个诊断: %v", len(items), err)
	}
	for i := range notification.Alerts {
		notification.Alerts[i].Status = "resolved"
		notification.Alerts[i].EndsAt = now
	}
	notification.Status = "resolved"
	if _, err := repository.ReceiveAlerts(t.Context(), target, notification); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Minute)
	state, err = repository.FaultState(t.Context(), target)
	if err != nil || state.Open {
		t.Fatalf("到期仍开放: %+v %v", state, err)
	}
	// 到期后才允许创建下一场任务；原故障的后续告警记录仍能查询。
	next, err := repository.ReceiveAlerts(t.Context(), target, testNotification())
	if err != nil || !next.Created || next.DiagnosisID == first.DiagnosisID {
		t.Fatalf("新故障=%+v %v", next, err)
	}
	history, err := repository.DiagnosisAlerts(t.Context(), first.DiagnosisID)
	if err != nil || len(history.RecoveryAlerts) != 1 {
		t.Fatal("恢复后丢失后续告警标记")
	}
	manual, err := repository.Manual(t.Context(), target, "单独调查")
	if err != nil {
		t.Fatal(err)
	}
	history, err = repository.DiagnosisAlerts(t.Context(), manual)
	if err != nil || len(history.Alerts) != 0 || len(history.RecoveryAlerts) != 0 {
		t.Fatal("手动诊断不应凭空出现告警")
	}
	if _, err := repository.DiagnosisAlerts(t.Context(), newID()); err != sql.ErrNoRows {
		t.Fatal("不存在的诊断应返回未找到")
	}
}
