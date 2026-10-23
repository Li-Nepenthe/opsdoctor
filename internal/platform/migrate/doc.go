// Package migrate 按文件名顺序执行嵌入式 SQL 迁移，并用 MySQL 命名锁避免多进程交错迁移。
// 调用方传入自己的 embed.FS（实现 fs.FS）和目录；本包不包含 shop 或 doctor 的业务表。
// 每个 NNNN_描述.sql 文件只放一条可重入语句；MySQL DDL 会隐式提交，失败后不能承诺回滚。
// 已应用文件的校验和不可改变；新增表结构应追加文件，不能改写已提交的迁移。
package migrate
