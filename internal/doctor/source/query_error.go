// 本文件：标记经过适配器筛选、可以交还给工具调用者的查询错误。
//
// 大多数后端错误可能带连接地址或账号，工具会隐藏它们。SQL 写错时，调用者需要
// 看到未知列名等线索才能修正，所以只有经过适配器检查的错误才用这个类型。
//
// 主要内容：QueryError 保存可以公开的简短说明；Error 让它满足 error 接口。
// 阅读建议：对照 mysqlinspect 的 explainError 看哪些错误能进入这条路径。

package source

// QueryError 的文本必须已去掉敏感上下文；不要直接用原始网络错误构造它。
type QueryError struct {
	Message string // 由适配器筛选并限制长度的解释。
}

// Error 让工具注册表通过 errors.As 认出这类可公开的错误。
func (problem *QueryError) Error() string { return problem.Message }
