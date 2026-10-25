// 本文件：验证商品突发只查询商品，并且始终受固定工作者上限约束。
//
// 测试用假的 HTTP 发送器拦住请求，检查全部工作者是否一起到达，以及第二批是否被拒绝。
// 这些测试不连接靶场，也不会影响正在进行的故障实验。
//
// 主要内容：
//   - TestProductBurst：检查暂停普通流量后仍能发出一批查询，完成统计保持真实。
//   - TestBurstValidation：检查商品范围、批次大小与重复提交的边界。
//   - TestBurstCancellation：调度中途取消时，已就位的工作者仍能离开等待。
//
// 阅读建议：先看 entered 和 release 两个通道，它们把并发时间点固定下来，不靠猜测睡多久。

package loadgen

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestProductBurst 保留普通工作者的总上限。把普通流量暂停，可以精确核对一批请求的类型和数量。
func TestProductBurst(t *testing.T) {
	generator := testGenerator()
	generator.settings.Workers = 4
	generator.bursts.workers = 4
	generator.control.settings.QPS = 0
	entered, release := make(chan struct{}, 4), make(chan struct{})
	var active atomic.Int32
	generator.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/products/42" {
			t.Errorf("突发不该下单或更换商品：%s %s", request.Method, request.URL.Path)
		}
		active.Add(1)
		defer active.Add(-1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":"OK"}`))}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { generator.run(ctx); close(done) }()
	accepted, err := generator.bursts.submit(42, 4)
	if err != nil || accepted.Completed != 0 || !accepted.Running {
		t.Fatalf("接收时不能提前报告完成：%+v %v", accepted, err)
	}
	for range 4 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("没有让整批工作者一起开始查询")
		}
	}
	if _, err := generator.bursts.submit(42, 4); err == nil {
		t.Error("上一批尚未结束，却接受了叠加批次")
	}
	if active.Load() != 4 {
		t.Errorf("四个工作者没有同时在途：%d", active.Load())
	}
	close(release)
	cancel()
	<-done // run 会排空已发出的查询，返回后才能检查最终统计。
	report := generator.bursts.snapshot()
	total := generator.stats.snapshot(time.Now(), false)
	if active.Load() != 0 || report.Running || report.Completed != 4 || report.Failed != 0 || total.Requests != 4 {
		t.Fatalf("并发上限或真实完成数不正确：active=%d burst=%+v total=%+v", active.Load(), report, total)
	}
}

// TestBurstValidation 把非法输入挡在进入队列之前，避免靠工作者内部再丢掉超出的请求。
func TestBurstValidation(t *testing.T) {
	for _, testcase := range []struct {
		name     string // 每个边界单独命名，失败时能直接看出错放行了哪一种输入。
		skuID    int64  // 已有商品范围是 1..100。
		requests int    // 固定工作者数是四，不能临时扩容。
	}{
		{"商品零值", 0, 1},   // 商品编号必须显式给出。
		{"商品越界", 101, 1}, // 不制造不存在商品的流量。
		{"空批次", 1, 0},    // 接收空批次没有实际意义。
		{"负数批次", 1, -1},  // 不能把负数转成很大的无符号计数。
		{"超过工作者", 1, 5},  // 并发上限对每次提交都生效。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			bursts := &burstState{queue: make(chan *burstBatch, 1), workers: 4, productCount: 100}
			if _, err := bursts.submit(testcase.skuID, testcase.requests); err == nil || len(bursts.queue) != 0 {
				t.Fatal("无效批次进入了队列")
			}
		})
	}
}

// TestBurstCancellation 只让一个工作者接到任务，然后取消；其余任务必须标成未发出，不能伪装成成功。
func TestBurstCancellation(t *testing.T) {
	generator := testGenerator()
	accepted, err := generator.bursts.submit(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	batch := <-generator.bursts.queue
	// 不真正启动 HTTP 工作者，只接走第一项任务，精确停在第二项尚未交出的时刻。
	jobs := make(chan job)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { generator.dispatchBurst(ctx, jobs, batch); close(done) }()
	value := <-jobs
	cancel()
	<-done
	select {
	case <-value.burst.start:
	default:
		t.Fatal("已就位的工作者还在等待出发信号")
	}
	generator.bursts.finish(accepted.ID, true, 0)
	report := generator.bursts.snapshot()
	if report.Skipped != 1 || report.Completed != 1 || report.Failed != 1 || report.Running {
		t.Fatalf("取消后的任务状态不准确：%+v", report)
	}
}
