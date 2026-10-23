# 通用基础设施

只放不认识库存、订单或医生含义的能力；不得导入业务包和 gen 中的库存协议。每个 Go 包的 doc.go 列出文件职责。

目录内容：

- [config](config/doc.go)：环境读取与校验。
- [logging](logging/doc.go)：slog JSON 与请求关联。
- [httpx](httpx/doc.go)：HTTP 中间件和响应。
- [mysqlx](mysqlx/doc.go)：连接池和 UTC。
- [migrate](migrate/doc.go)：迁移文件、命名锁和历史。
- [lifecycle](lifecycle/doc.go)：信号与清理预算。
- [grpcx](grpcx/doc.go)：gRPC 日志、恢复与停机。
- [adminserver](adminserver/doc.go)：管理端口健康检查。
- [resilience](resilience/doc.go)：入口限流、有预算的重试和熔断。

阅读建议：先读 config/mysqlx/lifecycle，理解启动依赖；需要看请求处理时再读 httpx/grpcx/resilience。本阶段的待核对计数由 orderservice 装配，完整可观测性包留到阶段 3。
