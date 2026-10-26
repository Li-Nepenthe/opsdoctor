// Package store 保存医生自己的用户、任务和告警。
//
// 医生既要查看别的系统，也要记录自己的工作。这是两种不同的权限：source 里的
// 连接只能查看靶场；这里的连接可以写 doctor 库，但不连接 shop。把两者分开，
// 才不会因为需要保存任务，就顺便把业务库的写权限交给医生。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - alerts.go：在一个事务中去重告警分组、创建任务、保存告警快照。
//   - diagnoses.go：读取已经创建的诊断任务，供验收和后续页面使用。
//   - store.go：建立医生仓储并应用医生自己的迁移。
//   - store_integration_test.go：在专用医生测试库验证迁移、通知事务和分组去重。
//   - users.go：保存首次登录账号，并按邮箱查找密码摘要。
//
// 阅读顺序：Migrate → ReceiveAlerts → pendingGroup → List。SQL 在 migrations 目录。
package store
