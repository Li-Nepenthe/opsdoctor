-- 评分与原始诊断分开保存；格式错误不能覆盖医生原报告，也不能借续跑反复挑分数。
ALTER TABLE eval_results
    ADD COLUMN judge_id CHAR(32) NULL,
    ADD COLUMN judgement JSON NULL,
    ADD UNIQUE KEY uk_eval_judge (judge_id),
    ADD FOREIGN KEY (judge_id) REFERENCES diagnoses(id);
