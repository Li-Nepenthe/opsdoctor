-- 直接保存报告触发方式，验收时不用从措辞猜测是不是被预算迫使结束。
ALTER TABLE diagnoses
    ADD COLUMN report_mode VARCHAR(16) NOT NULL DEFAULT 'legacy' COMMENT 'voluntary、budget；legacy 为旧诊断',
    ADD COLUMN budget_reason VARCHAR(256) NOT NULL DEFAULT '' COMMENT '预算触发的原因';
