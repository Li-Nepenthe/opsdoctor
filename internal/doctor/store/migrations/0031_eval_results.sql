-- 每个场景轮次的每个组只有一个槽位。重试取回相同输入和诊断，不能重复付费。
CREATE TABLE eval_results (
    result_key VARCHAR(128) PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    scenario VARCHAR(8) NOT NULL,
    iteration INT UNSIGNED NOT NULL,
    experiment_group VARCHAR(8) NOT NULL,
    position INT UNSIGNED NOT NULL,
    fault_started_at DATETIME(6) NOT NULL,
    input JSON NOT NULL,
    diagnosis_id CHAR(32) NULL,
    result JSON NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    finished_at DATETIME(6) NULL,
    UNIQUE KEY uk_eval_slot (run_id,scenario,iteration,experiment_group),
    UNIQUE KEY uk_eval_diagnosis (diagnosis_id),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id),
    FOREIGN KEY (diagnosis_id) REFERENCES diagnoses(id) ON DELETE CASCADE
) ENGINE=InnoDB
