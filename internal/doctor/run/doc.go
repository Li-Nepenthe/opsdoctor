// Package run 决定哪个诊断现在可以执行，并管理取消和结束状态。
//
// 收到告警时先保存 pending 任务，调度器再挑选。一次最多运行两个任务，同一个
// 系统最多一个，避免告警风暴占满全部名额。取消时把 context 传到 agent、工具和模型。
//
// 文件清单：
//   - scheduler_test.go：用可控时间和阻塞模型验证调度名额、过时任务、费用拦截与取消。
//   - doc.go：调度职责和阅读入口。
//   - state.go：明确的状态转换表。
//   - scheduler.go：并发名额、启动清理和任务执行。
//
// 进程关闭会保留步骤。旧租约到期后，新进程才能接手；断开太久则结束任务，避免拿旧证据继续诊断。
// 阅读顺序：state.go → Repository 接口 → Scheduler.Start → execute。
package run
