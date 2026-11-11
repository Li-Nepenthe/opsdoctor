// 本文件：库存服务与客户端共用快照和业务错误，传输协议及数据库驱动各自在边界处转换。
// 从 Stock 的数量范围读起，再对照 grpcError 对领域错误的映射。

package inventory

// Stock 是业务层使用的库存快照，不依赖 protobuf 或数据库字段标签。
type Stock struct {
	SKUID     int64 // 与 products.id 相同。
	Available int32 // 与 MySQL 有符号 INT 和 protobuf int32 同范围。
}

// Error 表示库存业务中可由调用方处理的结果。
type Error string

// Error 返回领域错误的固定描述。
func (err Error) Error() string { return string(err) }

// 领域错误描述调用方可以处理的情况。gRPC 状态码在适配层决定。
const (
	ErrInvalidArgument   Error = "库存参数不合法"
	ErrNotFound          Error = "商品不存在"
	ErrInsufficientStock Error = "库存不足"
	ErrUnavailable       Error = "库存存储暂时不可用"
)
