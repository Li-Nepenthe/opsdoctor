-- 这里只创建 pending 任务。模型、报告、租约等字段保留计划书格式，但阶段 5 不执行它们。
-- active_group 是系统名加分组键的摘要；将来任务结束时清空它，才允许同组的新一轮诊断。
CREATE TABLE IF NOT EXISTS diagnoses (
    id CHAR(32) PRIMARY KEY,
    target VARCHAR(32) NOT NULL,
    `trigger` VARCHAR(16) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    title VARCHAR(256) NOT NULL,
    alert_group_key VARCHAR(2048) NOT NULL,
    active_group CHAR(64) NULL,
    input JSON NOT NULL,
    report JSON NULL,
    error TEXT NULL,
    prompt_version VARCHAR(64) NULL,
    model VARCHAR(128) NULL,
    tokens_in BIGINT UNSIGNED NOT NULL DEFAULT 0,
    tokens_out BIGINT UNSIGNED NOT NULL DEFAULT 0,
    cost_micros BIGINT UNSIGNED NOT NULL DEFAULT 0,
    lease_owner VARCHAR(64) NULL,
    lease_until DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    started_at DATETIME(6) NULL,
    finished_at DATETIME(6) NULL,
    UNIQUE KEY diagnoses_active_group (active_group),
    KEY diagnoses_target_status_created (target,status,created_at)
) ENGINE=InnoDB
