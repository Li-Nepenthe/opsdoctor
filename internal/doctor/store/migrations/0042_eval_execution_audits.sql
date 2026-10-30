-- 每次执行命令、推进轮次与运行槽位都追加当时版本；已有实验不回填虚构的执行记录。
CREATE TABLE eval_execution_audits (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    scenario VARCHAR(8) NOT NULL,
    iteration INT NOT NULL,
    operation VARCHAR(24) NOT NULL,
    result_key VARCHAR(128) NOT NULL,
    versions JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE
);
