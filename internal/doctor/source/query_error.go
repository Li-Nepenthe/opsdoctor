// 本文件：用 QueryError 标出可以交还给调用者的查询错误。只有适配器筛选过的列名等线索才进入这条路径。

package source

// QueryError 的文本必须已去掉敏感上下文；不要直接用原始网络错误构造它。
type QueryError struct {
	Message string // 由适配器筛选并限制长度的解释。
}

// Error 只返回适配器挑选的说明，避免带出原始后端错误中的连接信息。
func (problem *QueryError) Error() string { return problem.Message }
