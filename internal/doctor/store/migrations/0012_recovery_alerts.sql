-- 保存首次在观察期补入的时间，后面的 resolved 快照不能抹掉这个来历。
ALTER TABLE diagnosis_alerts ADD COLUMN recovery_received_at DATETIME(6) NULL;
