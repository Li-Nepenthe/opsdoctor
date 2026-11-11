// Package recoverycheck 用专用测试库验证医生进程退出后的恢复行为。集成测试在指定位置终止模拟进程，
// 让新的执行者只凭数据库留下的记录接手。终止进程的权限只属于验收程序。
//
// doc.go 说明这组验收与医生权限的分界。从 TestIntegrationProcessCrash 读起，
// 再看 TestIntegrationWriteCrash 怎样处理远端已执行、本地还没有结果的写操作。
package recoverycheck
