-- 故障时间和实验配置由评测器提供，模型不能通过工具参数改变它们。
ALTER TABLE diagnoses
    ADD COLUMN fault_started_at DATETIME(6) NULL,
    ADD COLUMN eval_group VARCHAR(8) NOT NULL DEFAULT '',
    ADD COLUMN eval_expected_model VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN eval_key VARCHAR(128) NULL,
    ADD UNIQUE KEY uk_diagnosis_eval_key (eval_key)
