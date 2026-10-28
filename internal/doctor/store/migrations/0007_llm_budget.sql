-- 金额按百万分之一元保存，不用浮点相加；预留额也算在 spent_micros 里。
CREATE TABLE llm_budget (
    account VARCHAR(64) PRIMARY KEY,
    spent_micros BIGINT UNSIGNED NOT NULL DEFAULT 0
) ENGINE=InnoDB
