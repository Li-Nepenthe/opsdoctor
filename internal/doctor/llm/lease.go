// 本文件：把本次执行的租约编号传到步骤保存与费用预留处。编号只在本机使用，取消后的收尾仍须核验租约。

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
