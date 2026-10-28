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
// 本阶段重启只把遗留 running 标为失败，阶段 7 才实现恢复；这里不含租约或写审批。
// 阅读顺序：state.go → Repository 接口 → Scheduler.Start → execute。
package run
