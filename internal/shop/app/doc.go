// Package app 存放靶场几个程序共用的数据库配置和内部运维入口。
//
// 库存服务、订单服务、order-worker 和 shopctl 都连同一个 MySQL。连接池的默认值
// （最多 20 条连接、保留 10 条空闲、每条最多用 5 分钟）写在这里一份，
// 避免各程序各写一份，慢慢变得不一致。
//
// 各程序自己的组装代码在子目录里，每个子目录对应 cmd/ 下的一个程序：
//   - inventoryservice：库存服务。
//   - orderservice：订单服务（阶段 2）。
//   - orderworker：订单消息消费者（阶段 2）。
//   - shopctl：命令行工具。
//
// 文件清单：
//   - ops.go：启动不对宿主机开放的运维入口，注册池改动指标。
//   - doc.go：本说明。
//   - config.go：DatabaseConfig，读取 MYSQL_DSN 和连接池参数。
//   - config_test.go：检查默认值，以及“空闲连接数不能超过最大连接数”这类约束。
//
// 阅读顺序：config.go → config_test.go，然后进入你关心的那个程序的子目录。
package app
