// Package mysqlx 创建 MySQL 连接池并统一 UTC 会话和连接超时。
// 各服务的 app 读取环境后传入 Config；本包依赖 database/sql 和 go-sql-driver/mysql。
// 它不包含业务 SQL、不执行迁移；连接池归调用方所有，退出时必须 Close。
package mysqlx
