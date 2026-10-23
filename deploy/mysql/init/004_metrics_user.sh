#!/bin/bash
# 初始化只读监控账号；已有数据卷也可手动执行本脚本，不需要删除或重建业务库。
# Linux checkout 的脚本通常没有执行位，MySQL 入口会用 source 在自己的 shell 中读取它。
# 用括号创建子 shell，让严格选项和密码环境只在这里生效，不影响官方入口后续的启动步骤。
(
set -euo pipefail
: "${MYSQL_EXPORTER_PASSWORD:?必须通过环境变量提供监控账号密码}"
: "${MYSQL_ROOT_PASSWORD:?必须通过环境变量提供 root 密码}"
export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
# 关闭反斜杠转义后，只需把单引号写成两个。密码不进入命令参数，也不输出到终端。
password_sql=${MYSQL_EXPORTER_PASSWORD//\'/\'\'}
mysql --protocol=socket -uroot <<SQL
SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES';
CREATE USER IF NOT EXISTS 'metrics'@'%' IDENTIFIED BY '$password_sql' WITH MAX_USER_CONNECTIONS 3;
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'metrics'@'%';
GRANT SELECT ON performance_schema.* TO 'metrics'@'%';
GRANT SELECT ON shop.* TO 'metrics'@'%';
SQL
# PROCESS 用于 InnoDB 状态，REPLICATION CLIENT 用于复制状态；不授予写数据或改结构的权限。
)
