// Package recoverycheck 验证医生进程真的消失以后，数据库中的断点是否足够恢复。
//
// 普通测试可以把状态改成“像是崩溃过”，这里则运行一个 Mock 子进程，在指定位置
// 终止它，再让新的执行者接手。终止进程的能力只属于这个验收包，医生不持有它。
//
// 文件清单：
//   - doc.go：说明验收程序与医生权限的分界。
//   - fixture_test.go：连接专用测试库，只清理本次创建的记录。
//   - crash_integration_test.go：验证查证、换模型、待审批、过时断点与写到一半的恢复。
//
// 阅读顺序：TestIntegrationProcessCrash → killCrashWorker → TestIntegrationCrashWorker；
// 再看 TestIntegrationWriteCrash，理解为什么远端已经成功时不能盲目重做。
package recoverycheck
