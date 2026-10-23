-- id 是聚簇主键，sku_id 故意使用独立二级索引，便于阶段 1 比较有索引和无索引的执行计划。
-- uk_stock_sku 保证一件商品只有一行库存，并让条件 UPDATE 只定位目标行；不要以 sku_id 替换主键。
-- DDL 取得元数据锁，启动迁移期间执行；CHECK 是最后一道防线，业务仍使用条件更新防超卖。
CREATE TABLE IF NOT EXISTS stock (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '存储用主键，业务按 sku_id 查询',
    sku_id BIGINT NOT NULL COMMENT '等于 products.id',
    available INT NOT NULL COMMENT '可售数量，不能为负',
    updated_at DATETIME(6) NOT NULL COMMENT 'UTC 最近变更时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_stock_sku (sku_id),
    CONSTRAINT chk_stock_available CHECK (available >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
