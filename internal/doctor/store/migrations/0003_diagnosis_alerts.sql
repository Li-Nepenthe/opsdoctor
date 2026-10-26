-- 指纹在同一个任务中唯一。重复通知更新该告警快照，不复制任务或堆积相同记录。
CREATE TABLE IF NOT EXISTS diagnosis_alerts (
    diagnosis_id CHAR(32) NOT NULL,
    fingerprint CHAR(16) NOT NULL,
    payload JSON NOT NULL,
    received_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (diagnosis_id,fingerprint),
    CONSTRAINT diagnosis_alerts_diagnosis FOREIGN KEY (diagnosis_id) REFERENCES diagnoses(id)
) ENGINE=InnoDB
