// 本文件：证明校时例外不会顺带接受提示、工具或场景的改动。
//
// 主要内容：TestAmendmentHash 用临时目录模拟原快照、允许修订和不允许修订。
// 阅读建议：只替换内存中参与哈希的内容，用户工作区不会被还原。

package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAmendmentHash 核对恢复白名单快照后，其他一字节变化仍然可见。
func TestAmendmentHash(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, root := range []string{"cmd", "internal", "tools", "gen", "api", "deploy", "eval"} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{"go.mod": "module test", "go.sum": "", "internal/clock.go": "old clock", "internal/system.md": "frozen prompt"} {
		if err := os.WriteFile(filepath.FromSlash(path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := sourceHash()
	if err != nil {
		t.Fatal(err)
	}
	// 环境守卫及其审计允许变化；随后单独改提示，证明额外变化仍会被发现。
	if err = os.WriteFile("internal/clock.go", []byte("new clock"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("internal/new.go", []byte("audit"), 0600); err != nil {
		t.Fatal(err)
	}
	overrides := map[string][]byte{"internal/clock.go": []byte("old clock"), "internal/new.go": nil}
	restored, err := sourceHashUsing(overrides)
	if err != nil || restored != before {
		t.Fatal("允许修订无法核对旧快照", err)
	}
	if err = os.WriteFile("internal/system.md", []byte("changed prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err = sourceHashUsing(overrides)
	if err != nil || restored == before {
		t.Fatal("提示变化被校时例外掩盖", err)
	}
}
