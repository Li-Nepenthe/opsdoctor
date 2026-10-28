-- 供应商返回的用量超过发送前估算时，留下持久化停用原因；重启不能绕过人工核对。
ALTER TABLE llm_budget ADD COLUMN stopped_reason VARCHAR(255) NOT NULL DEFAULT ''
