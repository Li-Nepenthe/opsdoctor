-- 同一次故障允许两家模型各有 E1；补跑仍在各自配置内最多一次。
ALTER TABLE eval_results
    ADD COLUMN execution_profile VARCHAR(32) NOT NULL DEFAULT '',
    DROP INDEX uk_eval_slot,
    ADD UNIQUE KEY uk_eval_slot (run_id,scenario,iteration,experiment_group,execution_profile,attempt)
