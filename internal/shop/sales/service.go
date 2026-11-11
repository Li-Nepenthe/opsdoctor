// 本文件：订单事件通过业务校验后才进入销量事务，重复送达可以正常确认位点。
// 消费者调用 Handle，返回成功后才提交 Kafka 位点。

package sales

import (
	"context"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
)

// Repository 必须把去重与累计销量放在同一事务内。
// 成功时返回这笔订单是否已处理。
type Repository interface {
	Apply(context.Context, events.OrderCreated) (bool, error)
}

// Service 校验销量事件并交给仓储记账。
type Service struct{ repository Repository }

// NewService 使用消费者装配的销量仓储。
func NewService(repository Repository) *Service { return &Service{repository: repository} }

// Handle 的 error 为 nil 时，消费者才可以提交位点；bool 用来单独统计重复消息。
func (service *Service) Handle(ctx context.Context, event events.OrderCreated) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	return service.repository.Apply(ctx, event)
}
