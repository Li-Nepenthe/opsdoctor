// 本文件：库存请求进入仓储前的业务规则。
//
// GRPCServer 调用 Service，Service 只认识它声明的 StockRepository 接口；测试因此可以换成假仓储。
// 先读接口的原子扣减要求，再看 validate；本层不能先查数量再决定扣减。

package inventory

import (
	"context"
	"fmt"
)

// StockRepository 在使用方声明所需能力，类似 Java 接口，但实现类不用写 implements。
// 实现必须支持并发，并把 ctx 传给实际 IO；Deduct 必须原子判断和扣减，不能先查再改。
type StockRepository interface {
	GetStock(context.Context, int64) (Stock, error)
	Deduct(context.Context, int64, int32) error
	Release(context.Context, int64, int32) error
}

// Service 保存库存规则和只读依赖；构造后没有可变状态，可被多个 RPC 同时调用。
type Service struct {
	repository StockRepository // repository 在 app 中注入；测试用假实现，无需启动 MySQL。
}

// NewService 让 app 把仓储交给业务层；测试可以在同一位置传假仓储，而不必改业务规则。
func NewService(repository StockRepository) *Service {
	return &Service{repository: repository}
}

// GetStock 校验商品 ID 后读取快照；取消或超时不再进入仓储，结果不代表已预留库存。
func (service *Service) GetStock(ctx context.Context, skuID int64) (Stock, error) {
	if err := validate(ctx, skuID, 1); err != nil {
		return Stock{}, err
	}
	return service.repository.GetStock(ctx, skuID)
}

// Deduct 只接受正数；并发安全由仓储的一条条件 UPDATE 保证，不在这里做先查后改。
func (service *Service) Deduct(ctx context.Context, skuID int64, quantity int32) error {
	if err := validate(ctx, skuID, quantity); err != nil {
		return err
	}
	return service.repository.Deduct(ctx, skuID, quantity)
}

// Release 归还正数件库存；没有幂等键，调用方不能因响应丢失就盲目重试。
func (service *Service) Release(ctx context.Context, skuID int64, quantity int32) error {
	if err := validate(ctx, skuID, quantity); err != nil {
		return err
	}
	return service.repository.Release(ctx, skuID, quantity)
}

// validate 把取消检查和参数规则放在一起，防止三个入口对非法输入的处理不一致。
func validate(ctx context.Context, skuID int64, quantity int32) error {
	if err := ctx.Err(); err != nil {
		// 请求已经结束时不必继续工作；%w 保留错误身份，让适配层识别超时和主动取消。
		return fmt.Errorf("库存请求已经结束：%w", err)
	}
	if skuID <= 0 || quantity <= 0 {
		return fmt.Errorf("sku_id 和 quantity 必须大于 0：%w", ErrInvalidArgument)
	}
	return nil
}
