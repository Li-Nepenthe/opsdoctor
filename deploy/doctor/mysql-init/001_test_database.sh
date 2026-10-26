#!/bin/bash
# 只在医生自己的 MySQL 实例创建专用测试库，不能复用 doctor 业务库跑集成测试。
(
set -euo pipefail
: "${MYSQL_ROOT_PASSWORD:?必须设置医生数据库 root 密码}"
export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
mysql --protocol=socket -uroot <<'SQL'
CREATE DATABASE IF NOT EXISTS opsdoctor_doctor_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
GRANT ALL PRIVILEGES ON opsdoctor_doctor_test.* TO 'doctor'@'%';
SQL
)
