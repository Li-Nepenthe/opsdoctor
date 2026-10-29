-- 评测器退出后暂停状态仍在；不能因为进程崩溃又启动一批付费告警诊断。
CREATE TABLE evaluation_targets (
    target VARCHAR(32) PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB
