// 本文件：抽取二十份不同层次的报告，生成人填写的盲评材料。
//
// 抽样会看场景、实验组和模型，防止只抽到容易的一类。展示时去掉模型品牌、
// 实验组和评委分数，只留下原报告、实际观察和参考材料。
//
// 主要内容：SelectCalibration 先打乱候选再覆盖各层；Blind 输出空白评分表；
// Agreement 只比较调用者明确提供的人类分数，不生成这些分数。
// 阅读建议：实际抽样编号另存数据库，重新导出仍是相同二十份。

package report

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// candidate 只参与抽样；这些分层标签不会展示在盲评表里。
type candidate struct{ key, scenario, stratum, mechanism string }

// SelectCalibration 保留所有候选，不按“答得好”排序；各层中先随机再选择。
func SelectCalibration(rows []store.EvalResult) ([]string, error) {
	// 只有存在报告且评委成功给分的记录才能比较一致率；缺失评分另列为失败。
	candidates := []candidate{}
	for _, row := range rows {
		var value Summary
		var grade judgementView
		if len(row.Data) == 0 || len(row.Judgement) == 0 {
			continue
		}
		if json.Unmarshal(row.Data, &value) != nil || json.Unmarshal(row.Judgement, &grade) != nil {
			return nil, fmt.Errorf("抽样数据无法解码")
		}
		if value.MixedModel || value.ReliabilityFailure || len(value.Report) == 0 || string(value.Report) == "null" || grade.Status != "ok" {
			continue
		}
		candidates = append(candidates, candidate{row.Key, row.Scenario, row.Group + "/" + value.Model, grade.Grade.Mechanism})
	}
	if len(candidates) < 20 {
		return nil, fmt.Errorf("可校准报告不足二十份，不能补造或重复抽样")
	}
	for index := len(candidates) - 1; index > 0; index-- {
		chosen, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return nil, err
		}
		candidates[index], candidates[chosen.Int64()] = candidates[chosen.Int64()], candidates[index]
	}
	selected := []string{}
	cases, groups, scores := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for len(selected) < 20 {
		best, weight := 0, -1
		for index, item := range candidates {
			score := 0
			if !cases[item.scenario] {
				score += 100
			}
			if !groups[item.stratum] {
				score += 30
			}
			if !scores[item.mechanism] {
				score += 5
			}
			if score > weight {
				best, weight = index, score
			}
		}
		item := candidates[best]
		selected = append(selected, item.key)
		cases[item.scenario], groups[item.stratum], scores[item.mechanism] = true, true, true
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return selected, nil
}

// BlindCase 的数据来自原诊断和通用评分参考，没有评委分数字段。
type BlindCase struct {
	Reference    any             `json:"参考原因"`
	Milestones   []string        `json:"待核对的证据项目"`
	Report       json.RawMessage `json:"原报告"`
	Symptoms     json.RawMessage `json:"初始输入"`
	Observations any             `json:"实际观察"`
}

// Blind 只生成空表。工具文字用足够长的代码围栏包住，不能伪造文档小标题。
func Blind(output io.Writer, values []BlindCase) error {
	if len(values) != 20 {
		return fmt.Errorf("盲评表必须有二十份真实报告")
	}
	fmt.Fprintln(output, "# 人工校准评分表\n\n请本人填写，不需要参考评委分数。机制填写 correct / partial / incorrect：关键因果完整且无相反结论、只说对部分、主因错误或只有症状复述。证据按每项实际观察判断，填达成的项目编号；不要把计划查询算作查到了。\n\n| 编号 | 机制（本人填） | 已达成证据编号（本人填） | 不相关写建议（本人填） | 理由 / 评委可能判断错的地方（本人填） |\n|---|---|---|---|---|")
	for index := range values {
		fmt.Fprintf(output, "| C%02d | | | | |\n", index+1)
	}
	// 不输出候选的模型、组或评委分数；JSON 只包含人做判断所需的原材料。
	for index, value := range values {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		fence := "````"
		for strings.Contains(string(raw), fence) {
			fence += "`"
		}
		if _, err = fmt.Fprintf(output, "\n## C%02d\n\n%sjson\n%s\n%s\n", index+1, fence, raw, fence); err != nil {
			return err
		}
	}
	return nil
}

// Agreement 缺一份人类分数都返回错误，不用评委分数补空白。
func Agreement(human, judge []string) (int, float64, error) {
	if len(human) != 20 || len(judge) != 20 {
		return 0, 0, fmt.Errorf("需要二十份人工分数与原评分")
	}
	matches := 0
	valid := map[string]bool{"correct": true, "partial": true, "incorrect": true}
	for index, score := range human {
		if !valid[score] || !valid[judge[index]] {
			return 0, 0, fmt.Errorf("第 %d 份缺少合法分数", index+1)
		}
		if score == judge[index] {
			matches++
		}
	}
	return matches, float64(matches) / 20, nil
}
