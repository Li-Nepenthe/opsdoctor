// 本文件：把用户批准的校时守卫调整作为独立审计接到原冻结批次。
//
// 主要内容：amendClock 校验原批次并记录授权；clockPriorHash 重建允许变更前的哈希。
// 阅读建议：这个入口只接受列出的环境守卫与审计代码变化。提示、工具、手册、评分
// 或场景变了，仍必须全量重跑，不能用一次校时授权把这些变化混进旧结果。

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// clockChanges 是本次基础设施修订的完整边界；测试和导航本来就不参与行为哈希。
var clockChanges = map[string]bool{
	"internal/platform/clockcheck/doc.go":                             false, // Go 目录说明也参与原始字节哈希。
	"internal/eval/runner/doc.go":                                     false,
	"internal/doctor/store/doc.go":                                    false,
	"internal/platform/clockcheck/sample.go":                          false,
	"internal/platform/clockcheck/policy.go":                          true,
	"internal/eval/runner/clock.go":                                   false,
	"internal/eval/runner/cli.go":                                     false,
	"internal/eval/runner/manifest.go":                                false,
	"internal/eval/runner/paired.go":                                  false,
	"internal/eval/runner/amendment.go":                               true,
	"internal/doctor/store/eval_reliability.go":                       false,
	"internal/doctor/store/eval_clock_amendment.go":                   true,
	"internal/doctor/store/migrations/0040_eval_clock_amendments.sql": true,
}

// clockPriorHash 将白名单文件暂按旧 Git 内容参与计算；不改磁盘，不还原用户的修改。
func clockPriorHash(ctx context.Context, commit, expected string) (string, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) {
		return "", fmt.Errorf("原提交号不合法")
	}
	previous := map[string][]byte{}
	// 只为已列出的文件读旧版本；其他文件仍取当前内容，任何额外变化都会使哈希不同。
	for path, added := range clockChanges {
		if added {
			previous[path] = nil
			continue
		}
		data, err := exec.CommandContext(ctx, "git", "show", commit+":"+path).Output()
		if err != nil {
			return "", fmt.Errorf("无法读取原校时代码 %s", path)
		}
		previous[path] = data
	}
	value, err := sourceHashUsing(previous)
	if err != nil || value == expected {
		return value, err
	}
	// 原冻结时这个目录说明仍是 Windows 换行，Git 中则为 LF。只还原该已核实
	// 差异，再与原完整哈希精确比较；不是忽略换行，也不扩大允许修改的文件集合。
	path := "internal/doctor/store/doc.go"
	previous[path] = bytes.ReplaceAll(previous[path], []byte("\n"), []byte("\r\n"))
	return sourceHashUsing(previous)
}

// amendClock 不创建模型任务，保留原清单、顺序和费用，再追加新的精确版本记录。
func amendClock(ctx context.Context, options command, repository *store.Store, output io.Writer) error {
	saved, err := repository.EvalRun(ctx, options.runID)
	if err != nil {
		return err
	}
	var plan pairedManifest
	if json.Unmarshal(saved.Config, &plan) != nil || plan.Kind != "paired" {
		return fmt.Errorf("不是冻结配对批次")
	}
	var old struct {
		Commit string   `json:"commit"`
		Hash   string   `json:"source_hash"`
		Cases  []string `json:"cases"`
		Model  string   `json:"model"`
	}
	if err = json.Unmarshal(plan.Versions, &old); err != nil {
		return err
	}
	// 先证实旧快照能被完整重建，才允许记录新提交号，不能只比较几个版本字符串。
	prior, err := clockPriorHash(ctx, old.Commit, old.Hash)
	if err != nil {
		return err
	}
	if prior != old.Hash {
		return fmt.Errorf("除了已批准的校时与审计文件，还有其他行为变化，拒绝续接原批次")
	}
	versions, err := manifest(ctx, old.Model, old.Cases)
	if err != nil {
		return err
	}
	if err = repository.AmendEvalClock(ctx, options.runID, options.makeupKey, options.reason, plan.Versions, versions); err != nil {
		return err
	}
	fmt.Fprintln(output, "已追加校时规则授权；原冻结清单不变，指定中断保留并允许一次新槽位补跑。")
	return nil
}
