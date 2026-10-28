// 本文件：证明客户端能读取故障开放状态，也能单独刷新后来到达的告警。
//
// 页面不需要读取工具审计才知道有没有晚到告警。验收脚本也不用猜测三分钟何时开始，
// 它读取目标系统的 fault.open 和 recovery_until，等医生确认这次故障结束。
//
// 主要内容：TestRecoveryEndpoints 检查按系统返回状态、后续告警和未登录拒绝。
// 阅读建议：事务和时间边界在 store 测试，这里只证明 HTTP 不丢字段或混系统。

package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// recoveryStore 只替换这次要观察的读取方法，其他身份行为沿用共同替身。
type recoveryStore struct{ memoryStore }

// FaultState 给 A 一个明确截止时间，B 无故障，检查接口是否保留系统差异。
func (*recoveryStore) FaultState(_ context.Context, target string) (store.FaultState, error) {
	if target != "alpha" {
		return store.FaultState{}, nil
	}
	until := time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)
	return store.FaultState{Open: true, RecoveryUntil: &until}, nil
}

// DiagnosisAlerts 返回带脚本文字的告警，接口只能传数据，页面另以 textContent 展示。
func (*recoveryStore) DiagnosisAlerts(context.Context, string) (store.AlertHistory, error) {
	return store.AlertHistory{Alerts: []json.RawMessage{}, RecoveryAlerts: []store.RecoveryAlert{{ReceivedAt: time.Date(2026, 10, 28, 6, 59, 0, 0, time.UTC), Alert: json.RawMessage(`{"labels":{"alertname":"<script>example</script>"},"status":"firing"}`)}}}, nil
}

// TestRecoveryEndpoints 使用真实登录路由，查询等待信息不需要额外权限或内部端点。
func TestRecoveryEndpoints(t *testing.T) {
	router, _, login := setupWithOptions(t, func(options *Options) { options.Store = &recoveryStore{} })
	response := apiCall(router, "GET", "/api/v1/targets", login, nil)
	var body struct {
		Data []struct {
			Name  string           `json:"name"`
			Fault store.FaultState `json:"fault"`
		} `json:"data"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Data) != 2 {
		t.Fatalf("无法读取系统: %d", response.Code)
	}
	if body.Data[0].Name != "alpha" || !body.Data[0].Fault.Open || body.Data[0].Fault.RecoveryUntil == nil || body.Data[1].Fault.Open || body.Data[1].Fault.RecoveryUntil != nil {
		t.Fatal("开放状态串到另一个系统或期限丢失")
	}
	path := "/api/v1/diagnoses/" + strings.Repeat("a", 32) + "/alerts"
	response = apiCall(router, "GET", path, login, nil)
	var history struct {
		Data store.AlertHistory `json:"data"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &history) != nil || len(history.Data.RecoveryAlerts) != 1 || history.Data.RecoveryAlerts[0].ReceivedAt.IsZero() {
		t.Fatal("没有单独返回后续告警及到达时间")
	}
	// 轻量接口仍受同一登录保护，不因只返回少量内容就允许匿名读取。
	if apiCall(router, "GET", path, "", nil).Code != 401 {
		t.Fatal("匿名读取了告警")
	}
	if apiCall(router, "GET", "/api/v1/diagnoses/not-an-id/alerts", login, nil).Code != 400 {
		t.Fatal("接受了非法诊断编号")
	}
}
