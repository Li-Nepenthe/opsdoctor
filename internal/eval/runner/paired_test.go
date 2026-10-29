// 本文件：用虚构运行状态检查配对身份、随机排列和续跑矩阵。
//
// 主要内容：TestPairedPlanBoundary 验证每轮五配置各一次；TestGraderMock 检查免费总开关。
// 阅读建议：测试不启动容器、不注入故障，也不会读取模型 Key。

package runner

import (
	"io"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestPairedPlanBoundary 同一组 E1 在两个模型下必须各有一个身份，不能被去重成一份。
func TestPairedPlanBoundary(t *testing.T) {
	status := runtimeStatus{Provider: "mock", Executions: map[string]run.ExecutionStatus{
		"primary":    {Model: "mock-primary", Provider: "mock", Account: "usd", Budget: run.BudgetStatus{Currency: "USD"}},
		"comparison": {Model: "mock-comparison", Provider: "mock", Account: "cny", Budget: run.BudgetStatus{Currency: "CNY"}},
	}}
	values, err := configurations(status, "mock-primary")
	if err != nil || len(values) != 5 {
		t.Fatal("五配置不完整", err)
	}
	orders := []trialOrder{}
	for iteration := 1; iteration <= 3; iteration++ {
		order, err := shuffled(values)
		if err != nil {
			t.Fatal(err)
		}
		orders = append(orders, trialOrder{Scenario: "S01", Iteration: iteration, Configurations: order})
	}
	if err = validateOrders(orders, []string{"S01"}, 3, values); err != nil {
		t.Fatal(err)
	}
	// 删除一轮、复制一份配置、换账户，都不能被当成合法续跑清单。
	if err = validateOrders(orders[:2], []string{"S01"}, 3, values); err == nil {
		t.Fatal("漏掉一轮")
	}
	old := orders[0].Configurations[0]
	orders[0].Configurations[0] = orders[0].Configurations[1]
	if err = validateOrders(orders, []string{"S01"}, 3, values); err == nil {
		t.Fatal("重复组被接受")
	}
	orders[0].Configurations[0] = old
	orders[0].Configurations[0].Account = "another"
	if err = validateOrders(orders, []string{"S01"}, 3, values); err == nil {
		t.Fatal("续跑改变账户")
	}
	second := status.Executions["comparison"]
	second.Account = "usd"
	status.Executions["comparison"] = second
	if _, err = configurations(status, "mock-primary"); err == nil {
		t.Fatal("两币种共用账户")
	}
}

// TestGraderMock 即使 --allow-paid 为真，医生的 Mock 总开关仍不允许读取收费配置。
func TestGraderMock(t *testing.T) {
	reader := config.NewReader(func(string) (string, bool) { t.Fatal("Mock 读取了收费配置"); return "", false })
	// 使用空仓储仅构造视图；没有请求，也不会访问数据库。
	value, err := newGrader(t.Context(), reader, store.New(nil), runtimeStatus{Provider: "mock"}, true, io.Discard)
	if err != nil || value.provider != "mock" {
		t.Fatal("Mock 评委启用了收费路径", err)
	}
}
