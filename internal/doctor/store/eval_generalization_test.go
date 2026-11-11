// 本文件：场景编号与批次一起校验。OT 编号只对阶段 9 的独立批次有效，阶段 8 保持原有 S01–S13 的范围。

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
