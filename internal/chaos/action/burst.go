// 本文件：让流量工具发送有限批次的商品查询，并确认真实完成情况。
//
// 每批只查一个商品。上一批结束后才安排下一批，不在注入器中创建并发 HTTP 工作者。
// loadgen 仍负责发送真实业务请求，本文件只调用它的本机控制接口。
//
// 主要内容：
//   - burst：依次请求商品突发，控制批次间距。
//   - waitBurst：核对批次编号、完成数与失败数，不能把接收成功当成执行成功。
//   - pause：让等待服从整个注入动作的截止时间。
//
// 阅读建议：出现失败立即停止后续批次，恢复流量和服务由外层实验流程负责。

package action

import (
	"context"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// burst 让每批内部同时出发，再按批次大小留出间距，使平均速率不超过 320 次/秒。
// 例如 16 个请求间隔 50ms，64 个请求间隔 200ms，避免把入口限流混成缓存症状。
func (runner *Runner) burst(ctx context.Context, step scenario.Step) error {
	for offset := 0; offset < step.Count; offset++ {
		started := time.Now()
		var accepted struct {
			Data struct {
				ID uint64 `json:"id"` // 后面的进度必须属于这一批，不能沿用上一批成功。
			} `json:"data"`
		}
		input := map[string]any{"sku_id": step.FirstSKUID + int64(offset), "requests": step.Requests}
		if err := runner.request(ctx, "POST", runner.LoadURL+"/burst", input, &accepted); err != nil {
			return err
		}
		if accepted.Data.ID == 0 {
			return fmt.Errorf("流量工具没有返回有效批次编号")
		}
		if err := runner.waitBurst(ctx, accepted.Data.ID, step.Requests); err != nil {
			return err
		}
		// 一批若已经花了更久，就不额外等待；只有批间距过短时才补足时间。
		interval := time.Duration(step.Requests) * time.Second / 320
		if err := pause(ctx, interval-time.Since(started)); err != nil {
			return err
		}
	}
	return nil
}

// waitBurst 不会自动重发失败批次，否则一次缓存失效可能被重复流量改变成另一种实验。
func (runner *Runner) waitBurst(ctx context.Context, id uint64, requests int) error {
	for {
		var response struct {
			Data struct {
				Burst struct {
					ID        uint64 `json:"id"`        // 防止把后来被别人发起的批次当成本次结果。
					Completed int    `json:"completed"` // 实际 HTTP 尝试已结束的数量。
					Failed    int    `json:"failed"`    // 非零就不能把这批称为成功商品查询。
					Skipped   int    `json:"skipped"`   // 未发出的任务必须保留为失败实验。
					Running   bool   `json:"running"`   // false 后才能使用最终统计。
				} `json:"burst"`
			} `json:"data"`
		}
		if err := runner.request(ctx, "GET", runner.LoadURL+"/control", nil, &response); err != nil {
			return err
		}
		batch := response.Data.Burst
		if batch.ID != id {
			return fmt.Errorf("流量工具返回了不同批次的进度")
		}
		if !batch.Running {
			if batch.Completed != requests || batch.Failed != 0 || batch.Skipped != 0 {
				return fmt.Errorf("商品突发未完整成功：完成 %d，失败 %d，未发出 %d", batch.Completed, batch.Failed, batch.Skipped)
			}
			return nil
		}
		// 只轮询控制状态，不制造新的业务请求；外层三分钟截止也能打断等待。
		if err := pause(ctx, 20*time.Millisecond); err != nil {
			return err
		}
	}
}

// pause 使用可取消的定时器，让停止实验不必等到整个批次间隔结束。
func pause(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
