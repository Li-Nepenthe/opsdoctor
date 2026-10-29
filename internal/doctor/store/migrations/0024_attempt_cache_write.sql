-- 同时保存缓存写入量和发送时的写入价，费用能按旧价重算。
ALTER TABLE llm_attempts ADD COLUMN cache_write_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0, ADD COLUMN reasoning_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0, ADD COLUMN cache_write_price_micros BIGINT UNSIGNED NOT NULL DEFAULT 0
