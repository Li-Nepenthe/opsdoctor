// Package logging 把服务与请求关联信息加入 JSON 日志。
//
// 各 app 创建 Logger，HTTP/gRPC 层补充 context；本包依赖 slog，不采集调用链或决定业务日志内容。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - logging.go：在 JSON 日志中自动关联请求与服务。
//   - logging_test.go：检查日志字段、UTC 时间和分组之后的关联信息。
//
// 阅读顺序：logging.go → logging_test.go。
package logging
