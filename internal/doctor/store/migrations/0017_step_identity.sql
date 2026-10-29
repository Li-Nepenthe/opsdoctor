-- 同一逻辑结果只保存一次；NULL 允许历史步骤和普通状态事件没有幂等编号。
ALTER TABLE diagnosis_steps
 ADD COLUMN event_key VARCHAR(192) NULL,
 ADD COLUMN parent_seq INT NOT NULL DEFAULT 0,
 ADD COLUMN call_id VARCHAR(128) NOT NULL DEFAULT '',
 ADD COLUMN counted BOOLEAN NOT NULL DEFAULT FALSE,
 ADD UNIQUE KEY diagnosis_step_event(diagnosis_id,event_key)
