# 程序入口

每个子目录只有一个 main.go，用包说明介绍入口。这里把进程参数交给 app，不写 SQL、业务校验或启动细节。

目录内容：

- [inventory-service](inventory-service/main.go)：库存常驻进程。
- [shopctl](shopctl/main.go)：造数与手动调用库存。

阅读建议：沿 main 中唯一的 app 调用往里读；不要为后续阶段提前建立空入口。
