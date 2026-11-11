// 本文件：库存服务校验请求后才交给仓储。并发扣减的判断和更新必须同时生效。
// 从 Deduct 进入仓储前可以看到，本层只检查参数和取消状态。

package inventory

import (
	"context"
	"fmt"
)

// StockRepository 提供库存查询与原子扣减、归还。
// 实现必须支持并发，并把 ctx 传给实际 IO。Deduct 必须原子判断和扣减，不能先查再改。
type StockRepository interface {
	GetStock(context.Context, int64) (Stock, error)
	Deduct(context.Context, int64, int32) error
	Release(context.Context, int64, int32) error
}

// Service 可由并发的库存请求共用。
type Service struct {
	repository StockRepository
}

// NewService 使用库存服务装配的仓储。
func NewService(repository StockRepository) *Service {
	return &Service{repository: repository}
}

// GetStock 校验商品 ID 后读取快照。取消或超时不再进入仓储，结果不代表已预留库存。
func (service *Service) GetStock(ctx context.Context, skuID int64) (Stock, error) {
	if err := validate(ctx, skuID, 1); err != nil {
		return Stock{}, err
	}
	return service.repository.GetStock(ctx, skuID)
}

// Deduct 只接受正数。并发安全由仓储的一条条件 UPDATE 保证，不在这里做先查后改。
func (service *Service) Deduct(ctx context.Context, skuID int64, quantity int32) error {
	if err := validate(ctx, skuID, quantity); err != nil {
		return err
	}
	return service.repository.Deduct(ctx, skuID, quantity)
}

// Release 归还正数件库存。没有幂等键，调用方不能因响应丢失就盲目重试。
func (service *Service) Release(ctx context.Context, skuID int64, quantity int32) error {
	if err := validate(ctx, skuID, quantity); err != nil {
		return err
	}
	return service.repository.Release(ctx, skuID, quantity)
}

// validate 优先处理请求取消，避免继续访问仓储。
func validate(ctx context.Context, skuID int64, quantity int32) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("库存请求已经结束：%w", err)
	}
	if skuID <= 0 || quantity <= 0 {
		return fmt.Errorf("sku_id 和 quantity 必须大于 0：%w", ErrInvalidArgument)
	}
	return nil
}
