// 本文件：场景说明通过完整性检查后才交给宿主机运行器。
// Load 只读取固定编号，Decode 对文件大小、字段和动作逐项校验。

package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Load 只读取已登记的文件。
func Load(directory string) ([]Scenario, error) {
	// 目录中新放入的文件不能自动成为可执行场景。
	var result []Scenario
	for _, id := range []string{"S01", "S02", "S03", "S04", "S05", "S06", "S07", "S08", "S09", "S10", "S11", "S12", "S13"} {
		data, err := os.ReadFile(filepath.Join(directory, id+".yaml"))
		if err != nil {
			return nil, fmt.Errorf("读取 %s：%w", id, err)
		}
		value, err := Decode(data)
		if err != nil {
			return nil, fmt.Errorf("检查 %s：%w", id, err)
		}
		if value.ID != id {
			return nil, fmt.Errorf("场景编号与文件名不一致：%s", id)
		}
		result = append(result, value)
	}
	return result, nil
}

// Decode 先检查内容，再逐个检查动作。失败时不返回半份可以误用的场景。
func Decode(data []byte) (Scenario, error) {
	const maxBytes = 64 * 1024 // 限制单份场景文档的大小。
	var value Scenario
	if len(data) > maxBytes {
		return value, fmt.Errorf("场景文件超过 64 KiB")
	}
	if err := yaml.UnmarshalWithOptions(data, &value, yaml.Strict()); err != nil {
		return value, err
	}
	allowed := " S01 S02 S03 S04 S05 S06 S07 S08 S09 S10 S11 S12 S13 "
	if value.ID == "" || !strings.Contains(allowed, " "+value.ID+" ") || value.Title == "" {
		return Scenario{}, fmt.Errorf("场景编号或标题不合法")
	}
	within, err := time.ParseDuration(value.Symptom.Within)
	if err != nil || within <= 0 || within > 3*time.Minute || (!value.Symptom.Healthy && (value.Symptom.PromQL == "" || len(value.Symptom.Alerts) == 0)) {
		return Scenario{}, fmt.Errorf("症状需要查询表达式与不超过三分钟的正等待时长")
	}
	if value.Symptom.Healthy && (value.ID != "S11" || value.GroundTruth.Component != "none" || len(value.Symptom.Alerts) != 0 || value.Symptom.PromQL != "") {
		return Scenario{}, fmt.Errorf("只有 S11 可使用无告警健康对照")
	}
	if len(value.Inject) == 0 || len(value.Restore) == 0 || len(value.Evidence) == 0 || value.GroundTruth.Component == "" || value.GroundTruth.Mechanism == "" || value.GroundTruth.Explanation == "" {
		return Scenario{}, fmt.Errorf("注入、恢复、答案和证据里程碑都不能为空")
	}
	// 评分规则单独校验，不能因为编号写错就把另一项证据算成已取得。
	seenRules := map[int]bool{}
	for _, rule := range value.Rules {
		if rule.Milestone < 1 || rule.Milestone > len(value.Evidence) || seenRules[rule.Milestone] || (rule.Table != "stock" && rule.Table != "orders") || (rule.Kind != "full_scan" && rule.Kind != "missing_index") || (rule.Kind == "missing_index" && rule.Index == "") {
			return Scenario{}, fmt.Errorf("证据规则类型、对象或项目编号不合法")
		}
		seenRules[rule.Milestone] = true
	}
	for _, step := range append(append([]Step{}, value.Inject...), value.Restore...) {
		if err := ValidateStep(step); err != nil {
			return Scenario{}, err
		}
	}
	return value, nil
}
