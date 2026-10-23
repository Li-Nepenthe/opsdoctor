# 首次建库脚本

[001_test_databases.sql](001_test_databases.sql) 创建平台测试库；[002_shop_test_databases.sql](002_shop_test_databases.sql) 创建库存与造数测试库；[003_order_sales_test_databases.sql](003_order_sales_test_databases.sql) 创建订单与销量测试库。只授予 app 对专用库的权限，不写密码。

目录内容：

- 当前没有子目录。

阅读建议：先了解官方镜像只在空数据目录执行初始化，再看各脚本的 IF NOT EXISTS；shop 业务表不在这里。

- [004_metrics_user.sh](004_metrics_user.sh)：从环境读取密码创建只读监控账号；已有卷可在 MySQL 容器中单独执行，不重建业务数据。
