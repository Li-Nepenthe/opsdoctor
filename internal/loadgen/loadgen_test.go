// 本文件：验证压测请求分布、重试身份和真实统计口径。
//
// HTTP 通过假的 RoundTripper 在内存中处理，不连接网络。随机分布测试使用固定种子，结果可以复现。
//
// 主要内容：
//   - roundTripFunc：把普通函数适配成 HTTP 发送器。
//   - TestTrafficDistribution：验证 70/20/10 比例、Zipf 热门倾向和指定热点。
//   - TestRetryKeepsKey：超时只重试一次，保持请求体和幂等键。
//   - TestStatistics：核对 QPS、错误率和一毫秒桶的百分位。
//   - TestControl：检查暂停、热点解除和无效更新。
//   - TestBoundedWorkers：工作者数量受限，停止后没有在途请求残留。
//
// 阅读建议：先看第一次超时如何仍留在统计里，再看为什么控制参数不是实际 QPS。

package loadgen

import (
	"context"
	"io"
	"log/slog"
	mathrand "math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripFunc 让测试只提供一个函数，不必创建监听端口才能观察请求。
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 满足 http.RoundTripper 接口，http.Client 会照常执行请求流程。
func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

// testGenerator 使用不会联网的客户端。需要响应的测试会自行替换 Transport。
func testGenerator() *generator {
	settings := Config{Target: "http://example.invalid", QPS: 200, ProductCount: 500000, Workers: 2, RequestTimeout: time.Second}
	control := &controlState{settings: controlSettings{QPS: settings.QPS}, productCount: settings.ProductCount}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.Canceled })}
	return newGenerator(settings, control, client, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestTrafficDistribution 不要求每十次恰好七二一，而是核对大量固定随机样本的比例。
func TestTrafficDistribution(t *testing.T) {
	generator := testGenerator()
	generator.random = mathrand.New(mathrand.NewPCG(1, 2))
	generator.zipf = mathrand.NewZipf(generator.random, 1.1, 100, 499999)
	var counts [3]int
	hot := 0
	for range 100000 {
		value := generator.nextJob(0)
		counts[value.kind]++
		if value.skuID <= 100 {
			hot++
		}
		if value.skuID <= 0 || value.skuID > 500000 {
			t.Fatal("生成了不存在范围内的商品编号")
		}
		if value.kind == orderRequest && (value.key == "" || len(value.key) > 64) {
			t.Fatal("下单没有合法长度的幂等键")
		}
	}
	// 容许一个百分点的样本波动。固定种子让这个检查不会偶发靠运气通过。
	for index, want := range []int{70000, 20000, 10000} {
		if counts[index] < want-1000 || counts[index] > want+1000 {
			t.Fatalf("请求类型分布=%v", counts)
		}
	}
	if hot < 1000 {
		t.Fatalf("没有观察到 Zipf 热门倾向：前百商品出现 %d 次", hot)
	}
	for range 100 {
		if value := generator.nextJob(42); value.skuID != 42 {
			t.Fatal("指定热点未生效")
		}
	}
}

// TestRetryKeepsKey 检查超时重试与不该重试的响应，同时验证统计保留每次尝试。
func TestRetryKeepsKey(t *testing.T) {
	for _, testcase := range []struct {
		name          string      // 描述首次失败类型。
		kind          requestKind // 只有下单允许超时重试。
		firstStatus   int         // 非零时返回 HTTP 响应，零时返回网络超时。
		alwaysTimeout bool        // 第二次也超时，检查不会出现第三次尝试。
		attempts      uint64      // 包含首次的实际尝试数。
		errors        uint64      // 统计中的真实失败数。
	}{
		{"下单网络超时", orderRequest, 0, false, 2, 1},   // 重试成功不能抹掉第一次错误。
		{"下单 504", orderRequest, 504, false, 2, 1}, // HTTP 超时响应也沿用原键再试一次。
		{"两次都超时", orderRequest, 0, true, 2, 2},     // 达到两次总尝试后停止。
		{"下游不可用", orderRequest, 503, false, 1, 1},  // 503 不是本工具的超时重试条件。
		{"结果待核对", orderRequest, 409, false, 1, 1},  // 不能反复重做不确定请求。
		{"商品超时", productRequest, 0, false, 1, 1},   // loadgen 只实现约定的下单重试。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			generator := testGenerator()
			var keys, bodies []string
			generator.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				keys = append(keys, request.Header.Get("Idempotency-Key"))
				data, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				bodies = append(bodies, string(data))
				if (len(keys) == 1 && testcase.firstStatus == 0) || testcase.alwaysTimeout {
					return nil, context.DeadlineExceeded
				}
				status := 201
				if len(keys) == 1 {
					status = testcase.firstStatus
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"code":"OK"}`)), Header: make(http.Header)}, nil
			})
			generator.execute(t.Context(), job{kind: testcase.kind, skuID: 1, userID: 2, key: "same-order-key"})
			report := generator.stats.snapshot(time.Now(), false)
			if report.Requests != testcase.attempts || report.Errors != testcase.errors {
				t.Fatalf("统计=%+v", report)
			}
			if testcase.attempts == 2 && (keys[0] != "same-order-key" || keys[1] != keys[0] || bodies[1] != bodies[0]) {
				t.Fatalf("重试改变了请求身份：%v %v", keys, bodies)
			}
		})
	}
}

// TestStatistics 用已知的一百个样本核对百分位，不把配置 QPS 直接写进报告。
func TestStatistics(t *testing.T) {
	started := time.Now()
	stats := newStatistics(started)
	for index := 1; index <= 100; index++ {
		stats.record(productRequest, time.Duration(index)*time.Millisecond, index == 100)
	}
	stats.drop()
	window := stats.snapshot(started.Add(time.Second), true)
	if window.QPS != 100 || window.P50MS != 50 || window.P99MS != 99 || window.ErrorRate != 0.01 || window.Dropped != 1 {
		t.Fatalf("窗口统计不正确：%+v", window)
	}
	// 读取并清空窗口后，总计仍保留这一百次请求。
	total := stats.snapshot(started.Add(2*time.Second), false)
	if total.Requests != 100 || total.QPS != 50 {
		t.Fatalf("总计被窗口清空：%+v", total)
	}
	stats.record(orderRequest, 500*time.Microsecond, false)
	next := stats.snapshot(started.Add(2*time.Second), true)
	if next.Requests != 1 || next.P50MS != 1 {
		t.Fatalf("向上取整或窗口重置失败：%+v", next)
	}
}

// TestControl 使用内存 HTTP 请求检查部分更新，非法值不能改动现有设置。
func TestControl(t *testing.T) {
	control := &controlState{settings: controlSettings{QPS: 200}, productCount: 100}
	router := controlRouter(control, newStatistics(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, testcase := range []struct {
		body   string          // 每一行是一次控制操作。
		status int             // 是否接受更新。
		want   controlSettings // 更新后的完整设置。
	}{
		{`{"hot_sku_id":42}`, 200, controlSettings{QPS: 200, HotSKUID: 42}}, // 只改热点，速率不变。
		{`{"qps":0}`, 200, controlSettings{QPS: 0, HotSKUID: 42}},           // 零表示暂停，不是缺少字段。
		{`{"qps":-1}`, 400, controlSettings{QPS: 0, HotSKUID: 42}},          // 无效更新不改变当前状态。
		{`{"hot_sku_id":101}`, 400, controlSettings{QPS: 0, HotSKUID: 42}},  // 不向数据范围外制造基准流量。
		{`{"qps":200,"hot_sku_id":0}`, 200, controlSettings{QPS: 200}},      // 恢复速率并解除热点。
		{`{}`, 400, controlSettings{QPS: 200}},                              // 空更新容易误操作，明确拒绝。
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/control", strings.NewReader(testcase.body)))
		if response.Code != testcase.status || control.snapshot() != testcase.want {
			t.Fatalf("控制结果：%s %+v", response.Body.String(), control.snapshot())
		}
	}
}

// TestBoundedWorkers 让发送持续等到请求超时，确认高 QPS 也不会无限启动工作者。
func TestBoundedWorkers(t *testing.T) {
	generator := testGenerator()
	generator.settings.RequestTimeout = 10 * time.Millisecond
	generator.control.settings.QPS = 10000
	var active atomic.Int32
	generator.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if active.Add(1) > 2 {
			t.Error("超过固定工作者上限")
		}
		defer active.Add(-1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	generator.run(ctx)
	// run 返回意味着在途请求已经退出，不能只停止调度却把工作者留在后台。
	report := generator.stats.snapshot(time.Now(), false)
	if active.Load() != 0 || report.Requests == 0 || report.Dropped == 0 || report.ErrorRate != 1 {
		t.Fatalf("排空或统计失败：active=%d %+v", active.Load(), report)
	}
}
