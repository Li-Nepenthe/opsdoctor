// 本文件：按顺序完成下单，并在失败时判断能否安全重试。
//
// HTTP 请求先到这里。价格、库存、数据库和消息都通过小接口调用，测试可以逐步模拟失败。
// 跨服务操作没有共同事务，因此每个失败分支都要说明已经发生了什么。
//
// 主要内容：
//   - NewService：装配订单所需能力和收尾预算。
//   - Create：占位、查价、扣库存、写订单、发消息、保存回放结果。
//   - safeFailure：确认没有业务变更后才删除占位。
//   - unknown：保留占位，记录告警与待核对计数。
//   - Get、List：校验查询参数后读取订单。
//   - outcomeUnknown：用错误携带的信息判断写入结果是否确定。
//
// 阅读建议：先看 Create 的成功路径，再逐一看三个写操作失败后为什么处理不同。

package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// Repository 返回可能已分配编号的订单，即使提交答复丢失，日志也能保留这个线索。
type Repository interface {
	Create(context.Context, Order) (Order, error)
	Get(context.Context, int64) (Order, error)
	List(context.Context, int64, int, int) (Page, error)
}

// PriceReader 共用商品缓存，但不读取用于展示的库存状态。
type PriceReader interface {
	Get(context.Context, int64) (product.Product, error)
}

// StockWriter 只暴露订单需要的扣减和补偿，二者都不应自动重试。
type StockWriter interface {
	Deduct(context.Context, int64, int32) error
	Release(context.Context, int64, int32) error
}

// Publisher 同步告知消息发送结果。下单流程不会把发送失败误认为订单未创建。
type Publisher interface {
	Publish(context.Context, events.OrderCreated) error
}

// ReviewRecorder 同时写告警日志和计数。实现由 app 提供，不让业务流程绑定指标库。
type ReviewRecorder interface {
	RecordReview(context.Context, int64, int64, string, string)
}

// Service 只保存依赖和固定配置，每次下单的中间结果放在局部变量中。
type Service struct {
	repository     Repository       // 保存与查询订单。
	prices         PriceReader      // 商品价格来自缓存或数据库。
	stock          StockWriter      // 负责真实库存变化。
	publisher      Publisher        // 订单落库后发送消息。
	idempotency    IdempotencyStore // Redis 不可用时不能绕过占位。
	reviews        ReviewRecorder   // 记录结果待核对事件。
	cleanupTimeout time.Duration    // 请求取消后，补偿或保存结果仍有有限的收尾时间。
	logger         *slog.Logger     // 记录已落库但未发出消息的缺口。
}

// NewService 由 app 统一传入依赖，cleanupTimeout 必须大于库存调用超时。
func NewService(repository Repository, prices PriceReader, stock StockWriter, publisher Publisher, idempotency IdempotencyStore, reviews ReviewRecorder, cleanupTimeout time.Duration, logger *slog.Logger) *Service {
	return &Service{repository: repository, prices: prices, stock: stock, publisher: publisher, idempotency: idempotency, reviews: reviews, cleanupTimeout: cleanupTimeout, logger: logger}
}

