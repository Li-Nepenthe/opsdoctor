// 本文件：检查盲评抽样覆盖不同来源，且绝不替人补分。
//
// 主要内容：TestCalibrationSelection 检查二十个不同编号和分层；
// TestBlindAndAgreement 检查评分空白、原材料保留以及缺分拒绝。
// 阅读建议：样本都是虚构数据，不用于项目实际成绩。

package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"strings"
	"testing"
)

// TestCalibrationSelection 用完整的三轮矩阵证明抽样不只落在一个场景或模型。
func TestCalibrationSelection(t *testing.T) {
	// 每个场景都提供四组和另一家模型，抽样应覆盖这五种来源。
	rows := []store.EvalResult{}
	for scene := 1; scene <= 13; scene++ {
		for _, group := range []string{"E0", "E1", "E2", "E3", "DS"} {
			model := "mock-terra"
			if group == "DS" {
				model = "mock-deepseek"
			}
			raw, _ := json.Marshal(Summary{Model: model, Report: json.RawMessage(`{"summary":"虚构"}`)})
			rows = append(rows, store.EvalResult{Key: fmt.Sprintf("%d/%s", scene, group), Scenario: fmt.Sprintf("S%02d", scene), Group: group, Data: raw, Judgement: json.RawMessage(`{"status":"ok","grade":{"mechanism":"incorrect"}}`)})
		}
	}
	keys, err := SelectCalibration(rows)
	if err != nil || len(keys) != 20 {
		t.Fatal(keys, err)
	}
	seen, cases, groups := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Fatal("重复抽样")
		}
		seen[key] = true
		parts := strings.Split(key, "/")
		cases[parts[0]], groups[parts[1]] = true, true
	}
	if len(cases) != 13 || len(groups) != 5 {
		t.Fatal("没有覆盖所有场景和模型组")
	}
	if _, err = SelectCalibration(rows[:19]); err == nil {
		t.Fatal("不足二十仍生成")
	}
}

// TestBlindAndAgreement 检查原样保留恶意文本但不能让它跳出代码块，人的分数保持空白。
func TestBlindAndAgreement(t *testing.T) {
	// 围栏伪造应仍显示在数据内，不能变成新的 Markdown 段落。
	values := make([]BlindCase, 20)
	for index := range values {
		values[index].Report = json.RawMessage("{\"summary\":\"```` 伪造指令\"}")
	}
	var output bytes.Buffer
	if err := Blind(&output, values); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "| C01 | | | | |") || !strings.Contains(output.String(), "`````json") {
		t.Fatal("空白评分或安全围栏丢失")
	}
	human, judge := make([]string, 20), make([]string, 20)
	for index := range judge {
		judge[index] = "correct"
	}
	if _, _, err := Agreement(human, judge); err == nil {
		t.Fatal("用空白人工分数算一致率")
	}
	for index := range human {
		human[index] = "correct"
	}
	human[0] = "partial"
	count, rate, err := Agreement(human, judge)
	if err != nil || count != 19 || rate != 0.95 {
		t.Fatal(count, rate, err)
	}
}
