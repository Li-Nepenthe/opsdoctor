// 本文件：串起商品缓存、数据库查询和库存展示。
//
// HTTP 详情和订单价格查询都先进入这里。具体 Redis、MySQL 和 gRPC 操作由接口隔开。
// Cache Aside 可以理解成“应用自己照看缓存”：先查缓存，没命中再查数据库并补回缓存。
// 下文的“回源”就是缓存没有拿到结果，转去查询数据库。
//
// 主要内容：
//   - Repository、Cache、StockReader：列出商品流程需要的外部能力。
//   - NewService：选择是否合并同时查询同一商品的请求。
//   - Get：按缓存优先的顺序读取商品。
//   - load：确认缓存仍未命中后，只向数据库查询一次。
//   - Detail：附加库存信息，库存失败时保留商品详情。
//
// 阅读建议：先读 Get 的命中路径，再读 DoChan 怎样让多个请求等待同一次查询。

package product

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"golang.org/x/sync/singleflight"
)

// Repository 由使用者定义，测试可以换成内存实现，不必真的连接 MySQL。
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

// Service 保存商品查询依赖。同一实例的 group 才能把相同商品的请求合并起来。
type Service struct {
	repository Repository         // 数据库是商品信息的最终来源。
	cache      Cache              // 缓存失败允许回到数据库，不能把故障误当商品不存在。
	stock      StockReader        // 库存失败只影响展示中的 stock 字段。
	merge      bool               // false 时每个未命中的请求都自行回源，用于对比防护效果。
	group      singleflight.Group // singleflight 让相同 key 的并发调用共享一个执行结果。
	logger     *slog.Logger       // 只记录故障分类，不把缓存内容写进日志。
}

// NewService 接收已经装配好的依赖，让测试和真实服务使用相同的业务流程。
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
	// DoChan 返回一个结果通道，等待者可以在自己的请求取消时先离开。
	// 实际查询仍使用第一个请求的 context。它取消时，这一批等待者都会收到查询失败。
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
