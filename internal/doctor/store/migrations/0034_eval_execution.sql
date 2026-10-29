-- 每份评测诊断固定模型执行配置与账户，不能靠进程切换偷偷更换币种或供应商。
ALTER TABLE diagnoses
    ADD COLUMN eval_profile VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN eval_provider VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN eval_account VARCHAR(64) NOT NULL DEFAULT ''
