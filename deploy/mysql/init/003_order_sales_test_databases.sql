-- 订单与销量分别使用专用测试库，避免 go test 并发运行不同包时相互清理。
-- CREATE DATABASE 需要元数据锁；这里只创建空测试库，不改动 shop 业务库。
CREATE DATABASE IF NOT EXISTS opsdoctor_order_test CHARACTER SET utf8mb4;
CREATE DATABASE IF NOT EXISTS opsdoctor_sales_test CHARACTER SET utf8mb4;

-- app 用户由官方镜像初始化，密码来自环境变量。此处只授权，不保存任何密码。
GRANT ALL PRIVILEGES ON opsdoctor_order_test.* TO 'app'@'%';
GRANT ALL PRIVILEGES ON opsdoctor_sales_test.* TO 'app'@'%';
