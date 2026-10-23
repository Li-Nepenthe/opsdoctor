-- 每个 SKU 一行累计销量；主键支持阶段 2 worker 的定点累加，避免扫描全表。
-- 建表使用元数据锁；本阶段只写入造数基线，没有启动消费者。
CREATE TABLE IF NOT EXISTS product_sales (
    sku_id BIGINT NOT NULL COMMENT '商品 ID，也是本表主键',
    sold BIGINT NOT NULL COMMENT '累计已处理订单的件数',
    PRIMARY KEY (sku_id),
    CONSTRAINT chk_sales_sold CHECK (sold >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
