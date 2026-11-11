// Package migrate 在 shop 和 doctor 启动时把各自数据库的表结构升到当前版本。
// 多个实例同时启动时，只有持有该库命名锁的实例能执行迁移。先读 migrate.go 的 Apply，再看 files.go 如何识别旧文件被改写。
//
//   - doc.go：说明迁移器的使用场合和阅读顺序。
//   - migrate.go：在同一会话加锁、核对历史、执行新增的迁移，再解锁并归还连接。
//   - files.go：按版本读取 SQL 文件，并用原始字节计算校验和。
package migrate
