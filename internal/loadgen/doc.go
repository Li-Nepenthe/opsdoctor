// Package loadgen 为靶场持续生成真实 HTTP 请求。
// 一个调度循环决定请求类型与商品编号，固定数量的工作者发送请求，避免无限创建 goroutine。
// 控制端口允许调整 QPS 与热点商品，统计只来自实际完成的请求。
// cmd/loadgen 调用 Run。本包只通过 HTTP 访问订单服务，不直接改数据库或编造业务响应。
//
// 文件清单：
//   - doc.go：说明流量生成、控制与统计的分工。
//   - config.go：读取环境变量和命令行参数。
//   - generator.go：按 70/20/10 比例与 Zipf 分布调度请求。
//   - client.go：发送 HTTP 请求，下单超时用原幂等键重试一次。
//   - stats.go：记录真实完成数、错误数与延迟分布。
//   - control.go：提供本机控制接口和健康检查。
//   - app.go：启动、定期打印统计，并在停止时等待在途请求。
//   - loadgen_test.go：验证比例、热点切换、重试键和统计结果。
//
// 建议先读 generator.go，再看 client.go。统计与控制不参与伪造请求结果。
package loadgen
