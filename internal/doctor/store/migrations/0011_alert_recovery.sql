-- closed_at 记录整组恢复的到达时间，观察期结束前仍属于同一场故障。
-- 旧记录保持 NULL，不把已经关闭的历史故障重新打开。
ALTER TABLE alert_groups ADD COLUMN recovery_until DATETIME(6) NULL;
