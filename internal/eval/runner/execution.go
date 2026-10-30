// 本文件：为评测命令、轮次和槽位留下实际执行的代码版本。
//
// 主要内容：recordEvaluator 计算当前清单并追加数据库审计。
// 阅读建议：旧报告不回填新版本；已有冻结清单和校时修订仍保留，后续每次续跑另记。

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
	return repository.RecordEvalExecution(ctx, options.runID, options.scenario, options.iteration, operation, key, versions)
}
