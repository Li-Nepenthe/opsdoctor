// 本文件：从验收一侧核对场景答案是否使用架构说明中的组件标识。
//
// 测试放在 chaos 中，只有这里读取标准答案。医生的源码和容器仍然不知道场景
// 文件在哪里，也不能通过这个测试获得任何额外权限。
//
// 主要内容：TestGroundTruthComponents 检查全部场景的组件在架构命名表中存在。
// 阅读建议：本测试只核对名字，不执行注入，不给模型提供答案。

package scenario

import (
	"os"
	"strings"
	"testing"
)

// TestGroundTruthComponents 防止未来精确比较时，把同一位置的不同拼法算成诊断错误。
func TestGroundTruthComponents(t *testing.T) {
	data, err := os.ReadFile("../../../deploy/doctor/targets/shop.profile.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(data), "<!-- COMPONENTS_START -->")
	if !ok {
		t.Fatal("没有组件命名表")
	}
	table, _, ok := strings.Cut(rest, "<!-- COMPONENTS_END -->")
	if !ok {
		t.Fatal("组件命名表未结束")
	}
	values, err := Load("../../../eval/scenarios")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		// 精确匹配表格第一列，不能靠正文里恰好出现了这个字符串就算通过。
		if !strings.Contains(table, "| `"+value.GroundTruth.Component+"` |") {
			t.Errorf("%s 的组件 %q 不在架构命名表", value.ID, value.GroundTruth.Component)
		}
	}
}
