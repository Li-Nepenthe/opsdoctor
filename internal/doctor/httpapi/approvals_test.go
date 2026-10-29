// 本文件：证明批准接口只能由人使用，并且无法夹带新的执行参数。
//
// 假审批仓储只记录请求是否到达。登录、Cookie 和 CSRF 都使用真实路由，避免测试
// 绕过最需要检查的边界。
//
// 主要内容：TestApprovalHTTP 检查身份、CSRF、参数替换和批准人来源。
// 阅读建议：数据库里的双批准竞争另由 store 的集成测试验证。

package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
)

// fakeApprovals 保存被允许穿过 HTTP 边界的决定，不能把拒绝请求算成通过。
type fakeApprovals struct {
	calls    int
	user     string
	approved bool
}

// Approvals 返回展示数据，接口不能重新生成参数。
func (*fakeApprovals) Approvals(context.Context, string) ([]approval.Record, error) {
	return []approval.Record{}, nil
}

// DecideApproval 保存接口传来的登录人，检查身份来自令牌。
func (fake *fakeApprovals) DecideApproval(_ context.Context, _, user string, approved bool) (bool, error) {
	fake.calls++
	fake.user = user
	fake.approved = approved
	return true, nil
}

// TestApprovalHTTP 参数来自原单；人点击按钮只改变决定状态。
func TestApprovalHTTP(t *testing.T) {
	store := &fakeApprovals{}
	router, _, bearer := setupWithOptions(t, func(options *Options) { options.Approvals = store })
	path := "/api/v1/approvals/" + strings.Repeat("a", 32) + "/approve"
	if apiCall(router, "POST", path, "", []byte(`{}`)).Code != 401 {
		t.Fatal("匿名批准")
	}
	// 即使已经登录，也不能在批准时换参数或伪造决定人。
	if apiCall(router, "POST", path, bearer, []byte(`{"size":200,"decided_by":"someone"}`)).Code != 400 || store.calls != 0 {
		t.Fatal("更改审批参数")
	}
	login := apiCall(router, "POST", "/api/v1/auth/login", "", []byte(`{"email":"learner@example.invalid","password":"demo-password-only"}`))
	var body struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &body)
	for _, test := range []struct {
		name, csrf string
		status     int
	}{{"自动附带 Cookie 不够", "", 403}, {"当前页面确认", body.Data.CSRF, 200}} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", test.csrf)
			r.AddCookie(login.Result().Cookies()[0])
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatal(w.Code)
			}
		})
	}
	if store.calls != 1 || store.user != "test-user" || !store.approved {
		t.Fatal("批准人身份或次数错误")
	}
}
