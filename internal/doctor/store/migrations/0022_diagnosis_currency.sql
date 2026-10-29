-- 一次诊断只使用一种币种，换供应商恢复时不能直接相加。
ALTER TABLE diagnoses ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'CNY', ADD COLUMN cache_write_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0
