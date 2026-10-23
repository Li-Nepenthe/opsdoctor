-- 库存与造数测试分库，go test 同时运行两个包也不会互相清理数据。
-- CREATE DATABASE 取得元数据锁；这些库只用于可丢弃集成测试，不接收业务请求。
CREATE DATABASE IF NOT EXISTS opsdoctor_inventory_test CHARACTER SET utf8mb4;
CREATE DATABASE IF NOT EXISTS opsdoctor_seed_test CHARACTER SET utf8mb4;

-- app 由官方镜像创建，密码来自环境；这里只增加两个测试库权限，不写入任何凭据。
GRANT ALL PRIVILEGES ON opsdoctor_inventory_test.* TO 'app'@'%';
GRANT ALL PRIVILEGES ON opsdoctor_seed_test.* TO 'app'@'%';
