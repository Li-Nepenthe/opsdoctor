// 本文件：订单与商品服务在这里调用库存 RPC。读请求共享总时限，扣减和归还各只发送一次。
// 从 GetStock 对照 Deduct，可以看到查询重试和写入结果不确定的处理为何不同。

package inventoryclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config 由 app 从环境变量读取。客户端只接收检查过的值。
type Config struct {
	Timeout        time.Duration            // 一次逻辑调用的总预算，包含查询重试和等待。
	AttemptTimeout time.Duration            // 每次查询尝试的预算；0 沿用总截止时间，写请求不使用它。
	Retry          resilience.RetryPolicy   // 仅 GetStock 使用这份策略。
	Breaker        resilience.BreakerConfig // 两个熔断器用相同配置，但各自计数。
}

// Client 在同一连接上分别记录查询和扣减的技术故障。
type Client struct {
	rpc            inventoryv1.InventoryServiceClient
	timeout        time.Duration
	attemptTimeout time.Duration
	retry          *resilience.Retrier
	getBreaker     *resilience.Breaker
	deductBreaker  *resilience.Breaker
	retryCounter   prometheus.Counter
}

// WriteError 表示写请求的错误。连接断开不等于服务端没有执行，因此需要额外保存结果是否确定。
type WriteError struct {
	Cause          error
	MayHaveApplied bool // true 表示不能根据这次响应决定是否重新扣减。
}

// Error 返回本次库存写入的失败原因。
func (err *WriteError) Error() string { return err.Cause.Error() }

// Unwrap 保留底层错误的身份，供调用方区分超时与业务拒绝。
func (err *WriteError) Unwrap() error { return err.Cause }

// OutcomeUnknown 告知调用方库存可能已经改变。
func (err *WriteError) OutcomeUnknown() bool { return err.MayHaveApplied }

// New 提前检查配置，避免服务已经接流量后才发现保护参数不合法。
func New(rpc inventoryv1.InventoryServiceClient, config Config, logger *slog.Logger) (*Client, error) {
	if rpc == nil || config.Timeout <= 0 || config.AttemptTimeout < 0 || config.AttemptTimeout > config.Timeout {
		return nil, fmt.Errorf("库存客户端需要有效 RPC、正总时限，单次查询时限须在零与总时限之间")
	}
	retry, err := resilience.NewRetrier(config.Retry)
	if err != nil {
		return nil, err
	}
	// 库存不足属于正常业务结果。把它算成服务故障，会让其他商品也无法下单。
	get, err := resilience.NewBreaker("GetStock", config.Breaker, logger, healthyResult, canceledResult)
	if err != nil {
		return nil, err
	}
	deduct, err := resilience.NewBreaker("Deduct", config.Breaker, logger, healthyResult, canceledResult)
	if err != nil {
		return nil, err
	}
	return &Client{rpc: rpc, timeout: config.Timeout, attemptTimeout: config.AttemptTimeout, retry: retry, getBreaker: get, deductBreaker: deduct}, nil
}

// GetStock 的所有尝试共享一个截止时间，不能每次重试都重新获得完整预算。
func (client *Client) GetStock(parent context.Context, skuID int64) (inventory.Stock, error) {
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	var response *inventoryv1.GetStockResponse
	attempts := 0
	err := client.retry.Do(ctx, func() error {
		return client.getBreaker.Execute(func() error {
			// 放在熔断器内部，只有实际向库存发送第二次及之后的请求才算重试。
			if attempts > 0 && client.retryCounter != nil {
				client.retryCounter.Inc()
			}
			attempts++
			var err error
			response, err = client.queryAttempt(ctx, skuID)
			return err
		})
	}, retryableStockErrors(ctx))
	if err != nil {
		return inventory.Stock{}, convertError(err)
	}
	return inventory.Stock{SKUID: skuID, Available: response.GetAvailable()}, nil
}

// retryableStockErrors 只允许总预算内的暂时故障重试。熔断器直接拒绝时不会再发请求。
func retryableStockErrors(ctx context.Context) func(error) bool {
	return func(err error) bool {
		return ctx.Err() == nil && (status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded)
	}
}

// queryAttempt 给一次查询分配时间。父 context 仍限制整次调用，短预算到期不会取消其他尝试。
func (client *Client) queryAttempt(ctx context.Context, skuID int64) (*inventoryv1.GetStockResponse, error) {
	if client.attemptTimeout == 0 {
		// 没有单次预算时，查询直接使用整次调用的截止时间。
		return client.rpc.GetStock(ctx, &inventoryv1.GetStockRequest{SkuId: skuID})
	}
	// 本次尝试还受整次查询剩余时间限制。
	attempt, cancel := context.WithTimeout(ctx, client.attemptTimeout)
	defer cancel()
	return client.rpc.GetStock(attempt, &inventoryv1.GetStockRequest{SkuId: skuID})
}

// Deduct 一次只发一条 RPC。响应丢失时可能已经扣成功，不能擅自补发。
func (client *Client) Deduct(parent context.Context, skuID int64, quantity int32) error {
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return &WriteError{Cause: err}
	}
	called := false
	err := client.deductBreaker.Execute(func() error {
		called = true
		_, err := client.rpc.Deduct(ctx, &inventoryv1.DeductRequest{SkuId: skuID, Quantity: quantity})
		return err
	})
	if err == nil {
		return nil
	}
	// 未进入回调就被熔断器拒绝，可以确定本次没有扣库存。
	return &WriteError{Cause: convertError(err), MayHaveApplied: called && !healthyResult(err)}
}

// Release 不受扣减熔断器阻挡，否则已经占用的库存可能连归还机会都没有。
func (client *Client) Release(parent context.Context, skuID int64, quantity int32) error {
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return &WriteError{Cause: err}
	}
	_, err := client.rpc.Release(ctx, &inventoryv1.ReleaseRequest{SkuId: skuID, Quantity: quantity})
	if err == nil {
		return nil
	}
	return &WriteError{Cause: convertError(err), MayHaveApplied: !healthyResult(err)}
}

// healthyResult 把已明确答复的业务拒绝算作服务正常，只有技术故障才增加失败数。
func healthyResult(err error) bool {
	switch status.Code(err) {
	case codes.OK, codes.InvalidArgument, codes.NotFound, codes.FailedPrecondition:
		return true
	default:
		return false
	}
}

// canceledResult 排除调用方主动放弃的请求，它不能说明下游是否健康。
func canceledResult(err error) bool {
	return errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled
}

// convertError 不把 gRPC 类型泄露到业务分支中，HTTP 层只需判断领域错误。
func convertError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return inventory.ErrInvalidArgument
	case codes.NotFound:
		return inventory.ErrNotFound
	case codes.FailedPrecondition:
		return inventory.ErrInsufficientStock
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	}
	// 打开和半开额度耗尽都表示暂时无法调用。不把第三方库错误带进响应。
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return inventory.ErrUnavailable
	}
	return fmt.Errorf("调用库存服务：%w", inventory.ErrUnavailable)
}
