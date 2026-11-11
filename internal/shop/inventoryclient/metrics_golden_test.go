// 本文件：从库存客户端的真实注册入口验证查询和扣减各自暴露的熔断器指标。

package inventoryclient

import (
	"bytes"
	"os"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestBreakerMetricsGolden 在假 RPC 上只让扣减失败，以验证两个操作的状态和标签各自对应。
func TestBreakerMetricsGolden(t *testing.T) {
	common := telemetry.NewMetrics("order-service")
	client := testClient(t, &fakeRPC{writeErr: status.Error(codes.Unavailable, "库存服务暂不可用")}, 1)
	if err := client.RegisterMetrics(common); err != nil {
		t.Fatal(err)
	}
	// 一次扣减失败就打开本用例的熔断器，查询仍应为关闭状态。
	if err := client.Deduct(t.Context(), 1, 1); err == nil {
		t.Fatal("假 RPC 应当返回扣减失败")
	}
	want, err := os.ReadFile("testdata/breaker_metrics.golden.prom")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GatherAndCompare(common.Registry, bytes.NewReader(want), "circuit_breaker_state"); err != nil {
		t.Fatal(err)
	}
}
