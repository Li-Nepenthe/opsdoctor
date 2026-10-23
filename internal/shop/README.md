# 靶场业务

当前只实现库存与数据基线；不能依赖 doctor、chaos 或 eval。下单、缓存和消费由阶段 2 承担，本目录不提前建空包。

目录内容：

- [app](app/doc.go)：共用配置和两个程序的组装子包。
- [inventory](inventory/doc.go)：模型、业务、MySQL 与 gRPC 适配。
- [migrations](migrations/doc.go)：五张表和嵌入入口。
- [seed](seed/doc.go)：批量建立历史基线。

阅读建议：从 app/inventoryservice 或 app/shopctl 进入；读 inventory 时先看 service 接口，再看 repository SQL。
