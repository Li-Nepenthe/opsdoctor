-- 每个真实 HTTP 尝试都保留 token 预留，包括重试和结果不确定的请求。
CREATE TABLE eval_attempts (
    attempt_id CHAR(32) PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    maximum_tokens BIGINT UNSIGNED NOT NULL,
    charged_tokens BIGINT UNSIGNED NOT NULL,
    FOREIGN KEY (attempt_id) REFERENCES llm_attempts(id) ON DELETE CASCADE,
    FOREIGN KEY (run_id) REFERENCES eval_runs(id)
) ENGINE=InnoDB
