-- 每次暂停都追加原因，后来的成功不能抹掉先前的环境失败。
CREATE TABLE eval_interruptions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    scenario VARCHAR(8) NOT NULL,
    iteration INT NOT NULL,
    state VARCHAR(16) NOT NULL,
    reason TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE
);
