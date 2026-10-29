-- 多进程启动也只保留同一个名额分配入口。
INSERT IGNORE INTO scheduler_guard(id) VALUES(1)
