// Package migrate 在同一数据库会话持锁执行嵌入式迁移。
//
// 业务 migrations 包调用 Apply；这里负责文件校验、命名锁和历史，不承诺 MySQL DDL 可以整体回滚。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - files.go：读取并校验迁移文件，不执行 SQL。
//   - files_test.go：验证迁移文件和历史校验规则。
//   - migrate.go：在同一 MySQL 会话中串行执行迁移并记录历史。
//   - migrate_integration_test.go：在专用数据库验证迁移和命名锁。
//   - testdata/0001_probe.sql：仅供集成测试的可重入探针表。
//
// 阅读顺序：files.go → migrate.go → files_test.go → migrate_integration_test.go。
package migrate
