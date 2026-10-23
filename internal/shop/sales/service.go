// 本文件：在处理订单消息之前验证业务字段。
//
// Kafka 消费者调用 Handle，再由仓储执行事务。把校验放在这里，其他调用者也不会绕过它。
//
// 主要内容：
//   - Repository：销量流程需要的存储能力。
//   - Service、NewService：接收仓储依赖。
//   - Handle：拒绝无效消息，再执行去重与累加。
//
// 阅读建议：重复消息并不算错误，仓储会返回成功，让消费者能够提交位点。

package sales

import (
	"context"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
)

// Repository 的 Apply 必须把去重与加销量作为一件事完成，不能只成功一半。
type Repository interface {
	Apply(context.Context, events.OrderCreated) error
}

// Service 只持有仓储，不直接认识 Kafka 或数据库驱动。
type Service struct{ repository Repository } // repository 负责事务边界。

// NewService 由 app 把真实仓储传入，测试可以传入替身。
func NewService(repository Repository) *Service { return &Service{repository: repository} }

// Handle 只有在数据库处理成功后才返回 nil，消费者据此决定是否提交位点。
func (service *Service) Handle(ctx context.Context, event events.OrderCreated) error {
	if err := event.Validate(); err != nil {
		return err
	}
	return service.repository.Apply(ctx, event)
}
