-- 归属由评测器在创建时确定；模型和工具参数没有更换预算账户的入口。
ALTER TABLE diagnoses
    ADD COLUMN eval_run_id VARCHAR(64) NULL,
    ADD CONSTRAINT fk_diagnosis_eval_run FOREIGN KEY (eval_run_id) REFERENCES eval_runs(id)
