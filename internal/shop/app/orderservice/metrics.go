// 本文件：把订单转入待核对的次数写进指标，订单号、商品和原因留在日志中供人工核对。

package orderservice

import (
	"context"
	"log/slog"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

// reviewRecorder 把日志与计数放在同一个动作里，避免某个失败分支只做其中一项。
type reviewRecorder struct {
	logger      *slog.Logger       // JSON 日志里保留人工核对所需的编号。
	needsReview prometheus.Counter // 只记录状态转换次数，进程重启后从零开始。
}

// newReviewRecorder 使用 app 自己的注册表，不污染全局注册表，也便于测试重复创建服务。
func newReviewRecorder(logger *slog.Logger, metrics *telemetry.Metrics) *reviewRecorder {
	needsReview := metrics.Idempotency.WithLabelValues("needs_review")
	needsReview.Add(0) // 没有待核对事件时也导出零值，便于确认采集路径已经接通。
	return &reviewRecorder{logger: logger, needsReview: needsReview}
}

// RecordReview 使用固定原因分类，不把原始连接错误、DSN 或请求体写入告警。
func (recorder *reviewRecorder) RecordReview(ctx context.Context, orderID, skuID int64, key, reason string) {
	recorder.logger.WarnContext(ctx, "下单结果待人工核对", "order_id", orderID, "sku_id", skuID, "idempotency_key", key, "reason", reason)
	recorder.needsReview.Inc()
}
