-- 运行时间只累加真正持有租约的区间。等待审批不占调查时间。
ALTER TABLE diagnoses
 ADD COLUMN active_ms BIGINT NOT NULL DEFAULT 0,
 ADD COLUMN lease_updated_at DATETIME(6) NULL,
 ADD COLUMN cancel_requested BOOLEAN NOT NULL DEFAULT FALSE
