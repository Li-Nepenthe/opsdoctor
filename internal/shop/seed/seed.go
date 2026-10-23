// 本文件：控制完整规模与 small 模式的造数进度。
//
// shopctl 调用 Run。它先确认已有数据，再按商品范围逐批提交，返回真实计数与耗时。
// 先读目标规模常量和 Result，再看跳过、续跑与失败时如何报告已提交进度。SQL 细节放在 repository.go。
//
// 主要内容：
//   - Run：在已迁移的数据库中建立基线。ctx 控制所有查询与每批事务，返回已经提交的计数和错误。

package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 数据规模来自阶段 1 要求。每商品四单恰好组成 200 万订单，small 仅缩小规模不改变算法。
const (
	fullProducts     = 500_000
	smallProducts    = 100
	ordersPerProduct = 4
	batchProducts    = 1_000 // 每批最多 1000 商品和 4000 订单，控制参数个数、事务大小与内存占用。
	initialStock     = 100   // 可售余额独立于历史销量；四件历史订单已经由此前库存售出。
	priceCents       = 1999  // 固定 19.99 元，让用户能手算订单金额。
)

// Result 只包含计数与耗时，便于 CLI 打印真实结果。不包含数据库配置。
type Result struct {
	Products         int64         // Products 是运行后基线中的商品数，库存和销量行数与它相同。
	Orders           int64         // Orders 是历史订单数，每条数量为 1。
	InsertedProducts int64         // InsertedProducts 只统计本次新提交的商品，重跑完整基线时为 0。
	Skipped          bool          // Skipped 表示目标数据已存在，没有重置库存或覆盖任何行。
	Duration         time.Duration // Duration 包含检查现有数据和写入，不含 CLI 启动、连接与迁移。
}

// Run 在已迁移的数据库中建立基线。ctx 控制所有查询与每批事务，返回已经提交的计数和错误。
// 中断后允许从完整批次继续，但会拒绝行数不一致的库，避免把业务数据误当成可覆盖的样本。
func Run(ctx context.Context, database *sql.DB, small bool) (Result, error) {
	started := time.Now()
	existing, err := inspect(ctx, database)
	if err != nil {
		return Result{}, err
	}
	target := int64(fullProducts)
	if small {
		target = smallProducts
	}
	result := Result{Products: existing, Orders: existing * ordersPerProduct}
	if existing >= target {
		// 检测到完整基线时只报告并跳过，尤其不能把已经扣减的 available 重置为 100。
		result.Skipped = true
		result.Duration = time.Since(started)
		return result, nil
	}
	for first := existing + 1; first <= target; first += batchProducts {
		last := min(first+batchProducts-1, target)
		if err := insertBatch(ctx, database, first, last); err != nil {
			result.Duration = time.Since(started)
			return result, fmt.Errorf("生成商品 %d..%d：%w", first, last, err)
		}
		// 只有事务提交成功才累加进度。失败批次不会被当作已完成。
		result.InsertedProducts += last - first + 1
		result.Products = last
		result.Orders = last * ordersPerProduct
	}
	result.Duration = time.Since(started)
	return result, nil
}
