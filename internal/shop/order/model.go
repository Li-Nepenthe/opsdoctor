// 本文件：定义下单请求、订单快照和业务错误，供 HTTP、业务和仓储共用。
// Created 是首次下单和同键回放都要返回的结果，Order 还包含用户、商品和创建时间。

package order

import "time"

// CreateRequest 只接受正整数，数量范围与库存 RPC 的 int32 一致。
type CreateRequest struct {
	UserID   int64 `json:"user_id"`
	SKUID    int64 `json:"sku_id"` // 与 products.id 相同。
	Quantity int32 `json:"quantity"`
}

// Created 保存在幂等记录中，同一键重放时继续使用这份结果。
type Created struct {
	OrderID     int64  `json:"order_id"`     // 数据库生成的订单主键。
	AmountCents int64  `json:"amount_cents"` // 单价乘件数，单位是分。
	Status      string `json:"status"`
}

// Order 保存下单时的价格快照，商品后续改价不会改变已创建订单的金额。
type Order struct {
	OrderID     int64     `json:"order_id"` // 自增主键，供详情查询和消息去重。
	UserID      int64     `json:"user_id"`  // 用户列表查询的索引前缀。
	SKUID       int64     `json:"sku_id"`
	Quantity    int32     `json:"quantity"`
	AmountCents int64     `json:"amount_cents"` // 下单时的价格快照，后续商品改价不改变它。
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"` // 使用 UTC，响应按 RFC 3339 编码。
}

// Page 的 Items 在没有结果时也返回空数组，不返回 JSON null。
type Page struct {
	Items    []Order `json:"items"`     // 当前页的订单，按创建时间与编号倒序。
	Page     int     `json:"page"`      // 从 1 开始。
	PageSize int     `json:"page_size"` // 默认 20，最大 100。
}

// Error 表示调用者能够区分并处理的业务失败。
type Error string

// Error 返回这类业务失败的固定说明。
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

// Unwrap 保留数据库驱动报告的失败原因。
func (err *PersistenceError) Unwrap() error { return err.Cause }

// OutcomeUnknown 让业务流程用同一个小接口判断库存写入与订单写入的不确定性。
func (err *PersistenceError) OutcomeUnknown() bool { return err.MayHaveCommitted }
