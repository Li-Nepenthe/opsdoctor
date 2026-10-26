// Package prometheus 从一个已绑定的 Prometheus 读取指标和告警。
//
// 指标是一段时间里反复采样的数字。工具需要用它判断“什么时候开始变慢”，所以
// 既要支持看现在的值，也要支持查看之前的一段时间。本包把两种返回格式整理成
// source.Series，后面的工具再计算最大值、最后一个值等摘要。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - client.go：把医生的指标和告警查询转换成 Prometheus HTTP API 请求。
//   - client_test.go：用固定 HTTP 响应检查 Prometheus 适配器的契约。
//
// 阅读顺序：Query → point → Alerts。不能把 pending 告警说成已经正式触发。
package prometheus
