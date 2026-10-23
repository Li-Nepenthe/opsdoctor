-- order_id 主键让重复消息的 INSERT 发生唯一键冲突，阶段 2 据此避免重复计数。
-- 本阶段造数写入对应历史订单的处理记录，使销量基线一致；建表会取得元数据锁。
CREATE TABLE IF NOT EXISTS processed_events (
    order_id BIGINT NOT NULL COMMENT '已经累计过销量的订单 ID',
    processed_at DATETIME(6) NOT NULL COMMENT 'UTC 处理时间',
    PRIMARY KEY (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
