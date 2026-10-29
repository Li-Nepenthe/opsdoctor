-- 用户批准的校时规则变更另存审计，原冻结清单与失败记录都不能覆盖。
CREATE TABLE eval_clock_amendments (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    original_versions JSON NOT NULL,
    replacement_versions JSON NOT NULL,
    makeup_key VARCHAR(128) NOT NULL,
    reason TEXT NOT NULL,
    interruption_id BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (makeup_key) REFERENCES eval_results(result_key),
    FOREIGN KEY (interruption_id) REFERENCES eval_interruptions(id)
);
