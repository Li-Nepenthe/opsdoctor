# platform：通用基础设施

这里放每个服务都要用、但和业务无关的代码，比如读配置、写日志、连数据库、处理 HTTP 请求的公共部分。

判断一段代码该不该放这里，看它认不认识业务：只要出现了“库存”“订单”“诊断”这类概念，就不该放在这里。反过来，这里的包也不能导入任何业务包。这条规则由 archcheck 自动检查。

每个子目录都是一个 Go 包，包里的 `doc.go` 会说明这个包做什么、每个文件负责什么。

## 里面有什么

**启动和退出时用到的（阶段 0）：**

| 包 | 作用 |
|---|---|
| [config](config/doc.go) | 读取环境变量，启动时检查是否合法 |
| [logging](logging/doc.go) | 输出 JSON 日志，自动带上请求标识 |
| [mysqlx](mysqlx/doc.go) | 创建 MySQL 连接池，统一连接设置 |
| [migrate](migrate/doc.go) | 数据库迁移：按编号执行建表、改表的 SQL 文件 |
| [lifecycle](lifecycle/doc.go) | 优雅停机：收到退出信号后按顺序关闭资源 |
| [httpx](httpx/doc.go) | HTTP 接口共用的入口处理和统一响应格式 |

**服务之间调用时用到的（阶段 1–2）：**

| 包 | 作用 |
|---|---|
| [grpcx](grpcx/doc.go) | gRPC 服务共用的日志、panic 恢复和停机 |
| [adminserver](adminserver/doc.go) | 每个服务的管理端口：健康检查、指标、性能分析 |
| [resilience](resilience/doc.go) | 服务治理：限流、重试、熔断 |

**可观测性（阶段 3）：**

| 包 | 作用 |
|---|---|
| [telemetry](telemetry/doc.go) | 指标的统一注册、调用链初始化、在 Kafka 消息里传递调用链 |
| [alertsink](alertsink/doc.go) | 临时的告警接收器，检查令牌后把告警摘要写进日志。阶段 5 由医生取代 |

**长时间实验的时间检查（阶段 8）：**

| 包 | 作用 |
|---|---|
| [clockcheck](clockcheck/doc.go) | 同时读取日期和经过时间，区分校时跳变与任务晚得到 CPU；不连接任何外部服务 |

## 从哪里开始读

按上面的分组顺序读。阶段 0 的六个包是所有服务的地基，建议先读 config、logging、httpx，再读 mysqlx、migrate、lifecycle。
