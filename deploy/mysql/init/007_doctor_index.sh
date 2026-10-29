#!/bin/bash
# 医生的写账号只有五张架构表的 INDEX 权限，不授予 ALTER、业务数据读写或整库权限。
(
set -euo pipefail
: "${SHOP_MYSQL_DDL_PASSWORD:?必须提供医生的索引账号密码}"
: "${MYSQL_ROOT_PASSWORD:?必须提供 root 密码}"
export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
password_sql=${SHOP_MYSQL_DDL_PASSWORD//\'/\'\'}
mysql --protocol=socket -uroot <<SQL
SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES';
CREATE USER IF NOT EXISTS 'diag_ddl'@'%' IDENTIFIED BY '$password_sql' WITH MAX_USER_CONNECTIONS 1;
ALTER USER 'diag_ddl'@'%' WITH MAX_USER_CONNECTIONS 1;
REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'diag_ddl'@'%';
SQL

# 空数据卷还没有业务表。先创建账号，等服务完成迁移后再次运行本脚本补表级权限。
# 不为省这一步扩大到 shop.*，也不在初始化脚本里复制业务表结构。
tables=$(mysql --protocol=socket -uroot --batch --skip-column-names -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='shop' AND table_name IN ('products','stock','orders','product_sales','processed_events') AND table_type='BASE TABLE'")
if [ "$tables" != "5" ]; then
  printf '%s\n' '业务表尚未齐备：已创建索引账号，服务健康后再次运行 007_doctor_index.sh。'
  exit 0
fi
mysql --protocol=socket -uroot <<SQL
GRANT INDEX ON shop.products TO 'diag_ddl'@'%';
GRANT INDEX ON shop.stock TO 'diag_ddl'@'%';
GRANT INDEX ON shop.orders TO 'diag_ddl'@'%';
GRANT INDEX ON shop.product_sales TO 'diag_ddl'@'%';
GRANT INDEX ON shop.processed_events TO 'diag_ddl'@'%';
SQL
)
