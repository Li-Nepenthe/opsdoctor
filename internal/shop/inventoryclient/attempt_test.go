// 本文件：验证单次查询时限与整次调用时限各自限制什么。
//
// 假 RPC 等待 context 到期，模拟真实慢响应；测试不连接网络。
// 单独放置这些测试，方便对照默认行为与显式缩短单次等待后的区别。
//
// 主要内容：
//   - expiringRPC.GetStock：让前两次查询用完分配的时间。
//   - TestAttemptTimeout：检查默认共享预算和分次预算的不同结果。
//   - TestAttemptConfiguration：拒绝无效时限和重试次数。
//   - TestAttemptBudgetNeverRetriesDeduct：确认提高查询重试上限不会重复扣减。
//
// 阅读建议：先看两个超时用例，再看为什么扣减次数始终是一。

package inventoryclient

import (
	"context"
	"errors"
	"testing"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// expiringRPC 复用普通假客户端的写方法，只把查询改成真正等待取消。
type expiringRPC struct {
	fakeRPC // 保存次数和截止时间，测试结束后统一核对。
}

// GetStock 在前两次等待截止，第三次成功，便于判断总预算是否给重试留下余量。
func (fake *expiringRPC) GetStock(ctx context.Context, request *inventoryv1.GetStockRequest, options ...grpc.CallOption) (*inventoryv1.GetStockResponse, error) {
	response, _ := fake.fakeRPC.GetStock(ctx, request, options...)
	if fake.getCalls <= 2 {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return response, nil
}

// TestAttemptTimeout 用真实计时器走取消路径，同时检查每次截止都没有越过父预算。
func TestAttemptTimeout(t *testing.T) {
	for _, testcase := range []struct {
		name      string        // 说明采用哪一种预算分配方式。
		attempt   time.Duration // 零表示不额外限制一次尝试。
		calls     int           // 包含第一次调用。
		wantError bool          // 默认首次超时就会耗尽总预算。
	}{
		{"默认共享总时限", 0, 1, true},                     // 保持旧行为：一个慢请求就能用完全部预算。
		{"每次单独分配", 20 * time.Millisecond, 3, false}, // 两次短超时后，第三次仍有时间成功。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			fake := &expiringRPC{}
			client := testClient(t, &fake.fakeRPC, 100)
			client.rpc, client.attemptTimeout = fake, testcase.attempt
			parent, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()
			_, err := client.GetStock(parent, 1)
			if (err != nil) != testcase.wantError || fake.getCalls != testcase.calls {
				t.Fatalf("calls=%d err=%v", fake.getCalls, err)
			}
			deadline, _ := parent.Deadline()
			for _, actual := range fake.deadlines {
				if actual.After(deadline) {
					t.Fatal("单次尝试越过总截止时间")
				}
			}
		})
	}
}

// TestAttemptConfiguration 直接调用构造函数，防止绕过 app 后带着坏配置启动。
func TestAttemptConfiguration(t *testing.T) {
	for _, testcase := range []struct {
		name    string        // 描述被检查的输入边界。
		attempt time.Duration // 总时限固定为一秒。
		retries int           // 第一次之外还能尝试几次。
		valid   bool          // 是否允许构造客户端。
	}{
		{"默认", 0, 2, true},                                 // 原有配置不需要填写新字段。
		{"等于总预算", time.Second, 10, true},                   // 等号与最大重试次数都合法。
		{"超过总预算", time.Second + time.Nanosecond, 2, false}, // 不能承诺比整次调用更长的时间。
		{"负预算", -time.Nanosecond, 2, false},                // 拒绝无意义的负时限。
		{"超过次数上限", 0, 11, false},                           // 防止配置变成无限尝试。
		{"负次数", 0, -1, false},                              // 不能把负数悄悄当作不重试。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			policy := resilience.RetryPolicy{MaxRetries: testcase.retries, MinBudget: time.Millisecond}
			breaker := resilience.BreakerConfig{MinRequests: 100, FailurePercent: 50, Interval: time.Minute, OpenTimeout: time.Second, HalfOpenMax: 1}
			_, err := New(&fakeRPC{}, Config{Timeout: time.Second, AttemptTimeout: testcase.attempt, Retry: policy, Breaker: breaker}, nil)
			if (err == nil) != testcase.valid {
				t.Fatalf("valid=%t err=%v", testcase.valid, err)
			}
		})
	}
}

// TestAttemptBudgetNeverRetriesDeduct 覆盖全部允许的重试次数，确认写操作没有进入查询重试器。
func TestAttemptBudgetNeverRetriesDeduct(t *testing.T) {
	for retries := 0; retries <= 10; retries++ {
		for _, attempt := range []time.Duration{0, 20 * time.Millisecond} {
			fake := &fakeRPC{writeErr: status.Error(codes.DeadlineExceeded, "响应未确认")}
			client := testClient(t, fake, 100)
			client.attemptTimeout = attempt
			var err error
			client.retry, err = resilience.NewRetrier(resilience.RetryPolicy{MaxRetries: retries, MinBudget: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Deduct(t.Context(), 1, 1); !errors.Is(err, context.DeadlineExceeded) || fake.deductCalls != 1 {
				t.Fatalf("retries=%d calls=%d err=%v", retries, fake.deductCalls, err)
			}
		}
	}
}
