// 本文件：用内存 HTTP 传输检查模型协议、重试、费用预留与取消。
//
// 不开放端口，也不读取 API Key。测试里的密钥只是固定假字符串；Transport 直接
// 返回预设 HTTP 回复，因此运行 go test 不会产生供应商费用。
//
// 主要内容：TestHTTPPolicy 检查错误码；TestWireAudit 检查脱敏与协议；
// TestCancel、TestCancelBody 分别检查等待响应头和读取正文时的取消。
// TestAttemptPrices 检查请求跨过价格时段边界仍按发送时的价格结算。
// 阅读建议：账本的 Reserve 必须先于 Transport 被调用，这也是测试中的断言。

package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// memoryLedger 只模拟记账顺序，数据库的原子上限另由 store 集成测试验证。
type memoryLedger struct {
	mu           sync.Mutex        // 保护取消测试中两个 goroutine 的记录。
	reservations []llm.Reservation // 每次真正发送前的字节。
	settlements  []llm.Settlement  // 每次尝试的结算。
	deny         bool              // 模拟额度不足，不能触碰 HTTP。
}

// Reserve 返回独立编号，以便检查两次重试没有复用旧的付费记录。
func (ledger *memoryLedger) Reserve(_ context.Context, value llm.Reservation) (string, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.deny {
		return "", llm.ErrBudget
	}
	ledger.reservations = append(ledger.reservations, value)
	return fmt.Sprint(len(ledger.reservations)), nil
}

// Settle 保存已完成的记账，不把取消误当作供应商没有消费 token。
func (ledger *memoryLedger) Settle(_ context.Context, _ string, value llm.Settlement) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.settlements = append(ledger.settlements, value)
	return nil
}

// transportFunc 让 HTTP 客户端正常构造请求，但在真正联网前把请求交给测试。
type transportFunc func(*http.Request) (*http.Response, error)

