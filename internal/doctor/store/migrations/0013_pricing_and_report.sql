-- 费用按请求当时的三档单价复算。旧记录保持 NULL，不能把阶段 6 的保守旧账伪装成实价。
-- ALTER 仅改变医生的小表结构，迁移期间需要元数据锁，不触碰靶场业务表。
ALTER TABLE llm_attempts
    ADD COLUMN input_price_micros BIGINT NULL COMMENT '每百万输入 token 的未命中价格，微元',
    ADD COLUMN cache_hit_price_micros BIGINT NULL COMMENT '每百万缓存命中 token 的价格，微元',
    ADD COLUMN output_price_micros BIGINT NULL COMMENT '每百万输出 token 的价格，微元';

