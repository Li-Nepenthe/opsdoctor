#!/bin/bash
# 实验账号只用于宿主机改变指定索引，业务服务不会获得它的密码。
# 已有数据卷可显式运行本脚本；IF NOT EXISTS 不会覆盖已经配置的密码。
(
set -euo pipefail
: "${CHAOS_MYSQL_PASSWORD:?必须通过环境变量提供实验账号密码}"
: "${MYSQL_ROOT_PASSWORD:?必须通过环境变量提供 root 密码}"
export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
password_sql=${CHAOS_MYSQL_PASSWORD//\'/\'\'}
mysql --protocol=socket -uroot <<SQL
SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES';
CREATE USER IF NOT EXISTS 'chaos'@'%' IDENTIFIED BY '$password_sql' WITH MAX_USER_CONNECTIONS 2;
GRANT SELECT, ALTER, INDEX ON shop.stock TO 'chaos'@'%';
GRANT SELECT, ALTER, INDEX ON shop.orders TO 'chaos'@'%';
SQL
# ALTER/INDEX 用于两个表的索引变更；不给它订单写入、删库或其他库的权限。
)
