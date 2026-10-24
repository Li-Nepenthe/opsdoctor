// Package logging 负责输出 JSON 格式的日志，并自动带上“这条日志属于哪个请求”的信息。
//
// 为什么需要它：一个服务同时处理很多请求，日志会交错在一起。排查问题时，
// 要能把同一个请求的所有日志挑出来。办法是给每条日志加上同样的标识：
//   - service：哪个服务写的，例如 order-service。
//   - request_id：哪个 HTTP 请求。由 httpx 包在请求进来时生成。
//   - trace_id、span_id：哪条调用链，以及调用链中的哪一步。阶段 3 接入调用链后才有。
//
// 用法：app 在启动时用 New 创建日志器。处理请求时，这些标识放在 context 里一路往下传，
// 业务代码只管正常写日志（例如 logger.InfoContext(ctx, "...")），本包在写出时自动补上它们。
// 日志时间统一用 UTC，方便对齐不同机器的日志。
//
// 本包基于 Go 标准库的 log/slog。
//
// 文件清单：
//   - doc.go：本说明。
//   - logging.go：创建日志器，以及写日志时自动补字段的逻辑。
//   - logging_test.go：检查输出的 JSON 字段和 UTC 时间，以及日志分组后标识仍在最外层。
//
// 阅读顺序：先读 logging.go 的 Fields、WithFields 和 New，弄清标识怎样放进 context；
// 再读 Handle，看写日志时怎样把它们取出来补上。WithAttrs、WithGroup 等方法可以最后看。
package logging
