// 本文件：把执行权编号沿着 context 传到保存步骤和预留费用的位置。
//
// 租约到期后可能已有新进程接手。旧进程即使醒过来，也不能再写步骤或发付费请求。
// 编号只在本机传递，不进入提示、HTTP 正文或工具参数。
//
// 主要内容：WithLease 绑定本次接手的编号；Lease 供数据库检查执行权。
// 阅读建议：context.WithoutCancel 保留这里的值，所以收尾保存也仍要检查租约。

package llm

import "context"

// leaseKey 使用私有类型，其他包不能无意中用同名字符串覆盖编号。
type leaseKey struct{}

// WithLease 每次接手都换新编号；不能只用进程 ID，否则同进程再次接手会混淆旧工作。
func WithLease(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, leaseKey{}, owner)
}

// Lease 返回空串表示没有执行租约，例如不执行诊断的 MCP 查看请求。
func Lease(ctx context.Context) string {
	value, _ := ctx.Value(leaseKey{}).(string)
	return value
}
