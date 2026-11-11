// 本文件：商品和价格优先从缓存读取。未命中时回源 MySQL，库存查询失败只影响详情中的库存展示。
// 从 Get 到 load 可以看到同一商品的并发查询如何共用一次回源。

package product

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"golang.org/x/sync/singleflight"
)

// Repository 从商品数据源读取价格和名称。
type Repository interface {
	Get(context.Context, int64) (Product, error)
}

// Cache 的 hit 表示缓存里有记录。空值缓存也算命中，此时 err 为 ErrNotFound。
type Cache interface {
	Get(context.Context, int64) (value Product, hit bool, err error)
	Set(context.Context, int64, Product, bool) error
}

// StockReader 只暴露查库存，商品详情不需要扣减能力。
type StockReader interface {
	GetStock(context.Context, int64) (inventory.Stock, error)
}

// Service 在同一进程内合并同一商品的并发回源。
type Service struct {
	repository Repository
	cache      Cache
	stock      StockReader
	merge      bool // 关闭后，每个未命中的请求都自行回源。
	group      singleflight.Group
	logger     *slog.Logger
}

// NewService 设置商品查询的依赖与回源合并策略。
func NewService(repository Repository, cache Cache, stock StockReader, merge bool, logger *slog.Logger) *Service {
	return &Service{repository: repository, cache: cache, stock: stock, merge: merge, logger: logger}
}

// Get 合并的是一次回源过程。缓存本身保存结果，singleflight 不负责长期保存商品。
func (service *Service) Get(ctx context.Context, id int64) (Product, error) {
	if id <= 0 {
		return Product{}, ErrInvalidArgument
	}
	if value, hit, err := service.cache.Get(ctx, id); hit {
		return value, err
	}
	if !service.merge {
		return service.load(ctx, id)
	}
	// 等待者可以随自己的请求取消而离开。实际查询沿用第一个请求的 context，
	// 它取消时，其他共享这次查询的等待者也会收到失败。
	result := service.group.DoChan(strconv.FormatInt(id, 10), func() (any, error) {
		return service.load(ctx, id)
	})
	select {
	case <-ctx.Done():
		return Product{}, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return Product{}, value.Err
		}
		return value.Val.(Product), nil
	}
}

// load 在真正回源前再查缓存，处理“刚准备回源，前一个请求已经写好缓存”的情况。
func (service *Service) load(ctx context.Context, id int64) (Product, error) {
	if value, hit, err := service.cache.Get(ctx, id); hit {
		return value, err
	}
	value, err := service.repository.Get(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		// 数据库连接失败不能缓存为空，否则数据库恢复后仍会错误地返回 404。
		return Product{}, err
	}
	if cacheErr := service.cache.Set(ctx, id, value, errors.Is(err, ErrNotFound)); cacheErr != nil {
		service.logger.WarnContext(ctx, "商品查询完成，但缓存未写入", "sku_id", id)
	}
	return value, err
}

// Detail 允许库存展示降级。下单不会走这个方法，因此不会把未知库存当成可售库存。
func (service *Service) Detail(ctx context.Context, id int64) (Detail, error) {
	value, err := service.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	detail := Detail{Product: value, Stock: StockView{Status: "unavailable"}}
	stock, err := service.stock.GetStock(ctx, id)
	if err != nil {
		service.logger.WarnContext(ctx, "商品详情暂时无法展示库存", "sku_id", id)
		return detail, nil
	}
	detail.Stock = StockView{Status: "ok", Available: &stock.Available}
	return detail, nil
}
