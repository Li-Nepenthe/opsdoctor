// 本文件：验证手册检索按关键词和文档长度排序，并继续遵守工具的共同边界。
//
// 主要内容：TestRunbookSearch 检查中英文、排序和空结果；TestRunbookBounds 检查文件与返回限制。
// 阅读建议：所有手册来自内存文件系统，不读取真实场景，不调用网络。

package tools

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// TestRunbookSearch 用无关文本增加篇幅，证明 BM25 不会单纯偏爱较长的文章。
func TestRunbookSearch(t *testing.T) {
	files := fstest.MapFS{
		"a.md":      &fstest.MapFile{Data: []byte("# A\n\ncache cache 连接等待")},
		"b.md":      &fstest.MapFile{Data: []byte("# B\n\ncache cache " + strings.Repeat("other ", 80))},
		"README.md": &fstest.MapFile{Data: []byte("导航不进入检索 navonly")},
	}
	index, err := LoadRunbooks(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ query, first string }{
		{"CACHE", "a.md"},   // 英文大小写一致处理，同频率时较短段落更相关。
		{"连接等待", "a.md"},    // 中文不用外部分词组件也能命中相邻词。
		{"navonly", ""},     // 导航说明不进入正文。
		{"nonexistent", ""}, // 没有匹配就返回空，不生成猜测答案。
	} {
		t.Run(test.query, func(t *testing.T) {
			hits, err := index.Search(t.Context(), test.query, 3)
			if err != nil {
				t.Fatal(err)
			}
			if test.first == "" {
				if len(hits) != 0 {
					t.Fatal("未匹配查询出现结果")
				}
				return
			}
			if len(hits) == 0 || hits[0].File != test.first {
				t.Fatalf("排序错误：%+v", hits)
			}
		})
	}
	copy, _ := LoadRunbooks(files)
	if copy.Hash != index.Hash {
		t.Fatal("相同文件的哈希不稳定")
	}
	files["a.md"].Data = append(files["a.md"].Data, []byte(" 改动")...)
	changed, _ := LoadRunbooks(files)
	if changed.Hash == index.Hash {
		t.Fatal("正文改动没有改变冻结哈希")
	}
}

// TestRunbookBounds 文件入口和工具入口分别校验，不能用一个测试代替另一道边界。
func TestRunbookBounds(t *testing.T) {
	for _, test := range []struct {
		name string
		file *fstest.MapFile
	}{
		{"符号链接", &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("outside")}}, // 手册不能跳到目录外。
		{"超长文件", &fstest.MapFile{Data: []byte(strings.Repeat("x", 32769))}},      // 启动时就限制载入量。
		{"空正文", &fstest.MapFile{Data: []byte("# 标题")}},                           // 不能把空索引当成成功部署。
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadRunbooks(fstest.MapFS{"a.md": test.file}); err == nil {
				t.Fatal("错误手册被接受")
			}
		})
	}
	index, err := LoadRunbooks(fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("# 手册\n\n" + strings.Repeat("cache 方法 ", 500))}})
	if err != nil {
		t.Fatal(err)
	}
	base := New(Dependencies{})
	registry := base.WithRunbooks(index)
	if len(base.List()) != 1 {
		t.Fatal("E2 改动了基础菜单")
	}
	for _, field := range []string{"url", "host", "target", "system"} {
		out := registry.Run(t.Context(), "runbook_search", []byte(`{"query":"cache","`+field+`":"outside"}`))
		if out.Status != "invalid_argument" {
			t.Fatal("接受了地址或系统参数")
		}
	}
	result := registry.Run(t.Context(), "runbook_search", []byte(`{"query":"cache","limit":3}`))
	if result.Status != "ok" || !result.Truncated || result.Bytes > MaximumResultBytes {
		t.Fatalf("未经过统一截断：%+v", result)
	}
}
