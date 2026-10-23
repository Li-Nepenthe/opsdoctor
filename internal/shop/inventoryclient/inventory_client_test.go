// 本文件：用假的 RPC 检查库存客户端的保护规则。
//
// 测试不连接库存服务。每次调用由 fakeRPC 记录，便于确认写操作没有被悄悄重试。
//
// 主要内容：
//   - fakeRPC：记录调用次数，按测试要求返回错误。
//   - TestReadRetryAndDeadline：查询最多尝试三次，所有尝试共用截止时间。
//   - TestWriteOnceAndBreaker：区分业务拒绝、结果不明和未发送请求。
//   - TestCanceledRequest：已取消的请求不发送到下游。
//
// 阅读建议：先看表格每行的说明，再看 wantedCalls 为什么包含首次尝试。

package inventoryclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeRPC 按调用次数返回结果。这些测试按顺序执行，不需要给计数器加锁。
type fakeRPC struct {
	getCalls     int         // 查询次数包含首次尝试。
	deductCalls  int         // 扣减次数用于检查熔断和不重试。
	releaseCalls int         // 归还次数必须与显式调用次数一致。
	getFailures  int         // 查询前几次模拟连接故障。
	writeErr     error       // 写请求要返回的业务拒绝或连接故障。
	deadlines    []time.Time // 保存每次查询实际收到的截止时间。
}

// GetStock 模拟先失败再恢复，同时记录传进来的预算。
func (fake *fakeRPC) GetStock(ctx context.Context, _ *inventoryv1.GetStockRequest, _ ...grpc.CallOption) (*inventoryv1.GetStockResponse, error) {
	fake.getCalls++
	deadline, _ := ctx.Deadline()
	fake.deadlines = append(fake.deadlines, deadline)
	if fake.getCalls <= fake.getFailures {
		return nil, status.Error(codes.Unavailable, "暂时断开")
	}
	return &inventoryv1.GetStockResponse{Available: 7}, nil
}

// Deduct 只记录写请求，不模拟真实库存，避免测试把关注点变成数据库逻辑。
func (fake *fakeRPC) Deduct(context.Context, *inventoryv1.DeductRequest, ...grpc.CallOption) (*inventoryv1.DeductResponse, error) {
	fake.deductCalls++
	return &inventoryv1.DeductResponse{}, fake.writeErr
}

// Release 与 Deduct 分开计数，检查扣减熔断后仍然可以尝试归还。
func (fake *fakeRPC) Release(context.Context, *inventoryv1.ReleaseRequest, ...grpc.CallOption) (*inventoryv1.ReleaseResponse, error) {
	fake.releaseCalls++
	return &inventoryv1.ReleaseResponse{}, fake.writeErr
}

// testClient 让熔断样本足够大，普通查询重试测试不会提前打开熔断器。
func testClient(t *testing.T, fake *fakeRPC, minimum uint32) *Client {
	t.Helper()
	client, err := New(fake, Config{Timeout: time.Second,
		Retry:   resilience.RetryPolicy{MaxRetries: 2, Backoff: 0, MinBudget: time.Millisecond},
		Breaker: resilience.BreakerConfig{Enabled: true, MinRequests: minimum, FailurePercent: 50, Interval: time.Minute, OpenTimeout: time.Minute, HalfOpenMax: 1}},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// TestReadRetryAndDeadline 同时核对成功恢复和达到次数上限，防止只测到成功路径。
func TestReadRetryAndDeadline(t *testing.T) {
	for _, testcase := range []struct {
		name        string // 名字说明这组失败能否在预算内恢复。
		failures    int    // 前几次调用返回 Unavailable。
		wantedCalls int    // 期望的总调用数，而不是重试数。
		wantError   bool   // 达到上限仍未成功时必须返回错误。
	}{
		{"首次成功", 0, 1, false},    // 健康时不能多发请求。
		{"第二次重试恢复", 2, 3, false}, // 两次失败后仍允许最后一次尝试。
		{"三次仍失败", 9, 3, true},    // 上限包含第一次之外的两次重试。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			fake := &fakeRPC{getFailures: testcase.failures}
			client := testClient(t, fake, 100)
			parent, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()
			_, err := client.GetStock(parent, 1)
			if (err != nil) != testcase.wantError || fake.getCalls != testcase.wantedCalls {
				t.Fatalf("calls=%d err=%v", fake.getCalls, err)
			}
			want, _ := parent.Deadline()
			for _, deadline := range fake.deadlines {
				if !deadline.Equal(want) {
					t.Fatalf("重试延长了上游预算：%v != %v", deadline, want)
				}
			}
		})
	}
}

// TestWriteOnceAndBreaker 业务拒绝不能熔断。连接故障则可能已经写入，必须保留这个区别。
func TestWriteOnceAndBreaker(t *testing.T) {
	fake := &fakeRPC{writeErr: status.Error(codes.FailedPrecondition, "库存不足")}
	client := testClient(t, fake, 2)
	for range 3 {
		err := client.Deduct(t.Context(), 1, 1)
		var writeErr *WriteError
		if !errors.Is(err, inventory.ErrInsufficientStock) || !errors.As(err, &writeErr) || writeErr.MayHaveApplied {
			t.Fatalf("业务拒绝分类不对：%v", err)
		}
	}
	if fake.deductCalls != 3 {
		t.Fatalf("业务拒绝触发了熔断：%d", fake.deductCalls)
	}
	// 用新实例清空之前的成功计数，连续两次连接错误就达到失败率门槛。
	fake = &fakeRPC{writeErr: status.Error(codes.Unavailable, "响应丢失")}
	client = testClient(t, fake, 2)
	for range 2 {
		var writeErr *WriteError
		err := client.Deduct(t.Context(), 1, 1)
		if !errors.As(err, &writeErr) || !writeErr.MayHaveApplied {
			t.Fatalf("没有保留结果不明：%v", err)
		}
	}
	var blocked *WriteError
	err := client.Deduct(t.Context(), 1, 1)
	if !errors.As(err, &blocked) || blocked.MayHaveApplied || fake.deductCalls != 2 {
		t.Fatalf("熔断后仍然扣减：%v calls=%d", err, fake.deductCalls)
	}
	_ = client.Release(t.Context(), 1, 1)
	if fake.releaseCalls != 1 {
		t.Fatal("归还被扣减熔断器阻止")
	}
}

// TestCanceledRequest 确认请求还没发出时就取消，不会留下“可能已经扣减”的标记。
func TestCanceledRequest(t *testing.T) {
	fake := &fakeRPC{}
	client := testClient(t, fake, 2)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, getErr := client.GetStock(ctx, 1)
	deductErr := client.Deduct(ctx, 1, 1)
	if !errors.Is(getErr, context.Canceled) || !errors.Is(deductErr, context.Canceled) || fake.getCalls+fake.deductCalls != 0 {
		t.Fatalf("取消后仍调用：%v %v", getErr, deductErr)
	}
}
