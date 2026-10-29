-- 只保存抽样编号，不填写人的分数。重复导出不能换二十份更好看的报告。
CREATE TABLE eval_calibration (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    result_keys JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE
);
