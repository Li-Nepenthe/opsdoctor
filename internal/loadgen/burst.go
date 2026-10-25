// 本文件：把一小批商品查询同时交给已有的工作者。
//
// 秒杀开始时，一批用户可能同时打开同一件商品。普通的均匀发请求方式不容易重现这个瞬间。
// 这里先让工作者就位，再一起发出查询。仍复用固定的工作者，不为每个 HTTP 控制请求创建一批 goroutine。
//
// 主要内容：
//   - burstState、burstBatch：保存一批查询的进度和共同的出发信号。
//   - submit：检查商品范围与并发上限，拒绝叠加尚未结束的批次。
//   - finish、snapshot：记录真实完成数，供控制接口查询。
//   - dispatchBurst：等已有工作者接好任务后，再放行这一批商品查询。
//
// 阅读建议：start 通道关闭只表示“可以出发”，不表示请求已经成功。

package loadgen

import (
	"context"
	"fmt"
	"sync"
)

// burstReport 把接收、完成和未发出分开，不能把 202 Accepted 当成业务成功。
type burstReport struct {
	ID        uint64 `json:"id"`        // 进程内递增，便于确认看到的是哪一批。
	SKUID     int64  `json:"sku_id"`    // 仅出现在本机控制接口，不用作指标标签。
	Requests  int    `json:"requests"`  // 本批请求数，同时也是最多需要的工作者数。
	Completed int    `json:"completed"` // 真正执行完 HTTP 尝试的数量，包含失败。
	Failed    int    `json:"failed"`    // 失败仍保留在完成数及压测总统计中。
	Skipped   int    `json:"skipped"`   // 进程停止时尚未交给工作者的数量。
	Running   bool   `json:"running"`   // 所有任务完成或明确跳过后才变为 false。
}

// burstBatch 只装有限的任务信息。所有工作者等待同一个 start，不复制一份请求队列。
type burstBatch struct {
	id       uint64        // 对应控制接口返回的批次编号。
	skuID    int64         // 一批只查一个商品，不生成订单或扣库存。
	requests int           // 不超过固定工作者数量。
	start    chan struct{} // 调度者关闭后，已经就位的工作者一起开始。
}

// burstState 用一把锁保护进度；队列最多容纳一批，运行中的批次不能叠加。
type burstState struct {
	mu           sync.Mutex       // 保护 report，调度与 HTTP 请求会同时访问。
	report       burstReport      // 只保存最后一批，不随运行时间积累历史。
	queue        chan *burstBatch // 容量为一，工作者仍由 generator.run 统一创建。
	workers      int              // 复用启动配置的上限，最大为 256。
	productCount int              // 不向尚未造数的商品发送实验流量。
}

// submit 先检查边界，再占用唯一的批次位置，避免并发控制请求突破上限。
func (bursts *burstState) submit(skuID int64, requests int) (burstReport, error) {
	if skuID < 1 || skuID > int64(bursts.productCount) || requests < 1 || requests > bursts.workers {
		return burstReport{}, fmt.Errorf("sku_id 必须在已有商品范围内，requests 必须在 1..%d 之间", bursts.workers)
	}
	bursts.mu.Lock()
	defer bursts.mu.Unlock()
	if bursts.report.Running {
		return burstReport{}, fmt.Errorf("上一批商品查询尚未结束")
	}
	bursts.report = burstReport{ID: bursts.report.ID + 1, SKUID: skuID, Requests: requests, Running: true}
	bursts.queue <- &burstBatch{id: bursts.report.ID, skuID: skuID, requests: requests, start: make(chan struct{})}
	return bursts.report, nil
}

// finish 在请求结束或调度被取消时更新进度。跳过的任务没有发送，不能算成请求失败。
func (bursts *burstState) finish(id uint64, failed bool, skipped int) {
	bursts.mu.Lock()
	defer bursts.mu.Unlock()
	if bursts.report.ID != id {
		return // 旧批次不能改写后来一批的进度。
	}
	if skipped > 0 {
		bursts.report.Skipped += skipped
	} else {
		bursts.report.Completed++
		if failed {
			bursts.report.Failed++
		}
	}
	bursts.report.Running = bursts.report.Completed+bursts.report.Skipped < bursts.report.Requests
}

// snapshot 返回副本，HTTP 编码时不占用工作者更新统计所需的锁。
func (bursts *burstState) snapshot() burstReport {
	bursts.mu.Lock()
	defer bursts.mu.Unlock()
	return bursts.report
}

// dispatchBurst 暂停普通调度，复用同一组工作者，因而突发与普通流量相加也不会超过总上限。
func (generator *generator) dispatchBurst(ctx context.Context, jobs chan<- job, batch *burstBatch) {
	defer close(batch.start) // 即使中途取消，也要让已经接到任务的工作者结束等待。
	for sent := 0; sent < batch.requests; sent++ {
		select {
		case <-ctx.Done():
			generator.bursts.finish(batch.id, false, batch.requests-sent)
			return
		case jobs <- job{kind: productRequest, skuID: batch.skuID, burst: batch}:
		}
	}
}
