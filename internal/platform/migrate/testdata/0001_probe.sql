-- 只创建集成测试探针表，不是 shop 业务表；主键为后续计数/查找提供唯一性。
-- DDL 会持有元数据锁并隐式提交，IF NOT EXISTS 让中断后重试不会重复创建。
CREATE TABLE IF NOT EXISTS migration_probe (
    id BIGINT NOT NULL PRIMARY KEY
)
