-- 商品主键同时充当 SKU；金额以分保存，避免浮点舍入影响下单金额。
-- 建表会取得元数据锁，应在接收请求前运行；IF NOT EXISTS 允许 DDL 后意外退出再重试。
CREATE TABLE IF NOT EXISTS products (
    id BIGINT NOT NULL COMMENT '商品 ID，也是本项目的 SKU ID',
    name VARCHAR(128) NOT NULL COMMENT '用于展示的商品名称',
    price_cents BIGINT NOT NULL COMMENT '整数分，禁止浮点金额',
    created_at DATETIME(6) NOT NULL COMMENT 'UTC 创建时间',
    PRIMARY KEY (id),
    CONSTRAINT chk_products_price CHECK (price_cents >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
