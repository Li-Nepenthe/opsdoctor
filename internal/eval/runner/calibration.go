// 本文件：从已完成批次取出固定的二十份盲评材料。
//
// 主要内容：calibrate 首次抽样并保存编号，随后只读原报告和工具观察。
// 阅读建议：这条命令不调用模型，也不填写人的分数；导出完成后要停下等用户。

package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/judge"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
)

// calibrate 抽样有数据库幂等记录，重新导出不能重新挑一套容易取得一致的报告。
func calibrate(ctx context.Context, repository *store.Store, id string, output io.Writer) error {
	run, err := repository.EvalRun(ctx, id)
	if err != nil {
		return err
	}
	if run.Status != "completed" {
		return fmt.Errorf("正式批次尚未完成，暂不生成盲评表")
	}
	// 参考原因来自冻结时的文件；版本改变后不能拿新答案评价旧结果。
	var frozen pairedManifest
	var versions struct {
		Source string `json:"source_hash"`
	}
	if json.Unmarshal(run.Config, &frozen) != nil || json.Unmarshal(frozen.Versions, &versions) != nil {
		return fmt.Errorf("缺少冻结版本")
	}
	current, err := sourceHash()
	if err != nil || current != versions.Source {
		return fmt.Errorf("当前场景或代码与冻结版本不同，不能生成混用版本的盲评表")
	}
	rows, err := repository.EvalResults(ctx, id)
	if err != nil {
		return err
	}
	keys, err := repository.EvalCalibration(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		keys, err = report.SelectCalibration(rows)
		if err != nil {
			return err
		}
		keys, err = repository.SaveEvalCalibration(ctx, id, keys)
	}
	if err != nil {
		return err
	}
	scenarios, err := scenario.Load("eval/scenarios")
	if err != nil {
		return err
	}
	cases := map[string]scenario.Scenario{}
	for _, value := range scenarios {
		cases[value.ID] = value
	}
	byKey := map[string]store.EvalResult{}
	for _, row := range rows {
		byKey[row.Key] = row
	}
	values := []report.BlindCase{}
	// 只按已保存编号顺序输出。评委的 judgement 不进入 BlindCase。
	for _, key := range keys {
		row, ok := byKey[key]
		if !ok {
			return fmt.Errorf("抽样编号找不到原结果")
		}
		value, ok := cases[row.Scenario]
		if !ok {
			return fmt.Errorf("缺少原场景参考")
		}
		detail, err := repository.Detail(ctx, row.DiagnosisID)
		if err != nil {
			return err
		}
		values = append(values, report.BlindCase{Reference: value.GroundTruth, Milestones: value.Evidence, Report: detail.Report, Symptoms: detail.Input, Observations: judge.Observations(detail.Steps)})
	}
	return report.Blind(output, values)
}
