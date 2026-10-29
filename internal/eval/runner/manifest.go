// 本文件：记录一次试跑实际使用的代码、提示和手册版本。
//
// Git 提交号看不到未提交的新文件。准备阶段还会计算源文件哈希；续跑前核对这份
// 哈希，避免把两个不同实现的结果放进同一批次。凭据和运行产生的文档不参与哈希。
//
// 主要内容：manifest 生成可审阅配置；sourceHash 按固定顺序读取行为相关文件。
// 阅读建议：这只是试跑版本记录，正式评测仍须在用户批准之后单独冻结。

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// manifest 不保存场景答案，只有本次允许试跑的名字和版本。
func manifest(ctx context.Context, model string, cases []string) (json.RawMessage, error) {
	code, err := sourceHash()
	if err != nil {
		return nil, err
	}
	books, err := tools.LoadRunbooks(os.DirFS("docs/runbooks"))
	if err != nil {
		return nil, err
	}
	commit, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("无法读取代码提交号")
	}
	return json.Marshal(map[string]any{"kind": "pilot", "model": model, "group": "E1", "cases": cases, "commit": strings.TrimSpace(string(commit)), "source_hash": code, "prompt_version": agent.PromptVersion, "runbooks_hash": books.Hash})
}

// sourceHash 明确列出代码与部署目录，不遍历 .env、个人笔记或原始验收输出。
func sourceHash() (string, error) {
	digest := sha256.New()
	for _, root := range []string{"cmd", "internal", "tools", "gen", "api", "deploy", "eval"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, problem error) error {
			if problem != nil {
				return problem
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, "system.md") && !strings.HasSuffix(path, "shop.profile.md") {
				return nil
			} // 测试和导航改动不改变运行行为。
			if !entry.Type().IsRegular() {
				return fmt.Errorf("版本清单拒绝非普通文件")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(digest, "%s\x00%d\x00", filepath.ToSlash(path), len(data))
			_, _ = digest.Write(data)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	for _, path := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%s\x00%d\x00", path, len(data))
		_, _ = digest.Write(data)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
