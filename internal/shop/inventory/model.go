// 本文件：库存领域的快照类型与错误分类。
//
// gRPC 和 MySQL 都有自己的技术类型，业务层不应依赖它们，因此这里单独定义 Stock 和 Error。
// 先看两种数值的范围，再看 Error 方法如何满足 Go 的 error 接口。
//
// 主要内容：
//   - Error：把领域分类转换为错误文本，让这个类型能够直接作为 error 返回。

package inventory

// Stock 是业务层使用的库存快照，不依赖 protobuf 或数据库字段标签。
type Stock struct {
	SKUID     int64 // SKUID 等于 products.id，必须大于 0。
	Available int32 // Available 对应 MySQL 有符号 INT，与 protobuf int32 保持一致。
}

// Error 是可比较的领域错误。用具名字符串常量代替可被重新赋值的包级 error 变量。
// Go 通过方法集合实现接口：只要有 Error() string 方法，它就满足标准库 error 接口。
type Error string

// Error 把领域分类转换为错误文本，让这个类型能够直接作为 error 返回。
func (err Error) Error() string { return string(err) }

// 领域错误描述调用方可以处理的情况。gRPC 状态码在适配层决定。
const (
	ErrInvalidArgument   Error = "库存参数不合法"
	ErrNotFound          Error = "商品不存在"
	ErrInsufficientStock Error = "库存不足"
	ErrUnavailable       Error = "库存存储暂时不可用"
)
