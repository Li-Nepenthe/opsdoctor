// Package alerts 把重复收到的告警交给仓储，创建等待处理的诊断任务。
//
// 网络推送不保证只来一次。同一个通知可能正常重发，也可能在超时后重试。我们先
// 检查通知，再按“系统名加分组键”保存。成功响应表示已经落库，阶段 5 不执行诊断。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - service.go：检查 Alertmanager 通知，并把合法的分组交给仓储保存。
//
// 阅读顺序：Notification → Receive → Receiver。去重的数据库实现位于 store 包。
package alerts
