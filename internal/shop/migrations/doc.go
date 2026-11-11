// Package migrations 将 shop 的表结构随程序发布。库存、订单、消费者和 shopctl 使用同一套迁移；并发启动时由 platform/migrate 在同一数据库会话中加锁和核对执行历史。
// 从 Apply 读起，再按编号读下面五份 SQL。改表时须追加迁移，已执行文件的原始字节参与历史校验。
//
//   - migrations.go：嵌入同目录的 SQL 并将迁移交给 platform/migrate。
//   - 0001_products.sql：创建商品表，价格按整数分保存。
//   - 0002_stock.sql：创建库存表，扣减依赖 sku_id 上的唯一索引 uk_stock_sku。
//   - 0003_orders.sql：创建订单表，用用户与创建时间的联合索引查询订单列表。
//   - 0004_product_sales.sql：创建商品销量汇总表。
//   - 0005_processed_events.sql：用订单号记录已处理的消息，防止销量重复累加。
package migrations
