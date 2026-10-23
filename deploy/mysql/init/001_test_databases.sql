-- 只建立阶段 0 集成测试的独立数据库，不建立 shop 的任何业务表。
-- CREATE DATABASE 涉及元数据锁，没有表索引；IF NOT EXISTS 支持重新初始化。
CREATE DATABASE IF NOT EXISTS opsdoctor_mysql_test CHARACTER SET utf8mb4;
CREATE DATABASE IF NOT EXISTS opsdoctor_migrate_test CHARACTER SET utf8mb4;

-- app 由官方镜像使用 MYSQL_PASSWORD 环境变量创建；这里不出现密码。
-- GRANT 只授权这两个可丢弃测试库，不扩大对其他库的权限。
GRANT ALL PRIVILEGES ON opsdoctor_mysql_test.* TO 'app'@'%';
GRANT ALL PRIVILEGES ON opsdoctor_migrate_test.* TO 'app'@'%';
