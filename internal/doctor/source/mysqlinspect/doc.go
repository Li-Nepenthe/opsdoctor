// Package mysqlinspect 让医生查看 MySQL 为什么变慢，而不是查看业务数据内容。
//
// 执行计划说明数据库准备怎样找数据。索引和表统计说明它有哪些查找方式。语句摘要
// 把具体值换成问号，再统计次数和耗时。锁等待则说明哪些任务正在互相等待。这里
// 把这些问题各写成一个方法，刻意不提供“传 SQL 就能执行”的万能查询。
//
// 连接池最多三条连接，账号还必须设置 MAX_USER_CONNECTIONS 3。前者限制这个进程，
// 后者限制这个账号的所有连接。排队也计入十秒预算，医生不能在数据库忙时无限加连接。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - client.go：用一个最多三条连接的只读账号检查 MySQL。
//   - client_test.go：用 database/sql 的假驱动检查查询模板和连接排队预算。
//   - permissions_integration_test.go：用真实 diag_ro 账号证明“能看执行计划，不能写业务表”。
//
// 阅读顺序：New → Explain → TopQueries → LockWaits → query。
package mysqlinspect
