// 本文件：检查待核对日志与指标的实际内容。
//
// 测试直接读取独立注册表，不启动 HTTP，也不连接监控系统。
//
// 主要内容：
//   - TestReviewRecorder：核对必需日志字段、累计值和有限的指标标签。
//
// 阅读建议：计数器只有 result 和 service 标签，具体订单只出现在日志里。

package orderservice

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestReviewRecorder 验证调用一次便同时产生告警和计数，避免接口接上了却没有实际记录。
func TestReviewRecorder(t *testing.T) {
	var output bytes.Buffer
	registry := telemetry.NewMetrics("order-service")
	recorder := newReviewRecorder(slog.New(slog.NewJSONHandler(&output, nil)), registry)
	recorder.RecordReview(t.Context(), 123, 456, "request-1", "deduct_outcome_unknown")
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["order_id"] != float64(123) || entry["sku_id"] != float64(456) || entry["idempotency_key"] != "request-1" || entry["level"] != "WARN" {
		t.Fatalf("告警字段不完整：%v", entry)
	}
	// Gather 返回真实注册表里的值，检查的不是一个单独的测试计数变量。
	metrics, err := registry.Registry.Gather()
	if err != nil || len(metrics) == 0 {
		t.Fatalf("指标注册：%v %v", metrics, err)
	}
	if testutil.ToFloat64(registry.Idempotency.WithLabelValues("needs_review")) != 1 {
		t.Fatal("待核对未计入统一注册表")
	}
	var family = metrics[0]
	for _, candidate := range metrics {
		if candidate.GetName() == "idempotency_requests_total" {
			family = candidate
		}
	}
	if family.GetName() != "idempotency_requests_total" || len(family.Metric) != 1 || family.Metric[0].GetCounter().GetValue() != 1 {
		t.Fatalf("指标内容：%v", family)
	}
	labels := family.Metric[0].Label
	if len(labels) != 2 {
		t.Fatalf("出现了额外指标标签：%v", labels)
	}
	for _, label := range labels {
		if label.GetName() != "service" && label.GetName() != "result" {
			t.Fatalf("订单标识进入指标标签：%v", label)
		}
	}
}
