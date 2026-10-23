// 本文件：给库存 gRPC 调用加上时间限制和故障保护。
//
// product 和 order 的业务流程通过这里访问库存。生成的 protobuf 类型不再向业务层传递。
// 把客户端单独放置，是为了让两个业务包共用同一套保护规则。
//
// 主要内容：
//   - Config、New：检查超时配置，分别创建查询和扣减的熔断器。
//   - GetStock：在总预算内重试查询，把结果转换成库存快照。
//   - Deduct、Release：只发送一次写请求，避免重复扣减或归还。
//   - convertError：把 RPC 状态转换成业务能理解的错误。
//   - WriteError：保留“这次写操作是否可能已生效”的信息。
//
// 阅读建议：先比较三个公开方法，再看哪些错误算下游故障。

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
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config 由 app 从环境变量读取。客户端只接收检查过的值。
type Config struct {
	Timeout time.Duration            // 一次逻辑调用的总预算，包含查询重试和等待。
	Retry   resilience.RetryPolicy   // 仅 GetStock 使用这份策略。
	Breaker resilience.BreakerConfig // 两个熔断器用相同配置，但各自计数。
}

// Client 共用一条 gRPC 连接。每次调用的结果都存在局部变量里，不会互相覆盖。
type Client struct {
	rpc           inventoryv1.InventoryServiceClient // app 创建并管理底层连接。
	timeout       time.Duration                      // 每次调用开始时，用它缩短调用方的预算。
	retry         *resilience.Retrier                // 查询失败后决定是否再试。
	getBreaker    *resilience.Breaker                // 查询接口自己的失败记录。
	deductBreaker *resilience.Breaker                // 扣减接口自己的失败记录。
}

// WriteError 表示写请求的错误。连接断开不等于服务端没有执行，因此需要额外保存结果是否确定。
type WriteError struct {
	Cause          error // Cause 用于 errors.Is 继续匹配超时或业务错误。
	MayHaveApplied bool  // true 表示不能根据这次响应决定是否重新扣减。
}

// Error 提供可读的错误文本，底层原因仍通过 Unwrap 保留。
func (err *WriteError) Error() string { return err.Cause.Error() }

// Unwrap 让 errors.Is 和 errors.As 可以继续查看包裹的原因。
func (err *WriteError) Unwrap() error { return err.Cause }

// OutcomeUnknown 让业务层通过一个小接口识别结果不确定，不必导入本客户端的具体错误类型。
func (err *WriteError) OutcomeUnknown() bool { return err.MayHaveApplied }

// New 提前检查配置，避免服务已经接流量后才发现保护参数不合法。
func New(rpc inventoryv1.InventoryServiceClient, config Config, logger *slog.Logger) (*Client, error) {
	if rpc == nil || config.Timeout <= 0 || config.Retry.MaxRetries > 2 {
		return nil, fmt.Errorf("库存客户端需要有效 RPC、正超时，查询最多重试两次")
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
	return &Client{rpc: rpc, timeout: config.Timeout, retry: retry, getBreaker: get, deductBreaker: deduct}, nil
}

// GetStock 的所有尝试共享一个截止时间，不能每次重试都重新获得完整预算。
func (client *Client) GetStock(parent context.Context, skuID int64) (inventory.Stock, error) {
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	var response *inventoryv1.GetStockResponse
	err := client.retry.Do(ctx, func() error {
		return client.getBreaker.Execute(func() error {
			var err error
			response, err = client.rpc.GetStock(ctx, &inventoryv1.GetStockRequest{SkuId: skuID})
			return err
		})
	}, func(err error) bool {
		// 熔断器已经打开时，等待后重试也不会发出请求，直接把失败交给商品降级。
		return ctx.Err() == nil && (status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded)
	})
	if err != nil {
		return inventory.Stock{}, convertError(err)
	}
	return inventory.Stock{SKUID: skuID, Available: response.GetAvailable()}, nil
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
