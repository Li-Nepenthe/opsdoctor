-- 时间戳用于回答“什么时候”，单调时钟时长用于回答“花了多久”，两者不能相减代替。
-- 老任务没有测量值，保留 NULL；不从已经混用时钟的历史时间戳伪造耗时。
ALTER TABLE diagnoses
    ADD COLUMN duration_ms BIGINT UNSIGNED NULL,
    ADD COLUMN execution_segments INT UNSIGNED NOT NULL DEFAULT 0
