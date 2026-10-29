-- 步骤自己带币种，页面显示旧诊断时不能使用当前账户的币种。
ALTER TABLE diagnosis_steps ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'CNY', ADD COLUMN cache_write_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0, ADD COLUMN reasoning_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0
