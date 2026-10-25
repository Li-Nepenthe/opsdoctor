// 本文件：加载并检查场景文件，所有检查通过后才允许开始操作。
//
// 文件中的动作会改动靶场，所以不能把拼写错误当成默认值继续执行。
// 文件大小、编号和动作范围都在这里收紧。
//
// 主要内容：
//   - Load：按固定顺序读取阶段 4 的十一个场景。
//   - Decode：拒绝未知字段，并检查必需的实验信息。
//   - ValidateStep：检查每种动作自己的参数范围。
//
// 阅读建议：先读 Decode，再按感兴趣的动作查看 validate.go。

package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Load 只读取当前阶段明确支持的文件，不把目录中新放入的任意文件自动变成可执行场景。
func Load(directory string) ([]Scenario, error) {
	// 编号列表属于当前阶段的授权范围，S11/S12 不因目录中出现同名文件就启用。
	var result []Scenario
	for _, id := range []string{"S01", "S02", "S03", "S04", "S05", "S06", "S07", "S08", "S09", "S10", "S13"} {
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
	const maxBytes = 64 * 1024 // 十几个动作和讲解足够放下，拒绝误把大型文件当作配置。
	var value Scenario
	if len(data) > maxBytes {
		return value, fmt.Errorf("场景文件超过 64 KiB")
	}
	if err := yaml.UnmarshalWithOptions(data, &value, yaml.Strict()); err != nil {
		return value, err
	}
	allowed := " S01 S02 S03 S04 S05 S06 S07 S08 S09 S10 S13 "
	if value.ID == "" || !strings.Contains(allowed, " "+value.ID+" ") || value.Title == "" {
		return Scenario{}, fmt.Errorf("场景编号或标题不合法")
	}
	within, err := time.ParseDuration(value.Symptom.Within)
	if err != nil || within <= 0 || within > 3*time.Minute || value.Symptom.PromQL == "" || len(value.Symptom.Alerts) == 0 {
		return Scenario{}, fmt.Errorf("症状需要查询表达式与不超过三分钟的正等待时长")
	}
	if len(value.Inject) == 0 || len(value.Restore) == 0 || len(value.Evidence) == 0 || value.GroundTruth.Component == "" || value.GroundTruth.Mechanism == "" || value.GroundTruth.Explanation == "" {
		return Scenario{}, fmt.Errorf("注入、恢复、答案和证据里程碑都不能为空")
	}
	for _, step := range append(append([]Step{}, value.Inject...), value.Restore...) {
		if err := ValidateStep(step); err != nil {
			return Scenario{}, err
		}
	}
	return value, nil
}
