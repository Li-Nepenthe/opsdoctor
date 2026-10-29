// 本文件：验证列表标题在系统级告警分组下仍能说明发生了什么。
//
// 分组键只负责去重，不再包含告警名。标题必须读通知里的告警，不能退回空泛的
// “收到告警分组”。这里也检查顺序变化不会改变展示名称。
//
// 主要内容：TestTitle 检查单条、多条和乱序；TestTitleBound 检查数据库长度边界。
// 阅读建议：标题不参与去重，改变标题不能创建第二个任务。

package alerts

import (
	"strings"
	"testing"
)

// TestTitle 不需要分组标签，也能从第一批通知生成具体标题。
func TestTitle(t *testing.T) {
	for _, testcase := range []struct {
		name  string
		names []string
		want  string
	}{
		{"单条", []string{"InventoryCallFailures"}, "InventoryCallFailures"},                             // 只有一个症状时直接显示名称。
		{"多条", []string{"InventoryCallFailures", "OrderLatencyHigh"}, "InventoryCallFailures 等 2 条告警"}, // 系统分组可能包含多个症状。
		{"乱序", []string{"OrderLatencyHigh", "InventoryCallFailures"}, "InventoryCallFailures 等 2 条告警"}, // 重发时顺序不稳定。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			value := Notification{}
			for _, name := range testcase.names {
				value.Alerts = append(value.Alerts, Alert{Labels: map[string]string{"alertname": name}})
			}
			if got := Title(value); got != testcase.want {
				t.Fatalf("title=%s", got)
			}
		})
	}
}

// TestTitleBound 给数量后缀留空间，长告警名称也不能导致保存通知失败。
func TestTitleBound(t *testing.T) {
	value := Notification{Alerts: []Alert{{Labels: map[string]string{"alertname": strings.Repeat("长", 300)}}, {Labels: map[string]string{"alertname": strings.Repeat("长", 300)}}}}
	if title := Title(value); len([]rune(title)) > 256 || !strings.HasSuffix(title, "等 2 条告警") {
		t.Fatal("标题超长或数量丢失")
	}
}
