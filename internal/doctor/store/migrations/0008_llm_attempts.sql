-- 审计行在 HTTP 发送前提交。失败却拿不到用量时保留预留额，不能擅自当作免费。
CREATE TABLE llm_attempts (
    id CHAR(32) PRIMARY KEY,
    diagnosis_id CHAR(32) NOT NULL,
    account VARCHAR(64) NOT NULL,
    requested_model VARCHAR(128) NOT NULL,
    request JSON NOT NULL,
    response JSON NULL,
    maximum_micros BIGINT UNSIGNED NOT NULL,
    charged_micros BIGINT UNSIGNED NOT NULL,
    tokens_in BIGINT UNSIGNED NOT NULL DEFAULT 0,
    tokens_out BIGINT UNSIGNED NOT NULL DEFAULT 0,
    cache_hit_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'reserved',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    settled_at DATETIME(6) NULL,
    FOREIGN KEY (diagnosis_id) REFERENCES diagnoses(id),
    FOREIGN KEY (account) REFERENCES llm_budget(account),
    KEY llm_attempts_diagnosis (diagnosis_id)
) ENGINE=InnoDB
