// 本文件：检查日志字段、UTC 时间和分组之后的关联信息。
//
// 日志写进内存后按 JSON 解析，避免只凭字符串看起来相似就判通过。
// 先看 TestJSONFields，再看 TestGroupedFieldsStayCorrelated 如何防止 request_id 被藏到业务分组里。

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// TestJSONFields 检查日志格式、UTC 时间和 context 关联信息，避免跨服务排查时无法串联请求。
func TestJSONFields(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, "test-service", slog.LevelInfo).With("operation", "test")
	ctx := WithFields(context.Background(), Fields{RequestID: "request-a", TraceID: "trace-a", SpanID: "span-a"})
	logger.DebugContext(ctx, "不应输出")
	logger.InfoContext(ctx, "读取完成")
	// 解码实际输出而非只比较文本，确保每个字段都是有效 JSON 的一部分。
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"service":    "test-service", // 构造时设置的服务名应固定出现在每条日志中。
		"level":      "INFO",         // DEBUG 被过滤后，只应留下 INFO 这一条。
		"msg":        "读取完成",         // 日志正文应与调用方提供的消息一致。
		"request_id": "request-a",    // 请求标识应从当前 context 补入。
		"trace_id":   "trace-a",      // trace 标识用来串联同一调用链。
		"span_id":    "span-a",       // span 标识用来定位调用链中的当前操作。
		"operation":  "test",         // With 添加的固定业务字段不能在补标识时丢失。
	} {
		if record[name] != want {
			t.Errorf("%s = %v，期望 %s", name, record[name], want)
		}
	}
	if !strings.HasSuffix(record["time"].(string), "Z") {
		t.Fatal("时间不是 UTC")
	}
}

// TestGroupedFieldsStayCorrelated 确保业务字段分组后关联标识仍在顶层，日志查询不需要猜分组路径。
func TestGroupedFieldsStayCorrelated(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, "test", slog.LevelInfo).WithGroup("business").With("fixed", "value")
	ctx := WithFields(context.Background(), Fields{RequestID: "request-group"})
	logger.InfoContext(ctx, "分组日志", "dynamic", "value")
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	// 固定与动态属性都属于业务组，但服务名与请求标识必须能直接查询。
	if record["service"] != "test" || record["request_id"] != "request-group" {
		t.Fatal(record)
	}
	group, ok := record["business"].(map[string]any)
	if !ok || group["dynamic"] != "value" || group["fixed"] != "value" {
		t.Fatal(record)
	}
}
