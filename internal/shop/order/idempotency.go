// 本文件：约定一个幂等键的状态，以及下单流程可以怎样改变它。
// 客户端的键标识一次下单，服务端生成的 Owner 区分这个键过期前后的两次占位。

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
	Key           string   `json:"-"`           // 客户端幂等键，用来定位 Redis key。
	Owner         string   `json:"owner"`       // 服务端随机编号，避免同名键过期后被旧请求修改。
	Fingerprint   string   `json:"fingerprint"` // 请求参数摘要，防止同一键被拿来提交另一个订单。
	State         string   `json:"state"`
	SKUID         int64    `json:"sku_id"`           // 待核对日志需要的商品编号。
	OrderID       int64    `json:"order_id"`         // 未拿到数据库编号前为 0。
	ReviewAfter   int64    `json:"review_after_ms"`  // Unix 毫秒时间戳，此后仍在处理中的占位须转为待核对。
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
