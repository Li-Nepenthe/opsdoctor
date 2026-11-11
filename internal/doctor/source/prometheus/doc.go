// Package prometheus 从一个绑定的 Prometheus 读取指标和告警。即时查询和范围查询返回相同的证据结构，告警只收录已经 firing 的项。
//
// 从 Query 查看路径和时间参数，再看 point 如何拒绝坏采样，防止把缺值误报为零。
//
//   - client.go：校验采样序列、限制点数，并将告警筛选为已触发的项。
package prometheus
