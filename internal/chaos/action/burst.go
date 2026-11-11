// 本文件：宿主机控制接口一次只接收一批商品查询。
// burst 等待 waitBurst 核对本批的完成情况，失败后由外层流程恢复。

package action

import (
	"context"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// burst 让每批内部同时出发，再按批次大小留出间距，使平均速率不超过 320 次/秒。
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
					ID        uint64 `json:"id"` // 防止把后来被别人发起的批次当成本次结果。
					Completed int    `json:"completed"`
					Failed    int    `json:"failed"`
					Skipped   int    `json:"skipped"` // 未发出的任务必须保留为失败实验。
					Running   bool   `json:"running"` // 仅在工作结束后使用最终统计。
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
