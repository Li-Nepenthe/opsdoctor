-- 外部 MCP 调用不依赖某个诊断；开始先落库，崩溃也保留 running 记录供人工核对。
-- 索引按系统和时间查阅，执行完成更新走主键，不扫描业务表。
CREATE TABLE mcp_audit (
    id CHAR(32) PRIMARY KEY,
    target VARCHAR(32) NOT NULL,
    tool_name VARCHAR(128) NOT NULL,
    arguments_summary TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NULL,
    KEY mcp_audit_target_time (target, created_at)
) ENGINE=InnoDB
