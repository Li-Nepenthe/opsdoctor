// Package loki 从指定的 Loki 查询日志。工具只提供服务、时间和文本条件，适配器负责转义，防止搜索词改变查询结构。
//
// 从 expression 查看筛选条件怎样拼接，再看 Search 如何检查每行日志所属的服务。
//
//   - client.go：按结构化条件查询日志，校验范围、来源和返回数量。
package loki
