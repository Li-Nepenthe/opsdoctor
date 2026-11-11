// Package logging 供各服务写 JSON 日志。排查一个请求时，可以按请求标识筛选日志，再用调用链标识找到对应操作。
//
//   - logging.go：负责给每条记录补齐关联字段，从 Handle 开始可以看到业务分组与顶层标识如何同时保留。
package logging