// RoundTrip 满足 net/http 接口，不需要起真实供应商连接。
func (function transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

// clientForTest 使用独立账本和日志缓冲，避免包级状态在并行测试中互相影响。
func clientForTest(t *testing.T, ledger *memoryLedger, transport transportFunc, output io.Writer) *Client {
	t.Helper()
	client, err := New(Config{BaseURL: "https://model.invalid", Key: "sk-demo123456789", Timeout: time.Minute, Prices: llm.Prices{InputMicros: 2000000, CacheHitMicros: 40000, OutputMicros: 8000000}}, transport, ledger, llm.NewRedactor(), slog.New(slog.NewTextHandler(output, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// requestForTest 每个测试都使用完整归属和有限输出，不绕过生产的请求校验。
func requestForTest() llm.Request {
	return llm.Request{DiagnosisID: "demo-diagnosis", Model: "requested-model", MaxTokens: 1024, Messages: []llm.Message{{Role: "user", Content: "symptom"}}}
}

// reply 生成假的 HTTP 响应，Body 仍由生产代码按有界读取规则消费。
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"2"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// TestHTTPPolicy 用虚拟时钟检查确切等待时间，计算机忙不会改变结果。
func TestHTTPPolicy(t *testing.T) {
	for _, testcase := range []struct{ status, attempts int }{
		{400, 1}, {401, 1}, {402, 1}, {403, 1}, {429, 3}, {500, 3}, {502, 3}, {503, 3}, {504, 3}, {501, 1},
	} {
		// 仅限流和选定的暂时服务错误可重试；其余要由用户或开发者修正。
		t.Run(fmt.Sprint(testcase.status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ledger := &memoryLedger{}
				calls := 0
				client := clientForTest(t, ledger, func(*http.Request) (*http.Response, error) {
					calls++
					return reply(testcase.status, `{"error":{"message":"private"}}`), nil
				}, io.Discard)
				start := time.Now()
				_, err := client.Complete(t.Context(), requestForTest())
				if err == nil || calls != testcase.attempts || len(ledger.reservations) != calls || time.Since(start) != time.Duration(calls-1)*2*time.Second {
					t.Fatalf("calls=%d elapsed=%s err=%v", calls, time.Since(start), err)
				}
			})
		})
	}
}

// TestWireAudit 核对真实发送字节就是落库字节，模型名和缓存 token 不会丢失。
func TestWireAudit(t *testing.T) {
	// 先比较发送与审计字节，再验证供应商用量被原样保留。
	ledger := &memoryLedger{}
	client := clientForTest(t, ledger, func(request *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(request.Body)
		if len(ledger.reservations) != 1 || string(raw) != string(ledger.reservations[0].Request) || strings.Contains(string(raw), "sk-demo") {
			t.Fatal("发送前没有保存同一份脱敏内容")
		}
		var wire llm.Request
		_ = json.Unmarshal(raw, &wire)
		if wire.Thinking.Type != "disabled" || wire.Stream || wire.MaxTokens != 1024 || wire.DiagnosisID != "" {
			t.Fatal("协议字段不符合约定")
		}
		return reply(200, `{"model":"actual-model","choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"service_topology","arguments":"{}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_cache_hit_tokens":80}}`), nil
	}, io.Discard)
	request := requestForTest()
	request.Messages[0].Content = "key sk-demo123456789"
	result, err := client.Complete(t.Context(), request)
	if err != nil || result.Model != "actual-model" || result.Usage.CacheHit != 80 || result.CostMicros != 204 || len(result.Message.ToolCalls) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	ledger.deny = true
	if _, err := client.Complete(t.Context(), request); !errors.Is(err, llm.ErrBudget) {
		t.Fatal("预算未拦截")
	}
}

// TestCancel 的 Transport 一直等 context，取消必须传到它，且不会再发第二次请求。
func TestCancel(t *testing.T) {
	ledger := &memoryLedger{}
	entered := make(chan struct{}) // Transport 写，测试读，只发送一次后关闭。
	var logs strings.Builder
	client := clientForTest(t, ledger, func(request *http.Request) (*http.Response, error) {
		close(entered)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}, &logs)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1) // 调用 goroutine 写，测试等待后退出，不留后台请求。
	go func() { _, err := client.Complete(ctx, requestForTest()); finished <- err }()
	<-entered
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) || len(ledger.reservations) != 1 || !ledger.settlements[0].Unknown || !strings.Contains(logs.String(), "模型请求已中断") {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消没有在两秒内结束")
	}
}

// cancelledBody 模拟响应头已到达、正文还没传完的情况，Read 一直等到请求被取消。
type cancelledBody struct {
	ctx     context.Context // 和发送 HTTP 请求使用同一份取消信号。
	entered chan struct{}   // 第一次读取时关闭，让测试准确控制取消时机。
}

// Read 只调用一次就返回取消错误，不依赖真实网络是否恰好卡住。
func (body *cancelledBody) Read([]byte) (int, error) {
	close(body.entered)
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

// Close 没有真实资源需要释放，保留接口让生产客户端执行原有清理路径。
func (*cancelledBody) Close() error { return nil }

// TestCancelBody 防止只有 HTTP.Do 的取消被记录，读正文中断却变成普通解析错误。
func TestCancelBody(t *testing.T) {
	ledger := &memoryLedger{}
	entered := make(chan struct{})
	var logs strings.Builder
	client := clientForTest(t, ledger, func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &cancelledBody{ctx: request.Context(), entered: entered}}, nil
	}, &logs)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1) // 结果发出后请求 goroutine 退出，测试不会遗留读正文的任务。
	go func() { _, err := client.Complete(ctx, requestForTest()); finished <- err }()
	<-entered
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) || len(ledger.reservations) != 1 || !ledger.settlements[0].Unknown || !strings.Contains(logs.String(), "模型请求已中断") {
			t.Fatalf("正文取消没有传到调用方：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消读正文超过两秒")
	}
}

// TestAttemptPrices 固定发送时的价格，返回时价格改变也不能改变这次结算。
func TestAttemptPrices(t *testing.T) {
	ledger := &memoryLedger{}
	cheap := llm.Prices{InputMicros: 1000000, CacheHitMicros: 20000, OutputMicros: 4000000}
	selected := cheap
	client := clientForTest(t, ledger, func(*http.Request) (*http.Response, error) {
		selected = llm.Prices{InputMicros: 2000000, CacheHitMicros: 40000, OutputMicros: 8000000}
		return reply(200, `{"model":"requested-model","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1000,"prompt_cache_hit_tokens":800,"completion_tokens":100}}`), nil
	}, io.Discard)
	client.config.PricesAt = func(time.Time) llm.Prices { return selected }
	result, err := client.Complete(t.Context(), requestForTest())
	// 200 未缓存输入 + 800 缓存输入 + 100 输出，按发出时的闲时价结算。
	if err != nil || result.CostMicros != 616 || ledger.reservations[0].Prices != cheap || ledger.settlements[0].CostMicros != 616 {
		t.Fatalf("结算改变：%+v %v", result, err)
	}
}
