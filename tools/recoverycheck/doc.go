// Package recoverycheck 用专用测试库验证医生进程退出后的恢复行为。集成测试在断点保存后终止测试子进程，
// 让新的执行者只凭数据库留下的记录接手。终止进程的权限只属于验收程序。
//
// 从 TestIntegrationProcessCrash 读起，
// 再看 TestIntegrationWriteCrash 如何在远端已执行、本地尚无结果时，只靠读取状态确认原操作。
package recoverycheck
