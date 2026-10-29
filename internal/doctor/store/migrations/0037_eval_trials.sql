-- 一轮故障要跨过注入、诊断、恢复和冷却。状态单独保存，重启不能重新注入已完成的轮次。
CREATE TABLE eval_trials (
    run_id VARCHAR(64) NOT NULL,
    scenario VARCHAR(8) NOT NULL,
    iteration INT NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'pending',
    fault_started_at DATETIME(6) NULL,
    restored_at DATETIME(6) NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    PRIMARY KEY (run_id, scenario, iteration),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE
);

