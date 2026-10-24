// Package alertsink 是一个临时的告警接收器：接收 Alertmanager 推送的告警，把摘要写进日志。
//
// 告警的流程是：Prometheus 按规则发现异常 → 交给 Alertmanager 分组 → Alertmanager 推送到
// 一个 HTTP 地址。到了阶段 5，这个地址就是医生，医生收到告警就开始诊断。医生出现之前，
// 先用这个接收器顶替，确认告警确实能送到。
//
// 它只做三件事：
//   - 检查请求头里的令牌（Authorization: Bearer ...），不对就返回 401；
//   - 限制请求体的大小；
//   - 把每条告警的名字、状态（firing 表示正在发生，resolved 表示已经恢复）和所属服务写进日志。
//
// 它不判断原因，不修复任何东西，也不访问业务数据库。
//
// 文件清单：
//   - doc.go：本说明。
//   - app.go：Run：读取配置，启动 HTTP 端口（8091），优雅停机。
//   - handler.go：处理推送：检查令牌、读取告警、写日志。
//   - handler_test.go：测试令牌错误和请求体过大时会被拒绝，以及日志里不会出现令牌。
//
// 阅读顺序：handler.go → app.go。
package alertsink
