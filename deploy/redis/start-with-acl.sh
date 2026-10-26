#!/bin/sh
# Redis 在启动前读取权限清单。密码只从环境取，不拼进 redis-server 的命令行。
set -eu
umask 077
: "${REDIS_PASSWORD:?请设置业务 Redis 密码}"
: "${SHOP_REDIS_RO_PASSWORD:?请设置医生 Redis 密码}"
if [ "$REDIS_PASSWORD" = "$SHOP_REDIS_RO_PASSWORD" ]; then
  echo '业务与医生的 Redis 密码必须不同' >&2
  exit 1
fi

# ACL 文件保存在内存目录。写入哈希可以避免文件里留下明文密码；密码仍须足够随机。
# printf 是 shell 内建命令，原文经标准输入送给 sha256sum，不进入外部进程的参数。
directory=/run/redis-acl
mkdir -p "$directory"
business_hash=$(printf '%s' "$REDIS_PASSWORD" | sha256sum | cut -d ' ' -f 1)
doctor_hash=$(printf '%s' "$SHOP_REDIS_RO_PASSWORD" | sha256sum | cut -d ' ' -f 1)
{
  printf 'user default reset on #%s ~* &* +@all\n' "$business_hash"
  # 先撤销全部权限，再列出允许的查看命令。SLOWLOG RESET 仍然被拒绝。
  printf 'user diag_ro reset on #%s -@all +hello +ping +info +slowlog|get +slowlog|len\n' "$doctor_hash"
} > "$directory/users.acl"
chown -R redis:redis "$directory"
chmod 700 "$directory"
chmod 600 "$directory/users.acl"
unset REDIS_PASSWORD SHOP_REDIS_RO_PASSWORD business_hash doctor_hash

# 继续使用官方入口完成降权和数据目录准备，服务最终以 redis 用户运行。
exec /usr/local/bin/docker-entrypoint.sh redis-server --aclfile "$directory/users.acl" "$@"
