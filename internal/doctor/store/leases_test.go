// 本文件：用锁内可读到的诊断行与名额计数核对租约决定。
// TestDecideClaim 逐项改变评测绑定、恢复条件和剩余名额，并检查多项条件同时不满足时先返回哪个错误。

package store

import "testing"

func TestDecideClaim(t *testing.T) {
	config := Config{ExecutionProfile: "primary", BudgetAccount: "operations", Currency: "CNY"}
	request := claimRequest{model: "script", provider: "mock"}
	pending := claimRow{status: "pending", currency: "CNY"}
	running := claimRow{status: "running", currency: "CNY", available: true, recent: true}
	waiting := claimRow{status: "waiting_approval", currency: "CNY"}
	eval := claimRow{
		status: "pending", evaluating: true, trigger: "eval", expectedModel: "script",
		profile: "primary", expectedProvider: "mock", account: "operations", currency: "CNY",
	}
	const (
		wrongModel    = "评测模式未开启或运行模型与实验配置不一致"
		wrongBinding  = "评测绑定的执行配置、供应商、账户或币种不一致"
		wrongCurrency = "不能跨币种恢复诊断，请保持原账户或取消后重新发起"
	)
	for _, tc := range []struct {
		name    string
		row     claimRow
		change  func(*claimRow)
		want    bool
		wantErr string
	}{
		{"普通待领取", pending, nil, true, ""},
		{"评测中跳过普通诊断", pending, func(r *claimRow) { r.evaluating = true }, false, ""},
		{"评测绑定相符", eval, nil, true, ""},
		{"评测已关闭", eval, func(r *claimRow) { r.evaluating = false }, false, wrongModel},
		{"评测模型不符", eval, func(r *claimRow) { r.expectedModel = "other" }, false, wrongModel},
		{"评测配置不符", eval, func(r *claimRow) { r.profile = "other" }, false, wrongBinding},
		{"评测供应商不符", eval, func(r *claimRow) { r.expectedProvider = "other" }, false, wrongBinding},
		{"评测账户不符", eval, func(r *claimRow) { r.account = "other" }, false, wrongBinding},
		{"评测币种不符", eval, func(r *claimRow) { r.currency = "USD" }, false, wrongBinding},
		{"模型错误先于配置错误", eval, func(r *claimRow) { r.expectedModel, r.profile = "other", "other" }, false, wrongModel},
		{"配置错误先于恢复币种错误", eval, func(r *claimRow) { r.status, r.currency = "running", "USD" }, false, wrongBinding},
		{"空配置跳过执行绑定", eval, func(r *claimRow) { r.profile, r.expectedProvider = "", "other" }, true, ""},
		{"新诊断可以选择币种", pending, func(r *claimRow) { r.currency = "USD" }, true, ""},
		{"恢复必须沿用原币种", running, func(r *claimRow) { r.currency = "USD" }, false, wrongCurrency},
		{"恢复币种错误先于取消", running, func(r *claimRow) { r.currency, r.cancelled = "USD", true }, false, wrongCurrency},
		{"已取消", pending, func(r *claimRow) { r.cancelled = true }, false, ""},
		{"终态", pending, func(r *claimRow) { r.status = "succeeded" }, false, ""},
		{"运行中租约仍有效", running, func(r *claimRow) { r.available = false }, false, ""},
		{"运行中证据过时", running, func(r *claimRow) { r.recent = false }, false, ""},
		{"租约过期且证据仍新", running, nil, true, ""},
		{"仍有待决定审批", waiting, func(r *claimRow) { r.undecided = 1 }, false, ""},
		{"审批全部已有决定", waiting, nil, true, ""},
		{"同系统名额已占", pending, func(r *claimRow) { r.sameTarget = 1 }, false, ""},
		{"全局还剩一个名额", pending, func(r *claimRow) { r.active = 1 }, true, ""},
		{"全局两个名额已满", pending, func(r *claimRow) { r.active = 2 }, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := tc.row
			if tc.change != nil {
				tc.change(&row)
			}
			ok, err := decideClaim(row, config, request)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if ok != tc.want || gotErr != tc.wantErr {
				t.Fatalf("决定=(%v, %q)，期望=(%v, %q)", ok, gotErr, tc.want, tc.wantErr)
			}
		})
	}
}
