-- 旧账户一直按人民币记账，新增字段不能重解释历史金额。
ALTER TABLE llm_budget ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'CNY'
