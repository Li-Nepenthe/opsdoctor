// 本文件：执行已领取的审批单，只使用人审阅过的计划。
// 远端可能已经修改成功，本地还没来得及保存结果。恢复时先核对索引或连接池，旧 DDL 仍在运行时继续等待。

package approval

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// execute 的状态检查不能靠提示词代替；直接交来 pending 单也不会执行。
func (backend *Backend) execute(parent context.Context, record Record, saveConnection func(int64) error) tools.Result {
	if record.Status != "executing" {
		return result(record.Tool, "rejected", "审批单尚未获批并领取，禁止执行")
	}
	ctx, cancel := context.WithTimeout(parent, backend.timeout)
	defer cancel()
	var err error
	var data any
	switch record.Tool {
	case "create_index":
		data, err = backend.createIndex(ctx, record, saveConnection)
	case "set_db_pool_size":
		data, err = backend.setPool(ctx, record)
	case "warm_cache":
		var output map[string]int
		err = backend.opsRequest(ctx, record.Plan.Service, "POST", "/ops/cache/warm", map[string]any{"ids": record.Plan.IDs}, &output)
		data = output
	default:
		return result(record.Tool, "rejected", "未定义的写工具，禁止执行")
	}
	if err != nil {
		// 即使只是等待回复超时，远端也可能已经修改成功，不能声称“取消就等于没发生”。
		return result(record.Tool, "outcome_unknown", err.Error()+"；请核对审批单和真实状态，不要直接重复申请")
	}
	raw, _ := json.Marshal(data)
	out := result(record.Tool, "ok", "已按获批参数完成或确认此前已经完成；请继续查询指标验证效果")
	out.Data = raw
	out.OriginalBytes = len(raw)
	return out
}

// createIndex 只在没有目标索引、且旧执行连接已消失后才重新发送；形状不符时留给人处理。
func (backend *Backend) createIndex(ctx context.Context, record Record, saveConnection func(int64) error) (any, error) {
	plan := record.Plan
	if backend.ddl == nil || !backend.tables[plan.Table] {
		return nil, fmt.Errorf("当前接入配置不再允许该索引操作")
	}
	for {
		columns, unique, exists, err := backend.metadata.IndexShape(ctx, plan.Table, plan.Index)
		if err != nil {
			return nil, fmt.Errorf("无法核对索引状态")
		}
		if exists {
			if unique != plan.Unique || !slices.Equal(columns, plan.Columns) {
				return nil, fmt.Errorf("同名索引结构与审批不同，未继续执行")
			}
			return map[string]any{"index": plan.Index, "confirmed": true}, nil
		}
		if record.ConnectionID == 0 {
			break
		}
		alive, err := backend.metadata.ConnectionAlive(ctx, record.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("无法确认旧的 DDL 连接是否仍在执行")
		}
		if !alive {
			break
		}
		// 不占据另一个写连接。旧 DDL 结束后重新看索引，而不是重复发送。
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("旧 DDL 连接仍在，等待超时，未重复执行")
		case <-timer.C:
		}
	}
	connection, err := backend.ddl.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取索引连接失败")
	}
	defer connection.Close()
	// 请求取消时 MySQL 可能仍在执行 DDL。用 ErrBadConn 丢弃专用连接，下一次恢复先核对线程和索引。
	defer func() { _ = connection.Raw(func(any) error { return driver.ErrBadConn }) }()
	var id int64
	if err = connection.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id); err != nil {
		return nil, fmt.Errorf("读取索引连接编号失败")
	}
	if err = saveConnection(id); err != nil {
		return nil, fmt.Errorf("执行线索未保存，未发送 DDL")
	}
	// Preview 在准备阶段由程序生成并与参数一起保存；人看到的就是这一条语句。
	if _, err = connection.ExecContext(ctx, plan.Preview); err != nil {
		return nil, fmt.Errorf("%s", databaseProblem(err))
	}
	return map[string]any{"index": plan.Index, "confirmed": true}, nil
}

// setPool 先看当前状态。同一审批已经成功时不再写；审批后重启或被别人修改则拒绝。
func (backend *Backend) setPool(ctx context.Context, record Record) (any, error) {
	plan := record.Plan
	var current PoolState
	if err := backend.opsRequest(ctx, plan.Service, "GET", "/ops/pool", nil, &current); err != nil {
		return nil, err
	}
	// 先识别“已经改好但回复丢了”，再检查是否被其他操作改变；顺序不能反过来。
	if current.LastOperation == record.ID && current.Current == plan.Size && current.Instance == plan.Before.Instance {
		return current, nil
	}
	if current.Instance != plan.Before.Instance || current.Current != plan.Before.Current || current.Revision != plan.Before.Revision {
		return nil, fmt.Errorf("审批后的进程或连接池已变化，未覆盖当前状态")
	}
	input := map[string]any{"operation": record.ID, "instance": plan.Before.Instance, "revision": plan.Before.Revision, "before": plan.Before.Current, "size": plan.Size}
	if err := backend.opsRequest(ctx, plan.Service, "POST", "/ops/pool", input, &current); err != nil {
		return nil, err
	}
	return current, nil
}
