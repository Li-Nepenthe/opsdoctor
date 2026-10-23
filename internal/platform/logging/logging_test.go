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
	for name, want := range map[string]string{"service": "test-service", "level": "INFO", "msg": "读取完成", "request_id": "request-a", "trace_id": "trace-a", "span_id": "span-a", "operation": "test"} {
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
