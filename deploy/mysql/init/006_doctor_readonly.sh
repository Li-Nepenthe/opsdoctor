#!/bin/bash
# 医生使用独立账号。即使工具代码出错，这个账号也不能写表、建索引或管理数据库。
(
set -euo pipefail
: "${SHOP_MYSQL_RO_PASSWORD:?必须提供医生的 MySQL 只读密码}"
: "${MYSQL_ROOT_PASSWORD:?必须提供 root 密码}"
export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
password_sql=${SHOP_MYSQL_RO_PASSWORD//\'/\'\'}
mysql --protocol=socket -uroot <<SQL
SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES';
CREATE USER IF NOT EXISTS 'diag_ro'@'%' IDENTIFIED BY '$password_sql' WITH MAX_USER_CONNECTIONS 3;
ALTER USER 'diag_ro'@'%' WITH MAX_USER_CONNECTIONS 3;
REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'diag_ro'@'%';
GRANT SELECT (SCHEMA_NAME,DIGEST_TEXT,COUNT_STAR,SUM_TIMER_WAIT,AVG_TIMER_WAIT,SUM_ROWS_EXAMINED,SUM_ROWS_SENT)
  ON performance_schema.events_statements_summary_by_digest TO 'diag_ro'@'%';
GRANT SELECT (ENGINE,REQUESTING_ENGINE_LOCK_ID,REQUESTING_THREAD_ID,BLOCKING_THREAD_ID)
  ON performance_schema.data_lock_waits TO 'diag_ro'@'%';
GRANT SELECT (ENGINE,ENGINE_LOCK_ID,OBJECT_SCHEMA,OBJECT_NAME,INDEX_NAME,LOCK_TYPE,LOCK_MODE)
  ON performance_schema.data_locks TO 'diag_ro'@'%';
GRANT SELECT (VARIABLE_NAME,VARIABLE_VALUE)
  ON performance_schema.global_status TO 'diag_ro'@'%';
SQL

# 首次空库中还没有业务表。先建账号和监控权限，让 MySQL 正常启动；服务完成迁移后
# 再运行本脚本补齐表级权限。不能为省掉这一步就授权整个 shop.* 或复制业务建表 SQL。
tables=$(mysql --protocol=socket -uroot --batch --skip-column-names -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='shop' AND table_name IN ('products','stock','orders','product_sales','processed_events') AND table_type='BASE TABLE'")
if [ "$tables" != "5" ]; then
  printf '%s\n' '业务表尚未齐备：已创建医生账号，服务健康后再次运行 006_doctor_readonly.sh 授予表级权限。'
  exit 0
fi
mysql --protocol=socket -uroot <<SQL
GRANT SELECT ON shop.products TO 'diag_ro'@'%';
GRANT SELECT ON shop.stock TO 'diag_ro'@'%';
GRANT SELECT ON shop.orders TO 'diag_ro'@'%';
GRANT SELECT ON shop.product_sales TO 'diag_ro'@'%';
GRANT SELECT ON shop.processed_events TO 'diag_ro'@'%';
SQL
# EXPLAIN 要求对涉及的表有 SELECT 权限。数据行的边界还必须由工具代码保证；
# 语句摘要则连账号都不授 QUERY_SAMPLE_TEXT，锁信息也不授 LOCK_DATA。
)
