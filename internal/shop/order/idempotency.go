// 本文件：描述一个幂等键从占位到完成或待核对的状态。
//
// 幂等的意思是“同一请求重复发送，也只产生一次业务效果”。本阶段的保证有保存时限。
// service 只使用这里的接口，Redis 如何原子更新由 cache.go 实现。
//
// 主要内容：
//   - Reservation：保存请求所有者、状态和已创建结果。
//   - IdempotencyStore：占位、完成、删除安全占位、进入待核对四个动作。
//   - validKey：检查公开契约允许的幂等键格式。
//
// 阅读建议：Owner 与客户端的 Key 不同。Owner 是服务端随机生成的，用来防止旧请求误改新记录。

package order

import "context"

// 三种状态只描述幂等记录，不改变订单表的业务状态。
const (
	stateProcessing  = "processing"   // 仍在预算内执行，重复请求稍后再查询。
	stateCompleted   = "completed"    // 已保存结果，可以直接回放。
	stateNeedsReview = "needs_review" // 结果不确定，不允许重新执行业务。
)

// Reservation 保存为 JSON。raw 和 EnteredReview 只用于本次调用，不写进 Redis。
type Reservation struct {
	Key           string   `json:"-"`                // 客户端幂等键，用来定位 Redis key。
	Owner         string   `json:"owner"`            // 服务端随机编号，避免同名键过期后被旧请求修改。
	Fingerprint   string   `json:"fingerprint"`      // 请求参数摘要，防止同一键被拿来提交另一个订单。
	State         string   `json:"state"`            // 处理中、完成、待核对三选一。
	SKUID         int64    `json:"sku_id"`           // 待核对日志需要的商品编号。
	OrderID       int64    `json:"order_id"`         // 未拿到数据库编号前为 0。
	ReviewAfter   int64    `json:"review_after_ms"`  // 超过这个时间的处理中记录不能继续声称仍在正常执行。
	Result        *Created `json:"result,omitempty"` // 只有完成状态有可回放结果。
	EnteredReview bool     `json:"-"`                // 本次读取是否把过期处理记录转成待核对。
	raw           string   // 读取时的完整 JSON，用于比较后再更新，避免覆盖他人的记录。
}

// IdempotencyStore 的 bool 返回值说明是否由本次调用取得处理权，不能仅凭查不到就开始下单。
type IdempotencyStore interface {
	Reserve(context.Context, string, CreateRequest) (Reservation, bool, error)
	Complete(context.Context, Reservation, Created) error
	Abandon(context.Context, Reservation) error
	Review(context.Context, Reservation, int64) error
}

// validKey 只接受契约列出的 ASCII 字符。空字符串表示客户端没有提供幂等键。
func validKey(key string) bool {
	if len(key) > 64 {
		return false
	}
	for _, char := range key {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}
