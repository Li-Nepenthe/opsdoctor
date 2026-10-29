-- 一批评测的配置与 token 额度独立于供应商费用账户，两种限制都必须满足。
CREATE TABLE eval_runs (
    id VARCHAR(64) PRIMARY KEY,
    target VARCHAR(32) NOT NULL,
    config JSON NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'preparing',
    token_limit BIGINT UNSIGNED NOT NULL,
    tokens_used BIGINT UNSIGNED NOT NULL DEFAULT 0,
    stopped_reason VARCHAR(256) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB
