// 本文件：检查场景定义在执行前能否挡住错误输入。
//
// 测试直接加载仓库中的十一个场景，也会构造拼错字段、越界参数和不支持的阶段编号。
// 所有测试都只读文件，不执行场景中的动作。
//
// 主要内容：
//   - TestRepositoryScenarios：确认实际交付的场景全部可加载。
//   - TestDecodeRejectsInvalid：未知字段和缺少恢复信息不能通过。
//   - TestStepBoundary：拒绝任意 SQL、服务名和秘密配置。
//
// 阅读建议：每条表格用例旁的说明就是它防止的误操作。

package scenario

import (
	"os"
	"strings"
	"testing"
)

// TestRepositoryScenarios 直接检查交付文件，避免测试样本正确而真实 YAML 已经写错。
func TestRepositoryScenarios(t *testing.T) {
	values, err := Load("../../../eval/scenarios")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 11 {
		t.Fatalf("场景数=%d", len(values))
	}
}

// TestDecodeRejectsInvalid 逐项修改一份有效场景，确认坏输入在外部操作之前被拒绝。
func TestDecodeRejectsInvalid(t *testing.T) {
	data, err := os.ReadFile("../../../eval/scenarios/S01.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, testcase := range []struct {
		name   string // 描述应被拒绝的原因。
		before string // 原文件中的唯一片段。
		after  string // 对应错误写法。
	}{
		{"未知字段", "title:", "titel:"},                    // 拼错字段不能当成默认值继续。
		{"超长预算", "within: 3m", "within: 4m"},            // 验收不能偷偷延长三分钟限制。
		{"尚未授权阶段", "id: S01", "id: S11"},                // 阶段 8 的对照场景暂不开放。
		{"错误索引", "index: uk_stock_sku", "index: other"}, // 不允许场景改掉其他索引。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			changed := strings.Replace(string(data), testcase.before, testcase.after, 1)
			if _, err := Decode([]byte(changed)); err == nil {
				t.Fatal("错误场景被接受")
			}
		})
	}
}

// TestStepBoundary 即使直接调用执行器，也只能使用这些受限动作。
func TestStepBoundary(t *testing.T) {
	for _, testcase := range []struct {
		name string // 说明本行拦截的越界方式。
		step Step   // 省略的字段为零值。
	}{
		{"拼 SQL", Step{Action: "drop_index", Table: "stock; DROP TABLE orders", Index: "uk_stock_sku"}},                      // 标识符不是任意文本。
		{"错配索引", Step{Action: "drop_index", Table: "orders", Index: "uk_stock_sku"}},                                         // 合法名字也不能错配。
		{"停止数据库", Step{Action: "stop", Service: "shop-mysql"}},                                                               // 当前阶段只有停止消费者这一种容器动作。
		{"修改密码", Step{Action: "deploy", Service: "order-service", Environment: map[string]string{"MYSQL_PASSWORD": "demo"}}}, // 发布记录不能携带秘密。
		{"任意镜像", Step{Action: "deploy", Service: "order-service", Image: "untrusted:latest"}},                                // 只发布事先准备的两个项目版本。
		{"任意代理", Step{Action: "latency", Proxy: "elsewhere", Stream: "downstream", LatencyMS: 100}},                          // 不能让工具影响别的项目。
		{"缓存越界", Step{Action: "delete_cache", FirstSKUID: 500000, Count: 2}},                                                 // 连续范围不得超出现有数据规模。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if err := ValidateStep(testcase.step); err == nil {
				t.Fatal("越界动作被接受")
			}
		})
	}
}
