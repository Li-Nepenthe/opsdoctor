// 本文件：验证第二系统的导出不会继续被标成自建靶场。
//
// 主要内容：TestMarkdownTarget 检查同一个导出器如何保持系统归属和原费用口径。
// 阅读建议：测试使用空结果；数字只从调用方给出的真实批次对象读取。

package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// TestMarkdownTarget 外部系统明确显示其名字，同时保留美元用量账本的说明。
func TestMarkdownTarget(t *testing.T) {
	var output bytes.Buffer
	if err := Markdown(&output, store.EvalRun{ID: "phase09-test", Target: "otel-demo", TokenLimit: 100}, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "接入系统 otel-demo") || strings.Contains(text, "来自自建靶场") || !strings.Contains(text, "按项目用量账本计") {
		t.Fatal("导出丢失系统归属或费用口径")
	}
}
