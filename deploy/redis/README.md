# Redis 的账号和启动方式

订单服务需要读写缓存，医生只需要查看运行情况。两者共用一个 Redis，却不能共用权限。

`start-with-acl.sh` 在启动时生成 ACL 文件。ACL 就是“哪个账号能执行哪些命令”的清单。`default` 账号保留业务所需的命令权限，但必须用 `REDIS_PASSWORD` 登录。`diag_ro` 使用另一个密码 `SHOP_REDIS_RO_PASSWORD`，只允许 INFO、SLOWLOG GET/LEN，以及连接握手所需的 HELLO、PING。它不能读取缓存内容，也不能清库。

文件保存在容器的内存目录 `/run/redis-acl`。脚本先用 `umask 077` 限制新文件的权限，再写入密码哈希。密码原文不出现在启动参数里。健康检查用 `REDISCLI_AUTH` 传密码；不要改成 `redis-cli -a`。

已有环境升级时，先把两个不同的密码填入本机 `.env`，再执行 `docker compose --env-file .env -f deploy/docker-compose.yml down` 和 README 的启动命令。不要加 `-v`，这样已有的缓存和幂等记录会保留。订单服务、实验工具和 Redis exporter 使用业务密码。医生容器只能得到只读账号的连接配置。
