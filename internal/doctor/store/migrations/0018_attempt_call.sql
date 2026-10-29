-- 多次 HTTP 重试归属于同一个发送意图，恢复时可找回结算完成但尚未写步骤的回复。
ALTER TABLE llm_attempts ADD COLUMN call_id VARCHAR(128) NOT NULL DEFAULT '',
 ADD KEY attempts_call(diagnosis_id,call_id)
