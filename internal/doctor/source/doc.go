// Package source 约定医生从一个已绑定系统读取证据的能力。工具只传入查询条件，后端地址与账号在组装时确定。日志和说明文字只作为证据。
//
// 从 source.go 的接口找到需要的证据，再进入对应后端的适配器。
//
//   - source.go：定义指标、调用链、日志、MySQL、Redis 与调用栈的只读接口及结果。
//   - select.go：检查供 EXPLAIN 使用的单条 SELECT，拒绝可能有副作用的写法。
//   - query_error.go：标记已由适配器检查、可以返回给工具的错误。
package source
