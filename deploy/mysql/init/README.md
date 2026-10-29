# deploy/mysql/init：首次启动脚本

MySQL 官方镜像在数据目录为空时（也就是第一次启动时），会按文件名顺序执行这个目录里的脚本。

| 脚本 | 作用 | 出现于 |
|---|---|---|
| [001_test_databases.sql](001_test_databases.sql) | 创建基础设施测试用的两个库（连接测试、迁移测试） | 阶段 0 |
| [002_shop_test_databases.sql](002_shop_test_databases.sql) | 创建库存测试库和造数测试库 | 阶段 1 |
| [003_order_sales_test_databases.sql](003_order_sales_test_databases.sql) | 创建订单测试库和销量测试库 | 阶段 2 |
| [004_metrics_user.sh](004_metrics_user.sh) | 创建只读的监控账号，给 mysqld-exporter 采集 MySQL 指标用 | 阶段 3 |
| [005_chaos_user.sh](005_chaos_user.sh) | 先创建实验账号；业务表出现后再执行一次，只授权两个指定表 | 阶段 4 |
| [006_doctor_readonly.sh](006_doctor_readonly.sh) | 创建医生只读账号，授予监控列权限；迁移后补业务表 SELECT | 阶段 5 |

## 为什么每个测试都有自己的库

集成测试会建表、删数据。如果测试连的是业务库 `shop`，一跑测试就会把造好的数据清掉。所以每类测试都有自己专用的库，测试代码也会检查库名，只允许在这些专用库里操作。脚本只给 `app` 账号这些库的权限。

## 两个细节

- **脚本可以重复执行。** 都用 `CREATE DATABASE IF NOT EXISTS` 这类写法，已有的数据卷上手动再执行一次也不会出错。手动执行的方法见根目录 README。
- **004 是 shell 脚本，不是 SQL。** 因为监控账号的密码要从环境变量读取，SQL 文件做不到。脚本里用子 shell 把严格模式（`set -u` 等）包起来，避免影响 MySQL 镜像后续的启动步骤。

空库第一次启动时，`stock` 和 `orders` 还不存在。它们要等业务服务连接 MySQL 后执行迁移才会创建。005 此时只建账号，服务健康后再按根 README 显式运行一次，补上两张表的权限。这样既能完成首次启动，也不用给实验账号整个业务库的修改权限。CI 同样按这个顺序执行。

006 同样分两步运行。它把 diag_ro 限制为最多三条连接，不授予修改表或业务行的权限。已有账号不自动换密码；根 README 给出迁移后的补授权命令。医生自己的库和测试库由另一个容器初始化，见 [doctor/mysql-init](../../doctor/mysql-init/README.md)。

阶段 7 的 [007_doctor_index.sh](007_doctor_index.sh) 为 diag_ddl 创建独立账号，最多一条连接，只授予五张业务表的 INDEX。空库时先建账号，迁移完成后再次执行补表级权限。006 另增加 performance_schema.threads 的 PROCESSLIST_ID 单列查看，医生据此判断旧 DDL 连接是否还在，不读取线程的业务语句内容。