// Create 遇到结果不明就停止自动处理，不能把“没有收到成功答复”当成“什么都没发生”。
func (service *Service) Create(ctx context.Context, request CreateRequest, key string) (Created, error) {
	if request.UserID <= 0 || request.SKUID <= 0 || request.Quantity <= 0 || !validKey(key) {
		return Created{}, ErrInvalidArgument
	}
	reservation := Reservation{Key: key, SKUID: request.SKUID}
	if key != "" {
		var owned bool
		var err error
		reservation, owned, err = service.idempotency.Reserve(ctx, key, request)
		if err != nil {
			return Created{}, err
		}
		if !owned {
			// 重复请求只检查记录，不再查价格、扣库存或发送消息。
			switch reservation.State {
			case stateCompleted:
				if reservation.Result == nil {
					return Created{}, ErrIdempotencyUnavailable
				}
				return *reservation.Result, nil
			case stateNeedsReview:
				if reservation.EnteredReview {
					service.reviews.RecordReview(ctx, reservation.OrderID, reservation.SKUID, key, "processing_budget_expired")
				}
				return Created{}, ErrOutcomeUnknown
			default:
				return Created{}, ErrInProgress
			}
		}
	}
	price, err := service.prices.Get(ctx, request.SKUID)
	if err != nil {
		return Created{}, service.safeFailure(ctx, reservation, err)
	}
	if price.PriceCents < 0 || price.PriceCents > math.MaxInt64/int64(request.Quantity) {
		return Created{}, service.safeFailure(ctx, reservation, ErrInvalidArgument)
	}
	if err := service.stock.Deduct(ctx, request.SKUID, request.Quantity); err != nil {
		if outcomeUnknown(err) {
			return Created{}, service.unknown(ctx, reservation, 0, "deduct_outcome_unknown")
		}
		return Created{}, service.safeFailure(ctx, reservation, err)
	}
	draft := Order{UserID: request.UserID, SKUID: request.SKUID, Quantity: request.Quantity, AmountCents: price.PriceCents * int64(request.Quantity), Status: "created", CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	saved, err := service.repository.Create(ctx, draft)
	if err != nil {
		if outcomeUnknown(err) {
			return Created{}, service.unknown(ctx, reservation, saved.OrderID, "order_commit_unknown")
		}
		// 已确认订单没提交才补偿。请求可能已经超时，所以收尾另给一份有限预算。
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
		releaseErr := service.stock.Release(cleanup, request.SKUID, request.Quantity)
		cancel()
		if releaseErr != nil {
			return Created{}, service.unknown(ctx, reservation, saved.OrderID, "compensation_failed")
		}
		return Created{}, service.safeFailure(ctx, reservation, err)
	}
	event := events.OrderCreated{OrderID: saved.OrderID, UserID: saved.UserID, SKUID: saved.SKUID, Quantity: saved.Quantity, AmountCents: saved.AmountCents, CreatedAt: saved.CreatedAt}
	// 订单已经提交，发送消息也是收尾工作，不能因为客户断开连接就放弃。
	// WithoutCancel 保留请求中的值（包括调用链），只去掉原来的取消信号和 deadline。
	// 再加自己的超时，避免 Kafka 不可用时一直占住处理请求的 goroutine。
	publishCtx, cancelPublish := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
	publishErr := service.publisher.Publish(publishCtx, event)
	cancelPublish()
	if publishErr != nil {
		// 订单已经存在，消息失败不能再当作下单失败重做。这一缺口留给人工核对，不实现 Outbox。
		service.logger.WarnContext(ctx, "订单已创建，但创建消息未确认发送", "order_id", saved.OrderID, "sku_id", saved.SKUID)
	}
	result := Created{OrderID: saved.OrderID, AmountCents: saved.AmountCents, Status: saved.Status}
	if key != "" {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
		err := service.idempotency.Complete(cleanup, reservation, result)
		cancel()
		if err != nil {
			return Created{}, service.unknown(ctx, reservation, saved.OrderID, "result_save_failed")
		}
	}
	return result, nil
}

// safeFailure 只接收已确认没有留下订单或库存变化的失败。删占位失败仍不能允许重做。
func (service *Service) safeFailure(ctx context.Context, reservation Reservation, cause error) error {
	if reservation.Key == "" {
		return cause
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
	err := service.idempotency.Abandon(cleanup, reservation)
	cancel()
	if err != nil {
		return service.unknown(ctx, reservation, 0, "reservation_delete_failed")
	}
	return cause
}

// unknown 尽力保存待核对状态。Redis 仍断开时也要告警，不能因为写状态失败就沉默。
func (service *Service) unknown(ctx context.Context, reservation Reservation, orderID int64, reason string) error {
	if reservation.Key != "" {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
		err := service.idempotency.Review(cleanup, reservation, orderID)
		cancel()
		if err != nil {
			service.logger.WarnContext(ctx, "待核对状态暂未写回，恢复后需检查旧占位", "order_id", orderID, "sku_id", reservation.SKUID)
		}
	}
	service.reviews.RecordReview(ctx, orderID, reservation.SKUID, reservation.Key, reason)
	return ErrOutcomeUnknown
}

// outcomeUnknown 遇到未提供确定性说明的写错误时保守处理，避免新适配器漏写说明就允许重复扣减。
func outcomeUnknown(err error) bool {
	var outcome interface{ OutcomeUnknown() bool }
	if errors.As(err, &outcome) {
		return outcome.OutcomeUnknown()
	}
	return true
}

// Get 在访问数据库前挡住无效编号。
func (service *Service) Get(ctx context.Context, id int64) (Order, error) {
	if id <= 0 {
		return Order{}, ErrInvalidArgument
	}
	return service.repository.Get(ctx, id)
}

// List 限制每页数量并检查偏移量乘法，避免整数溢出后查错页。
func (service *Service) List(ctx context.Context, userID int64, page, pageSize int) (Page, error) {
	if userID <= 0 || page <= 0 || pageSize <= 0 || pageSize > 100 || page-1 > math.MaxInt/pageSize {
		return Page{}, fmt.Errorf("分页参数：%w", ErrInvalidArgument)
	}
	return service.repository.List(ctx, userID, page, pageSize)
}
