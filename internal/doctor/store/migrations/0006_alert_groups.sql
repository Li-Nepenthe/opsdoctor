-- 分组的存续时间独立于诊断状态：报告交了，故障可能仍在继续。
CREATE TABLE alert_groups (
    group_hash CHAR(64) PRIMARY KEY,
    target VARCHAR(32) NOT NULL,
    diagnosis_id CHAR(32) NULL,
    opened_at DATETIME(6) NULL,
    last_seen_at DATETIME(6) NOT NULL,
    closed_at DATETIME(6) NULL,
    FOREIGN KEY (diagnosis_id) REFERENCES diagnoses(id),
    KEY alert_groups_target (target)
) ENGINE=InnoDB
