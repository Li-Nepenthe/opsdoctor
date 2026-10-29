// 本文件：证明网络重试有次数、时间和费用边界，不会把未知费用当成免费请求。
//
// 主要内容：TestNetworkRetry 检查无响应、部分响应和恢复；TestNetworkRetryBudget
// 检查第二次预留被拒时不再发送；TestNetworkAuditFailure 检查记账失败就停下。
// TestNetworkTimeoutOrigin 区分供应商未响应与上层取消，不把总预算到期当作补跑理由。
// 阅读建议：synctest 使用虚拟时钟，退避一秒和两秒无需真实等待。

package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestNetworkRetry 分开验证从未收到响应和响应已经开始的两种失败。
func TestNetworkRetry(t *testing.T) {
	for _, test := range []struct {
		name               string
		firstByte, recover bool
		calls              int
		wait               time.Duration
	}{
		{"始终没有响应最多三次", false, false, 3, 3 * time.Second}, // 两次额外尝试，退避一秒、两秒。
		{"第二次恢复保留首次预留", false, true, 2, time.Second},     // 后一次成功不能替前一次取消预留。
		{"部分响应不重试", true, false, 1, 0},                   // 连响应头都没收齐也可能已经收到字节。
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ledger := &memoryLedger{}
				calls := 0
				client := clientForTest(t, ledger, func(request *http.Request) (*http.Response, error) {
					calls++
					if test.recover && calls == 2 {
						return reply(200, `{"model":"requested-model","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`), nil
					}
					if test.firstByte {
						httptrace.ContextClientTrace(request.Context()).GotFirstResponseByte()
					}
					return nil, io.EOF
				}, io.Discard)
				start := time.Now()
				result, err := client.Complete(t.Context(), requestForTest())
				if calls != test.calls || len(ledger.reservations) != calls || len(ledger.settlements) != calls || time.Since(start) != test.wait || (err == nil) != test.recover {
					t.Fatalf("calls=%d wait=%s err=%v", calls, time.Since(start), err)
				}
				var charged int64
				for index, settlement := range ledger.settlements {
					if settlement.Status == "network_error" {
						if !settlement.Unknown {
							t.Fatal("网络失败丢失未知费用")
						}
						charged += ledger.reservations[index].MaximumMicros
					} else {
						charged += settlement.CostMicros
					}
				}
				if result.CostMicros != charged || len(result.AttemptIDs) != calls {
					t.Fatal("重试费用或审计编号丢失")
				}
			})
		})
	}
}

// TestNetworkRetryBudget 确认 retry 仍先预留费用，没有额度就不接触 HTTP。
func TestNetworkRetryBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ledger := &memoryLedger{}
		calls := 0
		client := clientForTest(t, ledger, func(*http.Request) (*http.Response, error) {
			calls++
			ledger.deny = true // 首次尝试之后额度耗尽；下一次 Reserve 必须拦住发送。
			return nil, io.EOF
		}, io.Discard)
		result, err := client.Complete(t.Context(), requestForTest())
		if !errors.Is(err, llm.ErrBudget) || calls != 1 || result.CostMicros != ledger.reservations[0].MaximumMicros {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}

// brokenSettlement 模拟账本暂时不可写，先前的预留记录仍然存在。
type brokenSettlement struct{ *memoryLedger }

// Settle 故意失败；客户端不能在失去结算能力后接着发送更多付费请求。
func (ledger brokenSettlement) Settle(context.Context, string, llm.Settlement) error {
	return errors.New("模拟账本写入失败")
}

// TestNetworkAuditFailure 防止把审计故障误当作可重试的供应商网络问题。
func TestNetworkAuditFailure(t *testing.T) {
	ledger := &memoryLedger{}
	calls := 0
	client := clientForTest(t, ledger, func(*http.Request) (*http.Response, error) { calls++; return nil, io.EOF }, io.Discard)
	client.ledger = brokenSettlement{ledger}
	if _, err := client.Complete(t.Context(), requestForTest()); err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

// TestNetworkTimeoutOrigin 用虚拟时钟验证审计分类；次数与未知预留都不能随分类改变。
func TestNetworkTimeoutOrigin(t *testing.T) {
	for _, callerTimeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("上层到期=%t", callerTimeout), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ledger := &memoryLedger{}
				client := clientForTest(t, ledger, func(request *http.Request) (*http.Response, error) {
					<-request.Context().Done()
					return nil, request.Context().Err()
				}, io.Discard)
				client.config.Timeout = 2 * time.Second
				parent, cancel := context.WithCancel(t.Context())
				defer cancel()
				if callerTimeout {
					var stop context.CancelFunc
					parent, stop = context.WithTimeout(parent, time.Second)
					defer stop()
				}
				_, err := client.Complete(parent, requestForTest())
				want := "network_timeout"
				if callerTimeout {
					want = "cancelled" // 上层时限或取消不能申请额外的付费补跑。
				}
				if !errors.Is(err, context.DeadlineExceeded) || len(ledger.reservations) != 1 || len(ledger.settlements) != 1 || ledger.settlements[0].Status != want || !ledger.settlements[0].Unknown {
					t.Fatalf("审计状态错误：%+v %v", ledger.settlements, err)
				}
			})
		})
	}
}
