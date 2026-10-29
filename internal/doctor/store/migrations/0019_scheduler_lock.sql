-- 一行协调锁只用于分配租约的短事务，不会在模型请求期间持有。
CREATE TABLE IF NOT EXISTS scheduler_guard (id INT PRIMARY KEY) ENGINE=InnoDB
