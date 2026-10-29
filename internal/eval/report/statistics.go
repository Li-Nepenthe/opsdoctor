// 本文件：按场景、实验组和模型分别汇总少量重复运行。
//
// 三次实验不足以代表生产系统。这里保留样本数、平均值和范围；基础设施失败
// 与中途换模型单列，不能在平均值里把它们悄悄算成成功或免费的运行。
//
// 主要内容：Statistics 产生分层统计；measurement 只收集实际存在的数值；
// judgementView 读取已保存评分，不在导出时再次调用评委。
// 阅读建议：机制没有通过评分校验时记为缺失，不能拿零分填补评委失败。

package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// measurement 的样本数随有效观测增长，未知时长和未知评分不塞进均值。
type measurement struct {
	n             int
	sum, min, max float64
}

// add 接收一项已经确认存在的观测，第一项同时初始化上下界。
func (value *measurement) add(number float64) {
	if value.n == 0 || number < value.min {
		value.min = number
	}
	if value.n == 0 || number > value.max {
		value.max = number
	}
	value.n++
	value.sum += number
}

// text 同时展示平均值、波动范围和 n，避免把一份有效报告说成三次结果。
func (value measurement) text() string {
	if value.n == 0 {
		return "未测量"
	}
	return fmt.Sprintf("%.3f [%.3f, %.3f] n=%d", value.sum/float64(value.n), value.min, value.max, value.n)
}

// judgementView 只读取需要汇总的字段，原完整评分仍保存在数据库和明细里。
type judgementView struct {
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	Coverage   float64 `json:"coverage"`
	CostMicros int64   `json:"cost_micros"`
	Grade      struct {
		Mechanism       string `json:"mechanism"`
		Reason          string `json:"reason"`
		UnrelatedAction bool   `json:"unrelated_action"`
	} `json:"grade"`
}

// statisticsRow 的医生费用只有一种币种；评委人民币另列，不做总金额相加。
type statisticsRow struct {
	scenario, group, model, currency                             string
	reliability, mixed, incomplete                               int
	component, mechanism, coverage, seconds, tools, rounds, cost measurement
	allCost, judgeCost                                           int64
}

// Statistics 分层后再输出，不把十三种不同问题混成一个笼统准确率。
func Statistics(output io.Writer, rows []store.EvalResult) error {
	grouped := map[string]*statisticsRow{}
	for _, row := range rows {
		var value Summary
		if len(row.Data) > 0 && json.Unmarshal(row.Data, &value) != nil {
			return fmt.Errorf("统计结果无法解码")
		}
		model := value.Model
		if model == "" {
			model = row.ExecutionProfile + "（尚未绑定）"
		}
		key := row.Scenario + "/" + row.Group + "/" + model
		entry := grouped[key]
		if entry == nil {
			entry = &statisticsRow{scenario: row.Scenario, group: row.Group, model: model, currency: value.Currency}
			grouped[key] = entry
		}
		entry.allCost += value.CostMicros
		var grade judgementView
		if len(row.Judgement) > 0 && json.Unmarshal(row.Judgement, &grade) != nil {
			return fmt.Errorf("统计评分无法解码")
		}
		entry.judgeCost += grade.CostMicros
		// 费用保留在上方；下面的排除只影响诊断质量和耗时样本，不让失败的钱消失。
		if len(row.Data) == 0 {
			entry.incomplete++
			continue
		}
		if value.ReliabilityFailure {
			entry.reliability++
			continue
		}
		if value.MixedModel {
			entry.mixed++
			continue
		}
		match := 0.0
		if value.ComponentMatch {
			match = 1
		}
		entry.component.add(match)
		entry.rounds.add(float64(value.Rounds))
		entry.tools.add(float64(value.Tools))
		entry.cost.add(float64(value.CostMicros) / 1e6)
		if value.DurationKnown {
			entry.seconds.add(value.Seconds)
		}
		if grade.Status == "ok" {
			score, ok := map[string]float64{"correct": 1, "partial": 0.5, "incorrect": 0}[grade.Grade.Mechanism]
			if !ok {
				return fmt.Errorf("保存的机制评分不合法")
			}
			entry.mechanism.add(score)
			entry.coverage.add(grade.Coverage)
		}
	}
	fmt.Fprintln(output, "\n## 分层结果\n\n每格为平均值 [最小, 最大] n=有效样本数。组件一致记 1，否则 0；机制正确/部分/错误记 1/0.5/0。基础设施失败和混合模型单列；评分失败不填零。三次样本很小，以下不是生产准确率。\n\n| 场景 | 组 | 模型 | 组件一致 | 机制 | 证据覆盖 | 轮数 | 工具数 | 秒 | 医生单次账本 | 全部医生费用（含排除项） | 评委费用 | 可靠性 / 混合 / 未完成 |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	keys := []string{}
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := grouped[key]
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s %s | %s %.6f | CNY %.6f | %d / %d / %d |\n", cell(value.scenario), cell(value.group), cell(value.model), value.component.text(), value.mechanism.text(), value.coverage.text(), value.rounds.text(), value.tools.text(), value.seconds.text(), cell(value.currency), value.cost.text(), cell(value.currency), float64(value.allCost)/1e6, float64(value.judgeCost)/1e6, value.reliability, value.mixed, value.incomplete); err != nil {
			return err
		}
	}
	return nil
}
