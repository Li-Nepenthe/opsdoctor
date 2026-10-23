// 本文件：定义订单请求、订单结果和业务错误。
//
// HTTP 和数据库分别使用这些值，不让传输或驱动类型混进下单判断。
//
// 主要内容：
//   - CreateRequest、Created：下单输入与创建结果。
//   - Order、Page：订单详情和分页列表。
//   - Error：HTTP 层需要区分的错误分类。
//   - PersistenceError：说明数据库提交是否可能已经成功。
//
// 阅读建议：Created 是回放给重复请求的结果，Order 则包含完整查询字段。

package order

import "time"

// CreateRequest 只接受正整数，数量范围与库存 RPC 的 int32 一致。
type CreateRequest struct {
	UserID   int64 `json:"user_id"`  // 靶场直接使用此编号，本阶段不做登录校验。
	SKUID    int64 `json:"sku_id"`   // 一个商品一个 SKU，等于 products.id。
	Quantity int32 `json:"quantity"` // 购买件数，不允许零和负数。
}

// Created 保存在幂等记录中，同一键重放时继续使用这份结果。
type Created struct {
	OrderID     int64  `json:"order_id"`     // 数据库生成的订单主键。
	AmountCents int64  `json:"amount_cents"` // 单价乘件数，单位是分。
	Status      string `json:"status"`       // 本阶段创建后统一为 created。
}

// Order 是数据库中的完整订单快照，不直接暴露 sql.Rows 等驱动类型。
type Order struct {
	OrderID     int64     `json:"order_id"`     // 自增主键，供详情查询和消息去重。
	UserID      int64     `json:"user_id"`      // 用户列表查询的索引前缀。
	SKUID       int64     `json:"sku_id"`       // 这笔订单购买的商品。
	Quantity    int32     `json:"quantity"`     // 购买件数。
	AmountCents int64     `json:"amount_cents"` // 下单时的价格快照，后续商品改价不改变它。
	Status      string    `json:"status"`       // 当前只存在 created 状态。
	CreatedAt   time.Time `json:"created_at"`   // 使用 UTC，响应按 RFC 3339 编码。
}

// Page 的 Items 在没有结果时也返回空数组，不返回 JSON null。
type Page struct {
	Items    []Order `json:"items"`     // 当前页的订单，按创建时间与编号倒序。
	Page     int     `json:"page"`      // 从 1 开始。
	PageSize int     `json:"page_size"` // 默认 20，最大 100。
}

// Error 是可比较的业务分类，调用者使用 errors.Is 判断。
type Error string

// Error 让具名字符串可以作为标准 error 返回。
func (err Error) Error() string { return string(err) }

// 错误分类与 HTTP 错误码一一对应。底层连接细节不会直接发给客户端。
const (
	ErrInvalidArgument        Error = "订单参数不合法"
	ErrNotFound               Error = "订单不存在"
	ErrInProgress             Error = "同一请求正在处理中"
	ErrOutcomeUnknown         Error = "下单结果待人工核对"
	ErrIdempotencyUnavailable Error = "幂等记录暂时不可用"
)

// PersistenceError 区分“事务尚未提交”与“提交答复丢失”，后者不能立即归还库存。
type PersistenceError struct {
	Cause            error // 保留底层原因，便于 errors.Is 和内部诊断。
	MayHaveCommitted bool  // true 时数据库可能已经保存订单。
}

// Error 返回具体失败原因，HTTP 层仍只输出安全的固定消息。
func (err *PersistenceError) Error() string { return err.Cause.Error() }

// Unwrap 让标准错误匹配能够穿过这一层包装。
func (err *PersistenceError) Unwrap() error { return err.Cause }

// OutcomeUnknown 让业务流程用同一个小接口判断库存写入与订单写入的不确定性。
func (err *PersistenceError) OutcomeUnknown() bool { return err.MayHaveCommitted }
