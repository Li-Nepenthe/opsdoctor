-- 每一行都是已经发生的操作。model 留实际回复的模型名，不能只看任务初始配置。
CREATE TABLE diagnosis_steps (
    diagnosis_id CHAR(32) NOT NULL,
    seq INT UNSIGNED NOT NULL,
    kind VARCHAR(16) NOT NULL,
    model VARCHAR(128) NOT NULL DEFAULT '',
    tool_name VARCHAR(64) NOT NULL DEFAULT '',
    args JSON NOT NULL,
    result MEDIUMTEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    tokens_in BIGINT UNSIGNED NOT NULL DEFAULT 0,
    tokens_out BIGINT UNSIGNED NOT NULL DEFAULT 0,
    cache_hit_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    cost_micros BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (diagnosis_id, seq),
    FOREIGN KEY (diagnosis_id) REFERENCES diagnoses(id)
) ENGINE=InnoDB
