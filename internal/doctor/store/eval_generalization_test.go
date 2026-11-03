// 本文件：验证第二系统编号只在阶段 9 的独立批次中有效。
//
// 主要内容：TestGeneralizationTrialIdentity 保留原 S01–S13 的验证行为，
// 同时防止把 OT 编号写进阶段 8 原批次。测试只调用纯校验函数，不连接数据库。
// 阅读建议：批次和场景要一起看，不能只给正则增加一个宽泛前缀。

package store

import "testing"

// TestGeneralizationTrialIdentity 旧阶段的合法和非法输入保持原样，新增范围严格受限。
func TestGeneralizationTrialIdentity(t *testing.T) {
	// 同一个 OT 编号放进旧批次应失败，只有明确的阶段 9 命名才通过。
	for _, test := range []struct {
		batch, scenario string
		iteration       int
		valid           bool
	}{
		{"phase08-formal", "S01", 1, true},
		{"phase08-formal", "S13", 3, true},
		{"phase08-formal", "OT01", 1, false},
		{"phase09-formal", "OT01", 1, true},
		{"phase09-formal", "OT08", 3, true},
		{"phase09-formal", "OT09", 1, false},
		{"phase09-formal", "OT01", 0, false},
		{"phase09-formal", "OT01", 4, false},
		{"another", "OT01", 1, false},
	} {
		if validEvalTrial(test.batch, test.scenario, test.iteration) != test.valid {
			t.Fatalf("batch=%s scenario=%s iteration=%d", test.batch, test.scenario, test.iteration)
		}
	}
}
