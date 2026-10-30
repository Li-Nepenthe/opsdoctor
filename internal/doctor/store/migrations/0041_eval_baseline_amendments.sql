-- 本次用户批准只修正诊断期流量检查；旧轮次与中断另存，不能用新注入覆盖它们。
CREATE TABLE eval_baseline_amendments (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    original_versions JSON NOT NULL,
    replacement_versions JSON NOT NULL,
    original_trial JSON NOT NULL,
    interruption_id BIGINT NOT NULL UNIQUE,
    reason TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (interruption_id) REFERENCES eval_interruptions(id)
);
