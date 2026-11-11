// 本文件：按完整或 small 规模分批造数，返回实际提交的进度并保留已有库存。
// 从 shopctl 调用的 Run 可以看到校验现有数据、续跑和跳过的条件。

package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 完整数据由 50 万商品、每商品四单组成；small 只缩小规模。
const (
	fullProducts     = 500_000
	smallProducts    = 100
	ordersPerProduct = 4
	batchProducts    = 1_000 // 每批最多 1000 商品和 4000 订单，控制参数个数、事务大小与内存占用。
	initialStock     = 100   // 可售余额独立于历史销量；四件历史订单已经由此前库存售出。
	priceCents       = 1999  // 固定 19.99 元，让用户能手算订单金额。
)

// Result 用于报告种子基线总量与本次实际提交的进度。
type Result struct {
	Products         int64
	Orders           int64
	InsertedProducts int64 // 只统计本次成功提交的商品。
	Skipped          bool
	Duration         time.Duration // 包含基线检查及写入，不含连接和迁移。
}

// Run 从已提交的商品编号继续，返回实际写入的进度。
// 已有五表数据不构成完整样本时停止，避免覆盖已发生的业务操作。
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
