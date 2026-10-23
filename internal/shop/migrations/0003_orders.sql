-- 阶段 1 只建立表并生成历史样本；下单业务在阶段 2 实现。
-- 联合索引先按用户缩小范围，再按创建时间排列；建表取得元数据锁，不在请求里执行。
CREATE TABLE IF NOT EXISTS orders (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '订单 ID，也作为后续消费幂等键',
    user_id BIGINT NOT NULL COMMENT '靶场直接接受的用户 ID，没有登录体系',
    sku_id BIGINT NOT NULL COMMENT '购买的商品 ID',
    quantity INT NOT NULL COMMENT '购买件数',
    amount_cents BIGINT NOT NULL COMMENT '下单时的总价，整数分',
    status VARCHAR(32) NOT NULL COMMENT '订单状态，样本使用 created',
    created_at DATETIME(6) NOT NULL COMMENT 'UTC 创建时间',
    PRIMARY KEY (id),
    KEY idx_orders_user_created (user_id, created_at),
    CONSTRAINT chk_orders_quantity CHECK (quantity > 0),
    CONSTRAINT chk_orders_amount CHECK (amount_cents >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
