// Package migrations 存放靶场（shop）的建表 SQL，并提供一个函数把它们交给迁移工具执行。
//
// 每个 .sql 文件是一次迁移，按编号顺序执行，规则见 platform/migrate。已经提交的文件不能再修改；
// 要改表结构，只能新增编号更大的文件。
//
// 谁调用它：库存服务、订单服务、order-worker 和 shopctl 启动时都会调用 Apply。
// 迁移工具保证，即使几个程序同时启动，每个文件也只执行一次。
//
// 五张表：
//   - products：商品。价格用整数“分”保存，避免浮点数的精度误差。
//   - stock：库存。主键是自增的 id；另有一个 sku_id 上的唯一索引 uk_stock_sku，扣库存时靠它快速找到那一行。
//   - orders：订单。有一个 (user_id, created_at) 联合索引，用来按用户查订单列表。
//   - product_sales：每个商品累计卖出的件数。阶段 2 起，由 order-worker 消费消息时累加。
//   - processed_events：已经处理过的订单消息。阶段 2 起，用来防止同一条消息被重复计入销量。
//
// 文件清单：
//   - doc.go：本说明。
//   - migrations.go：用 go:embed 指令把 SQL 文件打包进程序，并提供 Apply。
//   - 0001_products.sql、0002_stock.sql、0003_orders.sql、0004_product_sales.sql、0005_processed_events.sql：五张表。
//
// 阅读顺序：先读 migrations.go，再按编号读五个 SQL 文件。每个字段都用 COMMENT 写了说明。
package migrations
