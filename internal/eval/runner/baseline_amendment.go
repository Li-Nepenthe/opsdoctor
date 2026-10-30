// 本文件：把用户批准的流量守卫修正接到原批次，并审计尚未调用模型的轮次重启。
//
// 主要内容：baselinePriorHash 证明只改了获准文件；amendBaseline 核对后保存授权；
// sameBaselineBehavior 确认手册与模型安排等独立清单字段也没有变化。
// 阅读建议：这个命令不调用模型。批准必须是宿主机操作者显式执行，医生没有这个入口。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// baselineChanges 只包括基线检查、执行版本和重启审计；模型提示、工具、手册、场景不在其中。
var baselineChanges = map[string]bool{
	"internal/eval/runner/environment.go":                                false,
	"internal/eval/runner/load.go":                                       true,
	"internal/eval/runner/execution.go":                                  true,
	"internal/eval/runner/baseline_amendment.go":                         true,
	"internal/eval/runner/paired.go":                                     false,
	"internal/eval/runner/automatic.go":                                  false,
	"internal/eval/runner/cli.go":                                        false,
	"internal/eval/runner/calibration.go":                                false,
	"internal/eval/runner/doc.go":                                        false,
	"internal/doctor/store/eval_clock_amendment.go":                      false,
	"internal/doctor/store/eval_baseline_amendment.go":                   true,
	"internal/doctor/store/eval_execution.go":                            true,
	"internal/doctor/store/doc.go":                                       false,
	"internal/doctor/store/migrations/0041_eval_baseline_amendments.sql": true,
	"internal/doctor/store/migrations/0042_eval_execution_audits.sql":    true,
}

// baselinePriorHash 用前一提交的字节核对完整旧哈希，不修改工作区里的任何文件。
func baselinePriorHash(ctx context.Context, commit string) (string, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) {
		return "", fmt.Errorf("原提交号不合法")
	}
	previous := map[string][]byte{}
	// 白名单以外仍读取工作区现值，因此夹带改提示或场景会使完整旧哈希不再匹配。
	for path, added := range baselineChanges {
		if added {
			previous[path] = nil
			continue
		}
		data, err := exec.CommandContext(ctx, "git", "show", commit+":"+path).Output()
		if err != nil {
			return "", fmt.Errorf("无法读取原基线代码 %s", path)
		}
		previous[path] = data
	}
	return sourceHashUsing(previous)
}

// amendBaseline 先证实原故障已经恢复；不能在400QPS还运行时把轮次退回准备状态。
func amendBaseline(ctx context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	if options.interruption < 1 || options.scenario == "" {
		return fmt.Errorf("请指定 --interruption、--scenario 和 --iteration")
	}
	if _, err := os.Stat(".opsdoctor-chaos.json"); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("仍有故障恢复文件或无法确认，请先完成原场景恢复")
	}
	if err := waitBaseline(ctx, options, reader, client, output); err != nil {
		return err
	}
	saved, err := repository.EvalRun(ctx, options.runID)
	if err != nil {
		return err
	}
	var plan pairedManifest
	if json.Unmarshal(saved.Config, &plan) != nil || plan.Kind != "paired" {
		return fmt.Errorf("不是冻结配对批次")
	}
	// 第一次申请以校时修订后的版本为父版本；重试则仍取已保存的父版本。
	previous, err := repository.EvalBaselineParent(ctx, options.runID, plan.Versions)
	if err != nil {
		return err
	}
	var old struct {
		Commit string   `json:"commit"`
		Hash   string   `json:"source_hash"`
		Cases  []string `json:"cases"`
		Model  string   `json:"model"`
	}
	if json.Unmarshal(previous, &old) != nil {
		return fmt.Errorf("原版本格式无效")
	}
	prior, err := baselinePriorHash(ctx, old.Commit)
	if err != nil {
		return err
	}
	if prior != old.Hash {
		return fmt.Errorf("除了获准的流量检查与审计代码还有其他变化，拒绝续接")
	}
	versions, err := manifest(ctx, old.Model, old.Cases)
	if err != nil {
		return err
	}
	// 手册目录单独计算哈希；不能只检查源码，就漏掉没有进入源码目录的手册改动。
	if !sameBaselineBehavior(previous, versions) {
		return fmt.Errorf("提示、手册或模型安排改变，拒绝基础设施修订")
	}
	trial := store.EvalTrial{RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration}
	if err = repository.AmendEvalBaseline(ctx, trial, options.interruption, options.reason, previous, versions); err != nil {
		return err
	}
	fmt.Fprintln(output, "已保存基线修订和原中断轮次；随机顺序与旧结果不变，无模型槽位的轮次可重新注入。")
	return nil
}

// sameBaselineBehavior 只忽略本次明确获准变化的代码提交与哈希，其他清单字段全部相等。
func sameBaselineBehavior(before, after json.RawMessage) bool {
	var left, right map[string]any
	if json.Unmarshal(before, &left) != nil || json.Unmarshal(after, &right) != nil {
		return false
	}
	for _, key := range []string{"commit", "source_hash"} {
		delete(left, key)
		delete(right, key)
	}
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return equalJSON(l, r)
}
