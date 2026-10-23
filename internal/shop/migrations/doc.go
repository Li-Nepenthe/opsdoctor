// Package migrations 保存 shop 五张表与嵌入入口。
//
// 库存 app 和 shopctl 共用 Apply；它把 SQL 交给 platform/migrate。这里不放业务查询，已提交的迁移只能追加，不能修改。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - 0001_products.sql：商品主键、名称、价格与 UTC 时间。
//   - 0002_stock.sql：独立聚簇主键、SKU 唯一索引与非负库存。
//   - 0003_orders.sql：历史订单字段与用户时间联合索引。
//   - 0004_product_sales.sql：按 SKU 统计已处理件数。
//   - 0005_processed_events.sql：用订单主键记录已经处理的历史事件。
//   - migrations.go：把 shop 的 SQL 文件嵌入并交给通用迁移器。
//
// 阅读顺序：migrations.go → 按编号读五份 SQL，再回看 platform/migrate。
package migrations
