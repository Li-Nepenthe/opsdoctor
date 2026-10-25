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
SQL
# 空库启动时，业务表要等 Go 服务执行迁移才会出现。
# 先创建账号让 MySQL 正常启动；服务健康后再显式执行本脚本，补上两张表的权限。
# 不能为了提前授权就扩大到 shop.*，那会允许实验工具改动无关表。
tables=$(mysql --protocol=socket -uroot --batch --skip-column-names -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'shop' AND table_name IN ('stock', 'orders') AND table_type = 'BASE TABLE'")
if [ "$tables" != "2" ]; then
  printf '%s\n' '业务表尚未就绪：已创建实验账号，请在服务健康后再次执行 005_chaos_user.sh 完成表级授权。'
  exit 0 # 当前位于子 shell，只结束这段初始化，不结束 MySQL 官方入口。
fi
mysql --protocol=socket -uroot <<SQL
GRANT SELECT, ALTER, INDEX ON shop.stock TO 'chaos'@'%';
GRANT SELECT, ALTER, INDEX ON shop.orders TO 'chaos'@'%';
SQL
# ALTER/INDEX 用于两个表的索引变更；不给它订单写入、删库或其他库的权限。
)
