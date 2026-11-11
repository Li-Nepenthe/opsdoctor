// Package mysqlx 在各服务启动时建立 MySQL 连接池，统一限制连接数并设置 UTC 时区。
// 从 mysql.go 的 Open 读起：应用拿到的是通过连通性检查的池，停机时由应用关闭。
//
//   - mysql.go：在连接数据库之前校验配置，为每条新连接设置时区、超时和追踪选项。
package mysqlx
