// 本文件：在命令、轮次和槽位开始时登记评测器实际使用的代码版本。续跑追加当时的记录，不回填旧报告。

package runner

import (
	"context"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// recordEvaluator 发生在动作前；写审计失败时不继续注入或调用付费模型。
func recordEvaluator(ctx context.Context, options command, repository *store.Store, operation, key string) error {
	versions, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return err
	}
	return repository.RecordEvalExecution(ctx, store.EvalExecution{
		RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration,
		Operation: operation, ResultKey: key, Versions: versions,
	})
}
