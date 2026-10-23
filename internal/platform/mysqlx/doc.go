// Package mysqlx 建立配置一致的 MySQL 连接池。
//
// app 创建连接池，仓储借它执行 SQL；本包依赖 database/sql 与驱动，不知道业务表，也不自动迁移。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - mysql.go：建立经过校验的 MySQL 连接池。
//   - mysql_integration_test.go：用真实 MySQL 检查连接池和会话时区。
//   - mysql_test.go：离线验证 MySQL 配置规范化。
//
// 阅读顺序：mysql.go → mysql_test.go → mysql_integration_test.go。
package mysqlx
