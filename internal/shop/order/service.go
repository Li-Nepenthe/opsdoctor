// 本文件：协调从幂等占位到保存回放结果的六步下单流程。
// Create 通过各自的接口查价、扣库存、写订单和发消息；能否归还库存或允许重试，取决于失败时已经完成了什么。

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

// Service 协调订单写入、库存变更和幂等占位，跨服务的失败由它决定能否补偿。
type Service struct {
	repository     Repository
	prices         PriceReader
	stock          StockWriter
	publisher      Publisher
	idempotency    IdempotencyStore
	reviews        ReviewRecorder
	cleanupTimeout time.Duration
	publishTimeout time.Duration
	logger         *slog.Logger
}

// Dependencies 由订单服务装配，同一进程的请求复用价格、库存和消息客户端。
type Dependencies struct {
	Repository     Repository
	Prices         PriceReader
	Stock          StockWriter
	Publisher      Publisher
	Idempotency    IdempotencyStore
	Reviews        ReviewRecorder
	CleanupTimeout time.Duration // 补偿预算必须大于库存调用超时。
	PublishTimeout time.Duration // Kafka 的发送确认有独立的时间预算。
	Logger         *slog.Logger
}

// NewService 用同一组依赖处理下单和订单查询。
func NewService(deps Dependencies) *Service {
	return &Service{
		repository:     deps.Repository,
		prices:         deps.Prices,
		stock:          deps.Stock,
		publisher:      deps.Publisher,
		idempotency:    deps.Idempotency,
		reviews:        deps.Reviews,
		cleanupTimeout: deps.CleanupTimeout,
		publishTimeout: deps.PublishTimeout,
		logger:         deps.Logger,
	}
}

// Create 遇到结果不明就停止自动处理，不能把“没有收到成功答复”当成“什么都没发生”。
func (service *Service) Create(ctx context.Context, request CreateRequest, key string) (Created, error) {
	if request.UserID <= 0 || request.SKUID <= 0 || request.Quantity <= 0 || !validKey(key) {
		return Created{}, ErrInvalidArgument
	}
	reservation, owned, err := service.reserve(ctx, request, key)
	if err != nil {
		return Created{}, err
	}
	if !owned {
		return service.replay(ctx, reservation, key)
	}
	priceCents, err := service.orderPrice(ctx, request)
	if err != nil {
		return Created{}, service.safeFailure(ctx, reservation, err)
	}
	if err := service.deductStock(ctx, request, reservation); err != nil {
		return Created{}, err
	}
	saved, err := service.writeOrder(ctx, request, priceCents, reservation)
	if err != nil {
		return Created{}, err
	}
	service.publishCreated(ctx, saved)
	return service.saveResult(ctx, key, reservation, saved)
}

func (service *Service) reserve(ctx context.Context, request CreateRequest, key string) (Reservation, bool, error) {
	if key == "" {
		return Reservation{Key: key, SKUID: request.SKUID}, true, nil
	}
	return service.idempotency.Reserve(ctx, key, request)
}

func (service *Service) replay(ctx context.Context, reservation Reservation, key string) (Created, error) {
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

func (service *Service) orderPrice(ctx context.Context, request CreateRequest) (int64, error) {
	price, err := service.prices.Get(ctx, request.SKUID)
	if err != nil {
		return 0, err
	}
	if price.PriceCents < 0 || price.PriceCents > math.MaxInt64/int64(request.Quantity) {
		return 0, ErrInvalidArgument
	}
	return price.PriceCents, nil
}

func (service *Service) deductStock(ctx context.Context, request CreateRequest, reservation Reservation) error {
	if err := service.stock.Deduct(ctx, request.SKUID, request.Quantity); err != nil {
		if outcomeUnknown(err) {
			return service.unknown(ctx, reservation, 0, "deduct_outcome_unknown")
		}
		return service.safeFailure(ctx, reservation, err)
	}
	return nil
}

func (service *Service) writeOrder(ctx context.Context, request CreateRequest, priceCents int64, reservation Reservation) (Order, error) {
	draft := Order{
		UserID: request.UserID, SKUID: request.SKUID, Quantity: request.Quantity,
		AmountCents: priceCents * int64(request.Quantity), Status: "created",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	saved, err := service.repository.Create(ctx, draft)
	if err != nil {
		if outcomeUnknown(err) {
			return Order{}, service.unknown(ctx, reservation, saved.OrderID, "order_commit_unknown")
		}
		// 已确认订单没提交才补偿。请求可能已经超时，所以收尾另给一份有限预算。
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.cleanupTimeout)
		releaseErr := service.stock.Release(cleanup, request.SKUID, request.Quantity)
		cancel()
		if releaseErr != nil {
			return Order{}, service.unknown(ctx, reservation, saved.OrderID, "compensation_failed")
		}
		return Order{}, service.safeFailure(ctx, reservation, err)
	}
	return saved, nil
}

func (service *Service) publishCreated(ctx context.Context, saved Order) {
	event := events.OrderCreated{
		OrderID: saved.OrderID, UserID: saved.UserID, SKUID: saved.SKUID,
		Quantity: saved.Quantity, AmountCents: saved.AmountCents, CreatedAt: saved.CreatedAt,
	}
	// 订单已经提交，客户断开也要等发送确认。Kafka 一次写入最多三秒，因此它有独立于 Redis 收尾的有限预算。
	publishCtx, cancelPublish := context.WithTimeout(context.WithoutCancel(ctx), service.publishTimeout)
	publishStarted := time.Now()
	publishErr := service.publisher.Publish(publishCtx, event)
	cancelPublish()
	if publishErr != nil {
		// 订单已经存在，消息失败不能再当作下单失败重做。这一缺口留给人工核对，不实现 Outbox。
		service.logger.WarnContext(ctx, "订单已创建，但创建消息未确认发送", "order_id", saved.OrderID, "sku_id", saved.SKUID,
			"error_kind", publishErrorKind(publishErr), "publish_elapsed_ms", time.Since(publishStarted).Milliseconds())
	}
}

func (service *Service) saveResult(ctx context.Context, key string, reservation Reservation, saved Order) (Created, error) {
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

// publishErrorKind 记录可用于排查的错误分类，而不把第三方错误中的连接地址或凭据直接写进日志。
func publishErrorKind(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "transport_or_broker"
}
