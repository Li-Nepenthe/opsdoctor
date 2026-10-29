-- 第一次失败仍占原槽位；基础设施失败最多有一次补跑，不能覆盖或反复挑结果。
ALTER TABLE eval_results
    ADD COLUMN attempt TINYINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN retry_of VARCHAR(128) NULL,
    ADD UNIQUE KEY uk_eval_retry (retry_of),
    DROP INDEX uk_eval_slot,
    ADD UNIQUE KEY uk_eval_slot (run_id,scenario,iteration,experiment_group,attempt),
    ADD CONSTRAINT ck_eval_attempt CHECK (attempt IN (0,1))
