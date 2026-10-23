# 阶段 1 汇报书：库存服务（gRPC + MySQL）

日期：2026-10-23。分支：`phase-01-inventory`，基线 main `3aa6c99`。源码指纹、命令和原始结果见 [notes](../notes.md#phase-01)。本报告中的行号对应本阶段交付版本，日常导航见[代码地图](../代码地图.md)。

## 一、总览

**一句话总结**：库存 gRPC、shop 五表迁移、批量造数、手动调用和真实 MySQL 验收完成，同时落实三层说明规范。

库存查询、扣减和归还已接到 MySQL，扣减用单条条件 UPDATE 防止超卖。请求 deadline 一直传到驱动，服务具备日志、panic 恢复、数据库健康检查和有预算的停机。shopctl 能生成完整实验数据并通过正式 RPC 调用库存，重复造数保留已修改的余额。阶段 0 说明按新规范补齐，commentcheck 新增文件说明、包清单检查和地图输出；阶段 2 尚未开发。

| 计划书阶段 1 的“做”清单 | 结果 |
|---|---|
| 三个 RPC、逐字段说明、buf 生成并提交 | 已完成；源协议遵守第 5.1 节放在 api/proto |
| shop 全部表迁移 | 已完成，五份可重入建表迁移 |
| 服务、两类拦截器、健康端口、deadline | 已完成，9001 / 6062 |
| 库存不足、不存在、超时等错误映射 | 已完成，表格测试覆盖包装错误和安全消息 |
| 50 万商品/库存、200 万订单，small 和重跑跳过 | 已完成，全量与 small 均实测 |
| stock get / stock deduct | 已完成，使用 gRPC；没有直接改表的捷径 |

用户追加的规范升级、阶段 0 回放、精确 archcheck 例外也已完成。学习验收保持未勾选。

## 二、新引入的技术与依赖

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| google.golang.org/grpc | v1.84.0 | 服务、客户端、内存协议测试 | 计划指定，固定查询所得版本 | 另选 RPC 协议 |
| google.golang.org/protobuf | v1.36.12 | 消息与运行时 | 计划指定，和生成插件一致 | 手写序列化 |
| buf | v1.75.0 | lint、生成 | 固定协议工作流 | 人工拼接 protoc 命令 |
| protoc-gen-go | v1.36.12 | Go 消息生成 | 本地固定插件可复现 | 浮动远端插件 |
| protoc-gen-go-grpc | v1.6.2 | RPC 接口生成 | 同上 | 手写客户端/服务描述 |
| golang 构建镜像 | 1.27.1-alpine3.24 | Docker 多阶段构建 | Go 版本与项目一致 | 浮动 latest |
| alpine 运行镜像 | 3.24.2 | 非 root 运行及 wget 健康探测 | 不携带编译工具链 | 直接运行完整构建镜像 |
| genproto/googleapis/rpc（间接） | v0.0.0-20260706201446-f0a921348800 | gRPC 状态依赖 | 由固定模块图解析 | 手动替换协议运行时 |

Go 1.27.1、MySQL 8.4.13、Redis 8.10.2、Kafka 4.4.0、staticcheck 0.8.2 与 ubuntu-24.04 等沿用阶段 0，没有新增技术栈外组件。详见 [版本决定](../decisions.md)。

## 三、代码明细

### 3.1 包与职责

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| cmd/inventory-service、cmd/shopctl | 新增 | 薄入口 | 进程退出码 |
| gen/inventory/v1 | 新增生成物 | 消息、客户端、服务接口 | 三个 RPC 的类型 |
| internal/platform/grpcx | 新增 | 通用日志、恢复、停机 | Logging、Recovery、Shutdown |
| internal/platform/adminserver | 新增 | 健康与 HTTP 清理 | New、Shutdown |
| internal/shop/app | 新增 | shop 共用数据库配置 | DatabaseConfig |
| internal/shop/app/inventoryservice | 新增 | 库存生命周期和组装 | Run、Config |
| internal/shop/app/shopctl | 新增 | CLI 分派与调用 | Run |
| internal/shop/inventory | 新增 | 领域、服务、SQL、RPC 适配 | Stock、领域错误、仓储接口及实现、Service、GRPCServer |
| internal/shop/migrations | 新增 | 嵌入五份迁移 | Apply |
| internal/shop/seed | 新增 | 一致基线与分批事务 | Run、Result |
| internal/platform 原六包、internal/doctor/auth | 只改注释 | 原有能力保持 | 文件说明、doc.go 清单、自然讲解 |
| tools/commentcheck | 修改 | 三层说明校验、阅读输出 | -list / -map 模式 |
| tools/archcheck | 最小逻辑修改及注释 | 根 cmd/README.md 例外 | 保留程序目录仅 main.go 限制 |

### 3.2 文件明细

下面按相对 main 的 Git 变更清单生成，列出每个文件当前总行数及增删行数；报告自身的行数在生成后再次校准。提交后可用 `git diff --stat main...phase-01-inventory` 核对。生成代码也计入，没有省略文档和阶段 0 注释改动。

<!-- FILES_START -->
| 文件 | 当前行数 | + / − | 作用 |
|---|---:|---:|---|
| [.dockerignore](../../.dockerignore) | 9 | +9 / −0 | 排除凭据、Git 和个人文件的构建上下文 |
| [.env.example](../../.env.example) | 47 | +9 / −5 | 演示配置和两组新增测试 DSN |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 80 | +16 / −4 | 固定 Linux 质量检查、Compose 与库存冒烟 |
| [AGENTS.md](../../AGENTS.md) | 111 | +8 / −4 | 保留八步收尾并更新常用命令 |
| [README.md](../../README.md) | 169 | +80 / −17 | 当前范围、环境、测试、造数、协议生成命令 |
| [api/README.md](../../api/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [api/proto/README.md](../../api/proto/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [api/proto/inventory/README.md](../../api/proto/inventory/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [api/proto/inventory/v1/README.md](../../api/proto/inventory/v1/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [api/proto/inventory/v1/inventory.proto](../../api/proto/inventory/v1/inventory.proto) | 46 | +46 / −0 | 库存三个 RPC 的正式契约与字段说明 |
| [buf.gen.yaml](../../buf.gen.yaml) | 9 | +9 / −0 | 本地固定插件生成配置 |
| [buf.yaml](../../buf.yaml) | 9 | +9 / −0 | 协议模块和 lint/breaking 规则 |
| [cmd/README.md](../../cmd/README.md) | 10 | +10 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [cmd/inventory-service/main.go](../../cmd/inventory-service/main.go) | 18 | +18 / −0 | 库存服务入口保持很薄；组装、信号处理与资源回收都由 inventoryservice 管理。 |
| [cmd/shopctl/main.go](../../cmd/shopctl/main.go) | 19 | +19 / −0 | shopctl 是宿主机调试命令入口；实际参数解析和资源管理在 app/shopctl 中完成。 |
| [deploy/README.md](../../deploy/README.md) | 10 | +10 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 107 | +30 / −1 | 新增库存容器、端口、预算、健康探测 |
| [deploy/docker/README.md](../../deploy/docker/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [deploy/docker/go.Dockerfile](../../deploy/docker/go.Dockerfile) | 16 | +16 / −0 | 固定版本多阶段构建与非 root 运行 |
| [deploy/mysql/README.md](../../deploy/mysql/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [deploy/mysql/init/002_shop_test_databases.sql](../../deploy/mysql/init/002_shop_test_databases.sql) | 8 | +8 / −0 | 库存与造数测试分库，go test 同时运行两个包也不会互相清理数据。 |
| [deploy/mysql/init/README.md](../../deploy/mysql/init/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [docs/README.md](../../docs/README.md) | 10 | +10 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [docs/decisions.md](../../docs/decisions.md) | 92 | +39 / −0 | 设计决定 |
| [docs/learn/README.md](../../docs/learn/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [docs/learn/phase-00-foundation.md](../../docs/learn/phase-00-foundation.md) | 473 | +88 / −15 | 阶段 0 学习材料：先把地基讲清楚 |
| [docs/learn/phase-01-inventory.md](../../docs/learn/phase-01-inventory.md) | 487 | +487 / −0 | 阶段 1 学习材料：库存服务（gRPC + MySQL） |
| [docs/notes.md](../../docs/notes.md) | 223 | +95 / −0 | 实测记录 |
| [docs/reports/README.md](../../docs/reports/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [docs/reports/phase-01-inventory.md](../../docs/reports/phase-01-inventory.md) | 540 | +540 / −0 | 阶段 1 汇报书：库存服务（gRPC + MySQL） |
| [docs/代码地图.md](../../docs/代码地图.md) | 122 | +122 / −0 | OpsDoctor 代码地图 |
| [gen/README.md](../../gen/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [gen/inventory/README.md](../../gen/inventory/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [gen/inventory/v1/inventory.pb.go](../../gen/inventory/v1/inventory.pb.go) | 384 | +384 / −0 | buf 生成的消息与 RPC 接口，禁止手改 |
| [gen/inventory/v1/inventory_grpc.pb.go](../../gen/inventory/v1/inventory_grpc.pb.go) | 209 | +209 / −0 | buf 生成的消息与 RPC 接口，禁止手改 |
| [go.mod](../../go.mod) | 44 | +3 / −1 | 固定 gRPC/protobuf 模块版本 |
| [go.sum](../../go.sum) | 103 | +10 / −2 | 对应模块校验和 |
| [internal/README.md](../../internal/README.md) | 11 | +11 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [internal/doctor/README.md](../../internal/doctor/README.md) | 9 | +9 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [internal/doctor/auth/auth.go](../../internal/doctor/auth/auth.go) | 132 | +12 / −6 | 医生登录要用的令牌与密码规则。 |
| [internal/doctor/auth/auth_test.go](../../internal/doctor/auth/auth_test.go) | 126 | +4 / −0 | 认证规则的边界与伪造令牌测试。 |
| [internal/doctor/auth/doc.go](../../internal/doctor/auth/doc.go) | 11 | +10 / −3 | Package auth 医生认证的凭据规则，目前还没有接到 HTTP。 |
| [internal/platform/README.md](../../internal/platform/README.md) | 16 | +16 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [internal/platform/adminserver/doc.go](../../internal/platform/adminserver/doc.go) | 11 | +11 / −0 | Package adminserver 提供独立管理端口的健康检查。 |
| [internal/platform/adminserver/server.go](../../internal/platform/adminserver/server.go) | 52 | +52 / −0 | 管理端口的健康检查和有界关闭。 |
| [internal/platform/adminserver/server_test.go](../../internal/platform/adminserver/server_test.go) | 48 | +48 / −0 | 在内存中验证健康状态和管理接口边界。 |
| [internal/platform/config/config.go](../../internal/platform/config/config.go) | 96 | +11 / −6 | 把环境变量转换为有范围约束的配置值。 |
| [internal/platform/config/config_test.go](../../internal/platform/config/config_test.go) | 78 | +5 / −0 | 用内存环境验证配置读取和错误提示。 |
| [internal/platform/config/doc.go](../../internal/platform/config/doc.go) | 11 | +10 / −3 | Package config 把环境配置转成启动前可以信任的值。 |
| [internal/platform/grpcx/doc.go](../../internal/platform/grpcx/doc.go) | 13 | +13 / −0 | Package grpcx 处理一元 RPC 的通用拦截和有界停机。 |
| [internal/platform/grpcx/interceptor.go](../../internal/platform/grpcx/interceptor.go) | 56 | +56 / −0 | 给每个一元 RPC 加日志和 panic 恢复。 |
| [internal/platform/grpcx/interceptor_test.go](../../internal/platform/grpcx/interceptor_test.go) | 64 | +64 / −0 | 验证拦截器顺序与日志脱敏。 |
| [internal/platform/grpcx/shutdown.go](../../internal/platform/grpcx/shutdown.go) | 34 | +34 / −0 | 给 gRPC 的优雅停机加上总预算。 |
| [internal/platform/grpcx/shutdown_test.go](../../internal/platform/grpcx/shutdown_test.go) | 74 | +74 / −0 | 证明 gRPC 停机超时能取消在途请求。 |
| [internal/platform/httpx/doc.go](../../internal/platform/httpx/doc.go) | 11 | +10 / −3 | Package httpx 集中处理 HTTP 入口的通用保护与统一响应。 |
| [internal/platform/httpx/httpx.go](../../internal/platform/httpx/httpx.go) | 146 | +7 / −2 | HTTP 请求进入业务前后的通用保护。 |
| [internal/platform/httpx/httpx_test.go](../../internal/platform/httpx/httpx_test.go) | 107 | +5 / −0 | 验证 HTTP 中间件组合后的实际响应。 |
| [internal/platform/lifecycle/doc.go](../../internal/platform/lifecycle/doc.go) | 11 | +10 / −3 | Package lifecycle 将退出信号转换成取消通知，再按顺序清理资源。 |
| [internal/platform/lifecycle/lifecycle.go](../../internal/platform/lifecycle/lifecycle.go) | 64 | +5 / −0 | 把退出信号和资源清理组织成可复用流程。 |
| [internal/platform/lifecycle/lifecycle_test.go](../../internal/platform/lifecycle/lifecycle_test.go) | 71 | +5 / −0 | 验证清理顺序、取消信号和停机预算。 |
| [internal/platform/logging/doc.go](../../internal/platform/logging/doc.go) | 11 | +10 / −3 | Package logging 把服务与请求关联信息加入 JSON 日志。 |
| [internal/platform/logging/logging.go](../../internal/platform/logging/logging.go) | 151 | +11 / −6 | 在 JSON 日志中自动关联请求与服务。 |
| [internal/platform/logging/logging_test.go](../../internal/platform/logging/logging_test.go) | 65 | +5 / −0 | 检查日志字段、UTC 时间和分组之后的关联信息。 |
| [internal/platform/migrate/doc.go](../../internal/platform/migrate/doc.go) | 14 | +13 / −4 | Package migrate 在同一数据库会话持锁执行嵌入式迁移。 |
| [internal/platform/migrate/files.go](../../internal/platform/migrate/files.go) | 66 | +7 / −1 | 读取并校验迁移文件，不执行 SQL。 |
| [internal/platform/migrate/files_test.go](../../internal/platform/migrate/files_test.go) | 61 | +5 / −0 | 验证迁移文件和历史校验规则。 |
| [internal/platform/migrate/migrate.go](../../internal/platform/migrate/migrate.go) | 229 | +8 / −3 | 在同一 MySQL 会话中串行执行迁移并记录历史。 |
| [internal/platform/migrate/migrate_integration_test.go](../../internal/platform/migrate/migrate_integration_test.go) | 123 | +5 / −0 | 在专用数据库验证迁移和命名锁。 |
| [internal/platform/mysqlx/doc.go](../../internal/platform/mysqlx/doc.go) | 12 | +11 / −3 | Package mysqlx 建立配置一致的 MySQL 连接池。 |
| [internal/platform/mysqlx/mysql.go](../../internal/platform/mysqlx/mysql.go) | 98 | +8 / −2 | 建立经过校验的 MySQL 连接池。 |
| [internal/platform/mysqlx/mysql_integration_test.go](../../internal/platform/mysqlx/mysql_integration_test.go) | 43 | +5 / −0 | 用真实 MySQL 检查连接池和会话时区。 |
| [internal/platform/mysqlx/mysql_test.go](../../internal/platform/mysqlx/mysql_test.go) | 42 | +5 / −0 | 离线验证 MySQL 配置规范化。 |
| [internal/shop/README.md](../../internal/shop/README.md) | 12 | +12 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [internal/shop/app/config.go](../../internal/shop/app/config.go) | 25 | +25 / −0 | 读取各 shop 程序共用的数据库配置。 |
| [internal/shop/app/config_test.go](../../internal/shop/app/config_test.go) | 37 | +37 / −0 | 验证连接池默认值和参数之间的约束。 |
| [internal/shop/app/doc.go](../../internal/shop/app/doc.go) | 11 | +11 / −0 | Package app 保存 shop 程序共同的数据库配置读取步骤。 |
| [internal/shop/app/inventoryservice/app.go](../../internal/shop/app/inventoryservice/app.go) | 103 | +103 / −0 | 库存进程的资源组装与退出流程。 |
| [internal/shop/app/inventoryservice/config.go](../../internal/shop/app/inventoryservice/config.go) | 36 | +36 / −0 | 库存进程自己的监听地址与启动关闭预算。 |
| [internal/shop/app/inventoryservice/doc.go](../../internal/shop/app/inventoryservice/doc.go) | 11 | +11 / −0 | Package inventoryservice 组装库存进程并拥有它创建的资源。 |
| [internal/shop/app/shopctl/app.go](../../internal/shop/app/shopctl/app.go) | 79 | +79 / −0 | 分派宿主机调试命令并组织造数流程。 |
| [internal/shop/app/shopctl/doc.go](../../internal/shop/app/shopctl/doc.go) | 12 | +12 / −0 | Package shopctl 组装宿主机造数与库存调试命令。 |
| [internal/shop/app/shopctl/stock_client.go](../../internal/shop/app/shopctl/stock_client.go) | 85 | +85 / −0 | 解析库存命令并通过正式 gRPC 契约调用服务。 |
| [internal/shop/app/shopctl/stock_client_test.go](../../internal/shop/app/shopctl/stock_client_test.go) | 37 | +37 / −0 | 验证命令行参数不会被误解或截断。 |
| [internal/shop/inventory/doc.go](../../internal/shop/inventory/doc.go) | 17 | +17 / −0 | Package inventory 实现库存查询、扣减和归还，分开业务、数据与传输职责。 |
| [internal/shop/inventory/grpc_server.go](../../internal/shop/inventory/grpc_server.go) | 74 | +74 / −0 | 把 protobuf 请求接到库存业务并映射状态码。 |
| [internal/shop/inventory/grpc_server_test.go](../../internal/shop/inventory/grpc_server_test.go) | 98 | +98 / −0 | 验证 gRPC 错误契约和内存通道往返。 |
| [internal/shop/inventory/model.go](../../internal/shop/inventory/model.go) | 27 | +27 / −0 | 库存领域的快照类型与错误分类。 |
| [internal/shop/inventory/repository.go](../../internal/shop/inventory/repository.go) | 115 | +115 / −0 | 库存表的查询与原子更新。 |
| [internal/shop/inventory/repository_integration_test.go](../../internal/shop/inventory/repository_integration_test.go) | 163 | +163 / −0 | 真实 MySQL 的并发扣减与 deadline 实验。 |
| [internal/shop/inventory/repository_test.go](../../internal/shop/inventory/repository_test.go) | 45 | +45 / −0 | 离线区分暂时故障和 SQL 配置错误。 |
| [internal/shop/inventory/service.go](../../internal/shop/inventory/service.go) | 65 | +65 / −0 | 库存请求进入仓储前的业务规则。 |
| [internal/shop/inventory/service_test.go](../../internal/shop/inventory/service_test.go) | 100 | +100 / −0 | 离线验证参数规则、取消和仓储错误传递。 |
| [internal/shop/migrations/0001_products.sql](../../internal/shop/migrations/0001_products.sql) | 10 | +10 / −0 | 商品主键同时充当 SKU；金额以分保存，避免浮点舍入影响下单金额。 |
| [internal/shop/migrations/0002_stock.sql](../../internal/shop/migrations/0002_stock.sql) | 12 | +12 / −0 | id 是聚簇主键，sku_id 故意使用独立二级索引，便于阶段 1 比较有索引和无索引的执行计划。 |
| [internal/shop/migrations/0003_orders.sql](../../internal/shop/migrations/0003_orders.sql) | 15 | +15 / −0 | 阶段 1 只建立表并生成历史样本；下单业务在阶段 2 实现。 |
| [internal/shop/migrations/0004_product_sales.sql](../../internal/shop/migrations/0004_product_sales.sql) | 8 | +8 / −0 | 每个 SKU 一行累计销量；主键支持阶段 2 worker 的定点累加，避免扫描全表。 |
| [internal/shop/migrations/0005_processed_events.sql](../../internal/shop/migrations/0005_processed_events.sql) | 7 | +7 / −0 | order_id 主键让重复消息的 INSERT 发生唯一键冲突，阶段 2 据此避免重复计数。 |
| [internal/shop/migrations/doc.go](../../internal/shop/migrations/doc.go) | 15 | +15 / −0 | Package migrations 保存 shop 五张表与嵌入入口。 |
| [internal/shop/migrations/migrations.go](../../internal/shop/migrations/migrations.go) | 25 | +25 / −0 | 把 shop 的 SQL 文件嵌入并交给通用迁移器。 |
| [internal/shop/seed/doc.go](../../internal/shop/seed/doc.go) | 13 | +13 / −0 | Package seed 建立靶场的商品、库存与历史订单基线。 |
| [internal/shop/seed/repository.go](../../internal/shop/seed/repository.go) | 113 | +113 / −0 | 批量造数的五表参数和事务写入。 |
| [internal/shop/seed/seed.go](../../internal/shop/seed/seed.go) | 66 | +66 / −0 | 控制完整规模与 small 模式的造数进度。 |
| [internal/shop/seed/seed_integration_test.go](../../internal/shop/seed/seed_integration_test.go) | 104 | +104 / −0 | 用独立 MySQL 库验证续跑、跳过与回滚。 |
| [internal/shop/seed/seed_test.go](../../internal/shop/seed/seed_test.go) | 34 | +34 / −0 | 在内存中核对造数行数、编号和占位符。 |
| [tools/README.md](../../tools/README.md) | 10 | +10 / −0 | 分组目录职责、边界、子目录和阅读入口 |
| [tools/archcheck/check.go](../../tools/archcheck/check.go) | 387 | +8 / −2 | 按目录、导入和文件访问规则检查架构边界。 |
| [tools/archcheck/check_test.go](../../tools/archcheck/check_test.go) | 143 | +9 / −0 | 用故意违规的假模块检验架构规则。 |
| [tools/archcheck/doc.go](../../tools/archcheck/doc.go) | 12 | +11 / −3 | Package main 把目录分层和领域隔离变成可以在 CI 执行的检查。 |
| [tools/archcheck/main.go](../../tools/archcheck/main.go) | 37 | +4 / −0 | 把架构检查接到命令行与 CI。 |
| [tools/commentcheck/check.go](../../tools/commentcheck/check.go) | 272 | +31 / −7 | 扫描 Go 声明并检查代码层注释。 |
| [tools/commentcheck/check_test.go](../../tools/commentcheck/check_test.go) | 157 | +55 / −6 | 验证代码层规则与薄入口的例外。 |
| [tools/commentcheck/doc.go](../../tools/commentcheck/doc.go) | 14 | +13 / −3 | Package main 把目录、文件、代码三层说明的可检查部分接到 CI。 |
| [tools/commentcheck/files.go](../../tools/commentcheck/files.go) | 146 | +146 / −0 | 检查文件说明与包内文件清单，并输出代码地图。 |
| [tools/commentcheck/files_test.go](../../tools/commentcheck/files_test.go) | 104 | +104 / −0 | 证明三层说明的机器规则能发现缺失，并生成可点击的文件地图。 |
| [tools/commentcheck/main.go](../../tools/commentcheck/main.go) | 66 | +16 / −3 | 分派注释检查、符号清单和文件地图三种命令模式。 |
| [进度.md](../../进度.md) | 163 | +13 / −13 | 本阶段交付与工程验收；学习留给用户 |
| [项目计划书.md](../../项目计划书.md) | 1839 | +3 / −1 | 登记新增测试变量和获批 cmd/README 精确例外 |
<!-- FILES_END -->

### 3.3 类型与函数清单

手写部分以 `commentcheck -list ./...` 为初稿，按包分组，补充调用者。包含所有新增导出类型/函数、SQL 与并发核心函数以及测试辅助函数；旧包仅改说明的既有符号不重复当作新增。测试入口统一由 go test 调用。

<!-- SYMBOLS_START -->
#### cmd/inventory-service

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| main | [cmd/inventory-service/main.go:13](../../cmd/inventory-service/main.go#L13) | main 只把运行结果转成进程退出码；Run 返回前已执行 defer，os.Exit 不会绕过它的清理 | Go 运行时 |

#### cmd/shopctl

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| main | [cmd/shopctl/main.go:14](../../cmd/shopctl/main.go#L14) | main 注入进程参数、环境读取器和输出；失败后用非零退出码让脚本或 CI 停止后续步骤 | Go 运行时 |

#### internal/platform/adminserver

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| New | [internal/platform/adminserver/server.go:27](../../internal/platform/adminserver/server.go#L27) | New 创建未启动的 HTTP 服务器；probe 由 app 注入（例如数据库 Ping），本包不认识业务库 | inventoryservice.Run、TestHealth |
| Shutdown | [internal/platform/adminserver/server.go:46](../../internal/platform/adminserver/server.go#L46) | Shutdown 在正常排空失败后主动 Close，避免已经超过停机预算的连接继续占用资源 | inventoryservice.serve |
| TestHealth | [internal/platform/adminserver/server_test.go:18](../../internal/platform/adminserver/server_test.go#L18) | TestHealth 使用 Recorder 直接调用 handler，不启动 TCP 服务，验证状态和探测上下文 | go test |

#### internal/platform/grpcx

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Logging | [internal/platform/grpcx/interceptor.go:22](../../internal/platform/grpcx/interceptor.go#L22) | Logging 返回最外层拦截器，给每次 RPC 建立关联 ID，并记录内层恢复 panic 后的最终状态 | inventoryservice.Run、TestInterceptors |
| Recovery | [internal/platform/grpcx/interceptor.go:43](../../internal/platform/grpcx/interceptor.go#L43) | Recovery 放在 Logging 内层，把业务 panic 转成 INTERNAL；只恢复当前请求的 goroutine | inventoryservice.Run、TestInterceptors |
| TestInterceptors | [internal/platform/grpcx/interceptor_test.go:22](../../internal/platform/grpcx/interceptor_test.go#L22) | TestInterceptors 验证日志在恢复层外面，因此成功、普通错误、panic 都会记录最终状态且不泄密 | go test |
| Shutdown | [internal/platform/grpcx/shutdown.go:17](../../internal/platform/grpcx/shutdown.go#L17) | Shutdown 先停止接收新 RPC 并等待在途请求，清理预算耗尽时用 Stop 取消剩余 RPC | inventoryservice.serve、TestShutdownDeadline |
| blockingHealth | [internal/platform/grpcx/shutdown_test.go:22](../../internal/platform/grpcx/shutdown_test.go#L22) | blockingHealth 借用标准 health 协议模拟在途 RPC，不引入任何 shop 依赖 | TestShutdownDeadline 构造 |
| *blockingHealth.Check | [internal/platform/grpcx/shutdown_test.go:29](../../internal/platform/grpcx/shutdown_test.go#L29) | Check 一直等到 RPC 被取消；它模仿响应 context 的数据库查询，不包含固定睡眠 | 测试 gRPC 服务端 |
| TestShutdownDeadline | [internal/platform/grpcx/shutdown_test.go:37](../../internal/platform/grpcx/shutdown_test.go#L37) | TestShutdownDeadline 证明预算耗尽后 Stop 会取消在途 RPC，并且 Shutdown 等待自己的 goroutine 退出 | go test |

#### internal/shop/app

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| DatabaseConfig | [internal/shop/app/config.go:18](../../internal/shop/app/config.go#L18) | DatabaseConfig 统一读取 shop 服务与 shopctl 的数据库配置，避免两套默认连接池参数逐渐不一致 | inventoryservice.loadConfig、shopctl.runSeed |
| TestDatabaseConfig | [internal/shop/app/config_test.go:14](../../internal/shop/app/config_test.go#L14) | TestDatabaseConfig 验证默认连接预算和互相约束的 open/idle，错误里不得回显 DSN | go test |

#### internal/shop/app/inventoryservice

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Run | [internal/shop/app/inventoryservice/app.go:30](../../internal/shop/app/inventoryservice/app.go#L30) | Run 拥有库存进程的完整生命周期；具名 result 允许 defer 把关闭连接池的错误也返回给 main | cmd/inventory-service.main |
| serve | [internal/shop/app/inventoryservice/app.go:62](../../internal/shop/app/inventoryservice/app.go#L62) | serve 在两个端口都绑定成功后才接收请求；任意 Serve 异常都触发整组停机 | Run |
| normalServeError | [internal/shop/app/inventoryservice/app.go:98](../../internal/shop/app/inventoryservice/app.go#L98) | normalServeError 只过滤预期的关闭结果；端口故障等错误仍返回给进程入口 | serve |
| Config | [internal/shop/app/inventoryservice/config.go:24](../../internal/shop/app/inventoryservice/config.go#L24) | Config 只保存该服务实际使用的变量；不预读阶段 2/3 尚未实现的能力 | loadConfig 返回，Run 读取 |
| loadConfig | [internal/shop/app/inventoryservice/config.go:31](../../internal/shop/app/inventoryservice/config.go#L31) | loadConfig 汇总配置错误，让用户在建立连接和监听端口前，一次看到所有缺失项 | Run |

#### internal/shop/app/shopctl

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Run | [internal/shop/app/shopctl/app.go:28](../../internal/shop/app/shopctl/app.go#L28) | Run 分派命令并处理 Ctrl+C；reader/output 显式注入，使参数测试不污染进程环境或标准输出 | cmd/shopctl.main |
| runSeed | [internal/shop/app/shopctl/app.go:45](../../internal/shop/app/shopctl/app.go#L45) | runSeed 一条命令完成连接、迁移和批量写入；计时从连接前开始，便于如实记录初始化总耗时 | Run |
| stockOptions | [internal/shop/app/shopctl/stock_client.go:25](../../internal/shop/app/shopctl/stock_client.go#L25) | stockOptions 是已经通过本地校验的命令参数，不含数据库配置 | parseStock 返回，runStock 读取 |
| parseStock | [internal/shop/app/shopctl/stock_client.go:33](../../internal/shop/app/shopctl/stock_client.go#L33) | parseStock 在建立客户端前拒绝错误参数，防止“敲错命令却已经改了数据” | runStock、TestParseStock |
| runStock | [internal/shop/app/shopctl/stock_client.go:53](../../internal/shop/app/shopctl/stock_client.go#L53) | runStock 创建一个短生命周期客户端；本机靶场使用明文 HTTP/2，不能原样部署到公网 | Run |
| TestParseStock | [internal/shop/app/shopctl/stock_client_test.go:14](../../internal/shop/app/shopctl/stock_client_test.go#L14) | TestParseStock 在无网络环境检查 CLI 边界，尤其防止 int64 到 int32 的静默截断 | go test |

#### internal/shop/inventory

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| GRPCServer | [internal/shop/inventory/grpc_server.go:19](../../internal/shop/inventory/grpc_server.go#L19) | GRPCServer 只适配协议；嵌入生成的 Unimplemented 实现，让未来新增 RPC 时仍能向前兼容 | NewGRPCServer 构造；gRPC 注册 |
| NewGRPCServer | [internal/shop/inventory/grpc_server.go:25](../../internal/shop/inventory/grpc_server.go#L25) | NewGRPCServer 把业务对象接到 RPC 适配层；app 再把这个适配器注册到 gRPC 服务器 | inventoryservice.Run、testClient |
| *GRPCServer.GetStock | [internal/shop/inventory/grpc_server.go:30](../../internal/shop/inventory/grpc_server.go#L30) | GetStock 转换领域快照；生成的 Get 方法允许 nil 请求，缺失字段按零值进入业务校验 | 生成的 gRPC 服务分派、适配器测试 |
| *GRPCServer.Deduct | [internal/shop/inventory/grpc_server.go:39](../../internal/shop/inventory/grpc_server.go#L39) | Deduct 沿用 gRPC 收到的 ctx；不能换成 Background，否则调用方 deadline 会在这里丢失 | 生成的 gRPC 服务分派、适配器测试 |
| *GRPCServer.Release | [internal/shop/inventory/grpc_server.go:47](../../internal/shop/inventory/grpc_server.go#L47) | Release 成功返回空消息；不要把无字段误认为无结果，gRPC OK 本身就是操作成功的反馈 | 生成的 gRPC 服务分派、适配器测试 |
| grpcError | [internal/shop/inventory/grpc_server.go:55](../../internal/shop/inventory/grpc_server.go#L55) | grpcError 集中落实第 5.8.1 节契约；错误消息只使用固定中文，不能把驱动详情泄露给客户端 | 三个 GRPCServer 方法、TestGRPCErrorMapping |
| TestGRPCErrorMapping | [internal/shop/inventory/grpc_server_test.go:26](../../internal/shop/inventory/grpc_server_test.go#L26) | TestGRPCErrorMapping 覆盖契约全部错误码；包装后的错误也必须能映射，且不能泄漏底层文字 | go test |
| TestGRPCRoundTrip | [internal/shop/inventory/grpc_server_test.go:52](../../internal/shop/inventory/grpc_server_test.go#L52) | TestGRPCRoundTrip 用内存连接运行真实序列化与服务分派；单元测试不监听 TCP、不依赖外部服务 | go test |
| testClient | [internal/shop/inventory/grpc_server_test.go:71](../../internal/shop/inventory/grpc_server_test.go#L71) | testClient 构造真实 gRPC 内存通道；Cleanup 按后注册先执行的顺序关闭客户端、服务端和监听器 | TestGRPCRoundTrip、TestIntegrationDeadline |
| Error | [internal/shop/inventory/model.go:16](../../internal/shop/inventory/model.go#L16) | Error 是可比较的领域错误；用具名字符串常量代替可被重新赋值的包级 error 变量 | 领域错误常量、仓储 change |
| Error.Error | [internal/shop/inventory/model.go:19](../../internal/shop/inventory/model.go#L19) | Error 把领域分类转换为错误文本，让这个类型能够直接作为 error 返回 | error 接口、fmt |
| Stock | [internal/shop/inventory/model.go:9](../../internal/shop/inventory/model.go#L9) | Stock 是业务层使用的库存快照，不依赖 protobuf 或数据库字段标签 | Service/StockRepository/GRPCServer |
| MySQLStockRepository | [internal/shop/inventory/repository.go:40](../../internal/shop/inventory/repository.go#L40) | MySQLStockRepository 只持有可并发使用的连接池；关闭池的责任属于创建它的 app | NewMySQLStockRepository 构造 |
| NewMySQLStockRepository | [internal/shop/inventory/repository.go:45](../../internal/shop/inventory/repository.go#L45) | NewMySQLStockRepository 借用 app 已建好的连接池，避免每个仓储或请求都另开一组数据库连接 | inventoryservice.Run、集成测试 |
| *MySQLStockRepository.GetStock | [internal/shop/inventory/repository.go:50](../../internal/shop/inventory/repository.go#L50) | GetStock 使用请求 ctx 查询；无行时返回领域错误，网络错误保留原因并标记暂时不可用 | Service 对应方法、真实 MySQL 测试 |
| *MySQLStockRepository.Deduct | [internal/shop/inventory/repository.go:64](../../internal/shop/inventory/repository.go#L64) | Deduct 等待单条条件更新完成；影响行数为 0 时再区分库存不足与不存在，不根据预读来扣减 | Service 对应方法、真实 MySQL 测试 |
| *MySQLStockRepository.Release | [internal/shop/inventory/repository.go:69](../../internal/shop/inventory/repository.go#L69) | Release 以同样的原子更新方式归还；超过 int32 上限属于参数无法满足当前库存范围 | Service 对应方法、真实 MySQL 测试 |
| *MySQLStockRepository.change | [internal/shop/inventory/repository.go:74](../../internal/shop/inventory/repository.go#L74) | change 共用执行和错误分类；statement 只来自本文件常量，用户输入始终通过占位符传递 | Deduct、Release |
| classifyDatabaseError | [internal/shop/inventory/repository.go:94](../../internal/shop/inventory/repository.go#L94) | classifyDatabaseError 隔离驱动类型；业务层只识别领域错误和标准 context 错误 | GetStock、change、分类测试 |
| slowRepository | [internal/shop/inventory/repository_integration_test.go:123](../../internal/shop/inventory/repository_integration_test.go#L123) | slowRepository 仅用于集成测试：保留正常扣减/归还，用带 SLEEP 的真实查询延迟 GetStock | TestIntegrationDeadline |
| *slowRepository.GetStock | [internal/shop/inventory/repository_integration_test.go:129](../../internal/shop/inventory/repository_integration_test.go#L129) | GetStock 用 uk_stock_sku 找行后人为等待五秒；普通 SELECT 不持有排他行锁，ctx 应提前结束驱动等待 | deadline 测试的 Service |
| TestIntegrationDeadline | [internal/shop/inventory/repository_integration_test.go:139](../../internal/shop/inventory/repository_integration_test.go#L139) | TestIntegrationDeadline 验证真实 gRPC deadline 穿过 service，令驱动查询等待和服务端 handler 提前返回 | go test |
| integrationDatabase | [internal/shop/inventory/repository_integration_test.go:26](../../internal/shop/inventory/repository_integration_test.go#L26) | integrationDatabase 只允许专用测试库；每个测试串行清理 stock，不可能清空 shop 业务库 | 三个库存集成测试 |
| TestIntegrationConcurrentDeduct | [internal/shop/inventory/repository_integration_test.go:60](../../internal/shop/inventory/repository_integration_test.go#L60) | TestIntegrationConcurrentDeduct 让 100 个 goroutine 同时争抢 50 件；真实 InnoDB 锁与条件更新决定胜负 | go test |
| TestIntegrationStockBoundaries | [internal/shop/inventory/repository_integration_test.go:97](../../internal/shop/inventory/repository_integration_test.go#L97) | TestIntegrationStockBoundaries 检查不存在、归还和整数上界，覆盖单条 UPDATE 的零影响行数分支 | go test |
| TestDatabaseErrorClassification | [internal/shop/inventory/repository_test.go:18](../../internal/shop/inventory/repository_test.go#L18) | TestDatabaseErrorClassification 区分暂时不可用和永久配置错误，防止所有 SQL 错误都变成可重试 | go test |
| StockRepository | [internal/shop/inventory/service.go:15](../../internal/shop/inventory/service.go#L15) | StockRepository 在使用方声明所需能力，类似 Java 接口，但实现类不用写 implements | Service 依赖，MySQL/测试仓储实现 |
| Service | [internal/shop/inventory/service.go:22](../../internal/shop/inventory/service.go#L22) | Service 保存库存规则和只读依赖；构造后没有可变状态，可被多个 RPC 同时调用 | NewService 构造；GRPCServer 依赖 |
| NewService | [internal/shop/inventory/service.go:27](../../internal/shop/inventory/service.go#L27) | NewService 让 app 把仓储交给业务层；测试可以在同一位置传假仓储，而不必改业务规则 | inventoryservice.Run、业务/gRPC 测试 |
| *Service.GetStock | [internal/shop/inventory/service.go:32](../../internal/shop/inventory/service.go#L32) | GetStock 校验商品 ID 后读取快照；取消或超时不再进入仓储，结果不代表已预留库存 | GRPCServer 对应方法、业务单元测试 |
| *Service.Deduct | [internal/shop/inventory/service.go:40](../../internal/shop/inventory/service.go#L40) | Deduct 只接受正数；并发安全由仓储的一条条件 UPDATE 保证，不在这里做先查后改 | GRPCServer 对应方法、业务单元测试 |
| *Service.Release | [internal/shop/inventory/service.go:48](../../internal/shop/inventory/service.go#L48) | Release 归还正数件库存；没有幂等键，调用方不能因响应丢失就盲目重试 | GRPCServer 对应方法、业务单元测试 |
| validate | [internal/shop/inventory/service.go:56](../../internal/shop/inventory/service.go#L56) | validate 把取消检查和参数规则放在一起，防止三个入口对非法输入的处理不一致 | Service 的三个入口 |
| fakeRepository | [internal/shop/inventory/service_test.go:15](../../internal/shop/inventory/service_test.go#L15) | fakeRepository 保存单次测试的调用次数；本文件串行使用它，不拿普通 int 做并发计数 | 业务/gRPC 单元测试 |
| *fakeRepository.GetStock | [internal/shop/inventory/service_test.go:21](../../internal/shop/inventory/service_test.go#L21) | GetStock 返回固定快照，把业务层测试与 MySQL 分离 | 单元测试中的 Service |
| *fakeRepository.Deduct | [internal/shop/inventory/service_test.go:27](../../internal/shop/inventory/service_test.go#L27) | Deduct 只统计调用，用来验证参数校验发生在仓储之前 | 单元测试中的 Service |
| *fakeRepository.Release | [internal/shop/inventory/service_test.go:33](../../internal/shop/inventory/service_test.go#L33) | Release 与 Deduct 使用相同的假错误来源，验证补偿入口同样遵守业务规则 | 单元测试中的 Service |
| TestServiceValidation | [internal/shop/inventory/service_test.go:39](../../internal/shop/inventory/service_test.go#L39) | TestServiceValidation 检查三种入口的边界，并证明仓储错误不会被业务层吞掉 | go test |
| callService | [internal/shop/inventory/service_test.go:73](../../internal/shop/inventory/service_test.go#L73) | callService 让同一组边界案例覆盖三个公开方法，保持测试断言集中 | TestServiceValidation |
| TestServiceCancellationAndError | [internal/shop/inventory/service_test.go:86](../../internal/shop/inventory/service_test.go#L86) | TestServiceCancellationAndError 确认取消不访问仓储、仓储错误保持可识别，不被重新制造成普通字符串 | go test |

#### internal/shop/migrations

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Apply | [internal/shop/migrations/migrations.go:23](../../internal/shop/migrations/migrations.go#L23) | Apply 使用阶段 0 的命名锁和校验和机制应用迁移；调用方负责提供启动超时并关闭连接池 | inventoryservice.Run、shopctl.runSeed、两组业务集成测试 |

#### internal/shop/seed

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| insertStatement | [internal/shop/seed/repository.go:102](../../internal/shop/seed/repository.go#L102) | insertStatement 为非空且等宽的内部数据构造 VALUES；所有实际值都由驱动参数化传输 | insertBatch、TestMakeBatches |
| inspect | [internal/shop/seed/repository.go:36](../../internal/shop/seed/repository.go#L36) | inspect 拒绝混杂或缺失的基线；这里只校验行数及连续编号，不是对生产数据的完整一致性审计 | Run |
| batch | [internal/shop/seed/repository.go:50](../../internal/shop/seed/repository.go#L50) | batch 保存一张表一批的参数；二维切片的每一项是一行，any 允许整数、字符串、时间混合 | makeBatches 构造、insertStatement 使用 |
| makeBatches | [internal/shop/seed/repository.go:56](../../internal/shop/seed/repository.go#L56) | makeBatches 只在内存生成样本，方便单元测试检查 ID、金额和五表之间的对应关系 | insertBatch、TestMakeBatches |
| insertBatch | [internal/shop/seed/repository.go:78](../../internal/shop/seed/repository.go#L78) | insertBatch 在一个事务里写五张表；result 是具名返回值，defer 可将回滚错误追加到原始错误 | Run、种子集成测试 |
| Result | [internal/shop/seed/seed.go:26](../../internal/shop/seed/seed.go#L26) | Result 只包含计数与耗时，便于 CLI 打印真实结果；不包含数据库配置 | Run 返回、shopctl 输出 |
| Run | [internal/shop/seed/seed.go:36](../../internal/shop/seed/seed.go#L36) | Run 在已迁移的数据库中建立基线；ctx 控制所有查询与每批事务，返回已经提交的计数和错误 | shopctl.runSeed、种子集成测试 |
| seedDatabase | [internal/shop/seed/seed_integration_test.go:20](../../internal/shop/seed/seed_integration_test.go#L20) | seedDatabase 仅在专用库清理五张业务表；使用独立库避免与库存包并行测试时互相删数据 | 两个种子集成测试 |
| TestIntegrationSeed | [internal/shop/seed/seed_integration_test.go:52](../../internal/shop/seed/seed_integration_test.go#L52) | TestIntegrationSeed 验证从已提交批次继续、重复跳过、保持用户改过的库存及五表数量一致 | go test |
| TestIntegrationSeedRollback | [internal/shop/seed/seed_integration_test.go:78](../../internal/shop/seed/seed_integration_test.go#L78) | TestIntegrationSeedRollback 故意制造第三张表的主键冲突，证明前面两张表不会留下半批数据 | go test |
| TestMakeBatches | [internal/shop/seed/seed_test.go:14](../../internal/shop/seed/seed_test.go#L14) | TestMakeBatches 验证两个 SKU 的五表基线互相对应；此测试只生成内存参数，不连接数据库 | go test |

#### tools/archcheck

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| locationProblem | [tools/archcheck/check.go:116](../../tools/archcheck/check.go#L116) | locationProblem 检查根目录 Go 文件、cmd 入口文件数量和含糊的包名；返回空字符串表示通过 | check、TestLocations |
| TestLocations | [tools/archcheck/check_test.go:89](../../tools/archcheck/check_test.go#L89) | TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报 | go test |

#### tools/commentcheck

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| packageDocument | [tools/commentcheck/check.go:136](../../tools/commentcheck/check.go#L136) | packageDocument 只给 cmd/<程序>/main.go 开入口例外，避免普通包靠随意文件名绕过 doc.go 规则 | inspectFile |
| inspectFunction | [tools/commentcheck/check.go:148](../../tools/commentcheck/check.go#L148) | inspectFunction 检查文档和长函数内部注释，并记录方法接收者，防止同名方法在清单里混淆 | inspectFile |
| receiverName | [tools/commentcheck/check.go:181](../../tools/commentcheck/check.go#L181) | receiverName 直接读取 AST 类型表达式；支持指针和泛型接收者，不引入反射或类型加载依赖 | inspectFunction |
| inspectDeclaration | [tools/commentcheck/check.go:202](../../tools/commentcheck/check.go#L202) | inspectDeclaration 检查包级类型、常量、变量；常量和变量允许组注释，局部变量不在机器检查范围 | inspectFile |
| hasDoc | [tools/commentcheck/check.go:235](../../tools/commentcheck/check.go#L235) | hasDoc 不把空白注释或单独的 go: 指令当成说明，防止只有形式却没有可读内容 | packageDocument、函数/声明校验 |
| firstSentence | [tools/commentcheck/check.go:238](../../tools/commentcheck/check.go#L238) | firstSentence 以换行或中文句号结束第一句；清单是初稿，不替代完整文档注释 | inspectFunction、inspectDeclaration、describeFile、fileDescription |
| symbol | [tools/commentcheck/check.go:24](../../tools/commentcheck/check.go#L24) | symbol 是 -list 的一行；字段只保存源码信息，不执行或反射被检查的对象 | inspectFunction/inspectDeclaration 构造，writeList 读取 |
| writeList | [tools/commentcheck/check.go:253](../../tools/commentcheck/check.go#L253) | writeList 按包和位置稳定排序，输出 Markdown 表格；写入错误会返回，避免生成不完整的汇报书 | run、清单测试 |
| scanResult | [tools/commentcheck/check.go:32](../../tools/commentcheck/check.go#L32) | scanResult 把诊断、符号和文件导航放在一次扫描结果中，让三种输出模式使用同一份源码信息 | scan/inspectFile 累积，run 输出 |
| scan | [tools/commentcheck/check.go:40](../../tools/commentcheck/check.go#L40) | scan 遍历 patterns 指定的目录（支持 ./...）；语法错误和读取错误直接返回，不静默漏检 | run、检查器测试 |
| inspectFile | [tools/commentcheck/check.go:95](../../tools/commentcheck/check.go#L95) | inspectFile 解析单个文件并检查声明；标准生成标记由 ast.IsGenerated 识别，生成物跳过 | scan |
| TestListAndGroupedComments | [tools/commentcheck/check_test.go:103](../../tools/commentcheck/check_test.go#L103) | TestListAndGroupedComments 验证分组声明、15 行边界、长函数内部注释和清单输出 | go test |
| TestSyntaxErrorAndExit | [tools/commentcheck/check_test.go:123](../../tools/commentcheck/check_test.go#L123) | TestSyntaxErrorAndExit 检查语法错误和违规不会在 CLI 入口被吞掉，确保 CI 能收到失败 | go test |
| TestCommandPackageDoc | [tools/commentcheck/check_test.go:137](../../tools/commentcheck/check_test.go#L137) | TestCommandPackageDoc 验证 cmd 单文件入口的包说明例外，不放宽普通包或缺注释入口 | go test |
| fixture | [tools/commentcheck/check_test.go:17](../../tools/commentcheck/check_test.go#L17) | fixture 把源码样例写进独立临时目录；t.TempDir 负责清理，不修改真实仓库文件 | 检查器测试、documentedFixture |
| documentedFixture | [tools/commentcheck/check_test.go:34](../../tools/commentcheck/check_test.go#L34) | documentedFixture 给旧规则的样例补齐新的导航外壳，让每个旧测试仍只破坏自己负责的代码注释规则 | 旧规则测试 |
| TestMissingComments | [tools/commentcheck/check_test.go:54](../../tools/commentcheck/check_test.go#L54) | TestMissingComments 逐一删除不同位置的注释，证明检查器会失败且给出文件和行号 | go test |
| TestPackageAndGenerated | [tools/commentcheck/check_test.go:90](../../tools/commentcheck/check_test.go#L90) | TestPackageAndGenerated 验证空 doc.go 不能通过，标准生成文件则不要求手写注释 | go test |
| mentionsFilename | [tools/commentcheck/files.go:107](../../tools/commentcheck/files.go#L107) | mentionsFilename 按完整文件名匹配，避免 old_service.go 或 service.go.bak 冒充 service.go | inventoryProblems |
| writeMap | [tools/commentcheck/files.go:121](../../tools/commentcheck/files.go#L121) | writeMap 输出可粘贴到 docs/代码地图.md 的表格；链接以 docs 为起点，统一使用相对路径 | run |
| sourceFile | [tools/commentcheck/files.go:21](../../tools/commentcheck/files.go#L21) | sourceFile 是地图中的一项，也保留清单校验需要的包说明；这里只保存文本，不执行源码 | describeFile 构造，清单校验和地图使用 |
| describeFile | [tools/commentcheck/files.go:31](../../tools/commentcheck/files.go#L31) | describeFile 先识别生成物、doc.go 和薄入口，再把普通文件交给严格的文件说明校验 | inspectFile |
| fileDescription | [tools/commentcheck/files.go:57](../../tools/commentcheck/files.go#L57) | fileDescription 要求首行明确说明用途，并与 package 分开；否则 Go 会错误地把它当成包文档 | describeFile |
| isCommandMain | [tools/commentcheck/files.go:77](../../tools/commentcheck/files.go#L77) | isCommandMain 识别计划书规定的单文件入口；mycmd 或其他目录里的 main.go 不享有例外 | describeFile、packageDocument |
| inventoryProblems | [tools/commentcheck/files.go:82](../../tools/commentcheck/files.go#L82) | inventoryProblems 等扫描结束再核对 doc.go，避免因为文件遍历顺序而误报漏项 | scan |
| TestFileDescriptions | [tools/commentcheck/files_test.go:17](../../tools/commentcheck/files_test.go#L17) | TestFileDescriptions 区分真正的文件导航、贴在 package 上的包文档，以及只有空标题的占位注释 | go test |
| TestPackageInventory | [tools/commentcheck/files_test.go:48](../../tools/commentcheck/files_test.go#L48) | TestPackageInventory 验证完整文件名匹配；测试文件与生成物不强制列出，但仍可出现在人工说明中 | go test |
| TestMapMode | [tools/commentcheck/files_test.go:76](../../tools/commentcheck/files_test.go#L76) | TestMapMode 核对所有文件都进入地图、相对链接能解析、生成物有标记，并拒绝相互冲突的输出模式 | go test |
| main | [tools/commentcheck/main.go:16](../../tools/commentcheck/main.go#L16) | main 把命令行参数交给 run，并把失败转换为非零退出码，供 CI 阻止缺注释的提交 | Go 运行时 |
| run | [tools/commentcheck/main.go:24](../../tools/commentcheck/main.go#L24) | run 让检查、符号清单与文件地图共用一次扫描；传入 writer 后，测试可以直接检查输出而不用启动子进程 | main、CLI 测试 |
<!-- SYMBOLS_END -->

生成代码由 buf 维护，下面逐项列出导出类型、函数和方法；位置来自生成文件，不手改其注释。

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| GetStockRequest | [gen/inventory/v1/inventory.pb.go:27](../../gen/inventory/v1/inventory.pb.go#L27) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| GetStockRequest.Reset | [gen/inventory/v1/inventory.pb.go:34](../../gen/inventory/v1/inventory.pb.go#L34) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockRequest.String | [gen/inventory/v1/inventory.pb.go:41](../../gen/inventory/v1/inventory.pb.go#L41) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockRequest.ProtoMessage | [gen/inventory/v1/inventory.pb.go:45](../../gen/inventory/v1/inventory.pb.go#L45) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockRequest.ProtoReflect | [gen/inventory/v1/inventory.pb.go:47](../../gen/inventory/v1/inventory.pb.go#L47) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockRequest.Descriptor | [gen/inventory/v1/inventory.pb.go:60](../../gen/inventory/v1/inventory.pb.go#L60) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockRequest.GetSkuId | [gen/inventory/v1/inventory.pb.go:64](../../gen/inventory/v1/inventory.pb.go#L64) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| GetStockResponse | [gen/inventory/v1/inventory.pb.go:72](../../gen/inventory/v1/inventory.pb.go#L72) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| GetStockResponse.Reset | [gen/inventory/v1/inventory.pb.go:80](../../gen/inventory/v1/inventory.pb.go#L80) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockResponse.String | [gen/inventory/v1/inventory.pb.go:87](../../gen/inventory/v1/inventory.pb.go#L87) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockResponse.ProtoMessage | [gen/inventory/v1/inventory.pb.go:91](../../gen/inventory/v1/inventory.pb.go#L91) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockResponse.ProtoReflect | [gen/inventory/v1/inventory.pb.go:93](../../gen/inventory/v1/inventory.pb.go#L93) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockResponse.Descriptor | [gen/inventory/v1/inventory.pb.go:106](../../gen/inventory/v1/inventory.pb.go#L106) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| GetStockResponse.GetSkuId | [gen/inventory/v1/inventory.pb.go:110](../../gen/inventory/v1/inventory.pb.go#L110) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| GetStockResponse.GetAvailable | [gen/inventory/v1/inventory.pb.go:117](../../gen/inventory/v1/inventory.pb.go#L117) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| DeductRequest | [gen/inventory/v1/inventory.pb.go:125](../../gen/inventory/v1/inventory.pb.go#L125) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| DeductRequest.Reset | [gen/inventory/v1/inventory.pb.go:133](../../gen/inventory/v1/inventory.pb.go#L133) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductRequest.String | [gen/inventory/v1/inventory.pb.go:140](../../gen/inventory/v1/inventory.pb.go#L140) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductRequest.ProtoMessage | [gen/inventory/v1/inventory.pb.go:144](../../gen/inventory/v1/inventory.pb.go#L144) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductRequest.ProtoReflect | [gen/inventory/v1/inventory.pb.go:146](../../gen/inventory/v1/inventory.pb.go#L146) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductRequest.Descriptor | [gen/inventory/v1/inventory.pb.go:159](../../gen/inventory/v1/inventory.pb.go#L159) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| DeductRequest.GetSkuId | [gen/inventory/v1/inventory.pb.go:163](../../gen/inventory/v1/inventory.pb.go#L163) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| DeductRequest.GetQuantity | [gen/inventory/v1/inventory.pb.go:170](../../gen/inventory/v1/inventory.pb.go#L170) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| DeductResponse | [gen/inventory/v1/inventory.pb.go:178](../../gen/inventory/v1/inventory.pb.go#L178) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| DeductResponse.Reset | [gen/inventory/v1/inventory.pb.go:184](../../gen/inventory/v1/inventory.pb.go#L184) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductResponse.String | [gen/inventory/v1/inventory.pb.go:191](../../gen/inventory/v1/inventory.pb.go#L191) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductResponse.ProtoMessage | [gen/inventory/v1/inventory.pb.go:195](../../gen/inventory/v1/inventory.pb.go#L195) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductResponse.ProtoReflect | [gen/inventory/v1/inventory.pb.go:197](../../gen/inventory/v1/inventory.pb.go#L197) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| DeductResponse.Descriptor | [gen/inventory/v1/inventory.pb.go:210](../../gen/inventory/v1/inventory.pb.go#L210) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest | [gen/inventory/v1/inventory.pb.go:215](../../gen/inventory/v1/inventory.pb.go#L215) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| ReleaseRequest.Reset | [gen/inventory/v1/inventory.pb.go:223](../../gen/inventory/v1/inventory.pb.go#L223) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest.String | [gen/inventory/v1/inventory.pb.go:230](../../gen/inventory/v1/inventory.pb.go#L230) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest.ProtoMessage | [gen/inventory/v1/inventory.pb.go:234](../../gen/inventory/v1/inventory.pb.go#L234) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest.ProtoReflect | [gen/inventory/v1/inventory.pb.go:236](../../gen/inventory/v1/inventory.pb.go#L236) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest.Descriptor | [gen/inventory/v1/inventory.pb.go:249](../../gen/inventory/v1/inventory.pb.go#L249) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseRequest.GetSkuId | [gen/inventory/v1/inventory.pb.go:253](../../gen/inventory/v1/inventory.pb.go#L253) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| ReleaseRequest.GetQuantity | [gen/inventory/v1/inventory.pb.go:260](../../gen/inventory/v1/inventory.pb.go#L260) | nil 安全读取字段 | GRPCServer / shopctl / protobuf 使用方 |
| ReleaseResponse | [gen/inventory/v1/inventory.pb.go:268](../../gen/inventory/v1/inventory.pb.go#L268) | 生成消息类型 | shopctl、GRPCServer、protobuf 运行时 |
| ReleaseResponse.Reset | [gen/inventory/v1/inventory.pb.go:274](../../gen/inventory/v1/inventory.pb.go#L274) | 重置消息 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseResponse.String | [gen/inventory/v1/inventory.pb.go:281](../../gen/inventory/v1/inventory.pb.go#L281) | 消息字符串表示 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseResponse.ProtoMessage | [gen/inventory/v1/inventory.pb.go:285](../../gen/inventory/v1/inventory.pb.go#L285) | 标识 protobuf 消息 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseResponse.ProtoReflect | [gen/inventory/v1/inventory.pb.go:287](../../gen/inventory/v1/inventory.pb.go#L287) | 访问消息反射 | protobuf / gRPC 运行时；业务无直接调用 |
| ReleaseResponse.Descriptor | [gen/inventory/v1/inventory.pb.go:300](../../gen/inventory/v1/inventory.pb.go#L300) | 旧版描述兼容 API | protobuf / gRPC 运行时；业务无直接调用 |
| InventoryServiceClient | [gen/inventory/v1/inventory_grpc.pb.go:34](../../gen/inventory/v1/inventory_grpc.pb.go#L34) | 客户端三个 RPC 的接口 | shopctl、测试 |
| NewInventoryServiceClient | [gen/inventory/v1/inventory_grpc.pb.go:47](../../gen/inventory/v1/inventory_grpc.pb.go#L47) | 创建生成客户端 | shopctl、gRPC 测试 |
| inventoryServiceClient.GetStock | [gen/inventory/v1/inventory_grpc.pb.go:51](../../gen/inventory/v1/inventory_grpc.pb.go#L51) | 生成的 RPC 客户端调用 | shopctl、测试、未来调用方 |
| inventoryServiceClient.Deduct | [gen/inventory/v1/inventory_grpc.pb.go:61](../../gen/inventory/v1/inventory_grpc.pb.go#L61) | 生成的 RPC 客户端调用 | shopctl、测试、未来调用方 |
| inventoryServiceClient.Release | [gen/inventory/v1/inventory_grpc.pb.go:71](../../gen/inventory/v1/inventory_grpc.pb.go#L71) | 生成的 RPC 客户端调用 | shopctl、测试、未来调用方 |
| InventoryServiceServer | [gen/inventory/v1/inventory_grpc.pb.go:86](../../gen/inventory/v1/inventory_grpc.pb.go#L86) | 服务端三个 RPC 的接口 | GRPCServer 实现、gRPC 注册 |
| UnimplementedInventoryServiceServer | [gen/inventory/v1/inventory_grpc.pb.go:101](../../gen/inventory/v1/inventory_grpc.pb.go#L101) | 默认未实现方法，支持以后扩展协议 | GRPCServer 值嵌入 |
| UnimplementedInventoryServiceServer.GetStock | [gen/inventory/v1/inventory_grpc.pb.go:103](../../gen/inventory/v1/inventory_grpc.pb.go#L103) | 默认返回 Unimplemented | 接口默认实现（业务覆盖） |
| UnimplementedInventoryServiceServer.Deduct | [gen/inventory/v1/inventory_grpc.pb.go:106](../../gen/inventory/v1/inventory_grpc.pb.go#L106) | 默认返回 Unimplemented | 接口默认实现（业务覆盖） |
| UnimplementedInventoryServiceServer.Release | [gen/inventory/v1/inventory_grpc.pb.go:109](../../gen/inventory/v1/inventory_grpc.pb.go#L109) | 默认返回 Unimplemented | 接口默认实现（业务覆盖） |
| UnsafeInventoryServiceServer | [gen/inventory/v1/inventory_grpc.pb.go:118](../../gen/inventory/v1/inventory_grpc.pb.go#L118) | 显式放弃向前兼容的服务接口 | 本项目未使用 |
| RegisterInventoryServiceServer | [gen/inventory/v1/inventory_grpc.pb.go:122](../../gen/inventory/v1/inventory_grpc.pb.go#L122) | 注册三个服务方法 | inventoryservice.Run、测试 |

### 3.4 数据库变更

迁移目录：[internal/shop/migrations](../../internal/shop/migrations/doc.go)。一文件一条 DDL，沿用命名锁、校验和与可重入执行；不承诺整批 DDL 回滚。

| 表 | 主键 / 索引 | 迁移 |
|---|---|---|
| products | PRIMARY(id)，金额非负 CHECK | 0001_products.sql |
| stock | PRIMARY(id)、UNIQUE uk_stock_sku(sku_id)，余额非负 CHECK | 0002_stock.sql |
| orders | PRIMARY(id)、idx_orders_user_created(user_id, created_at)，数量/金额 CHECK | 0003_orders.sql |
| product_sales | PRIMARY(sku_id)，销量非负 CHECK | 0004_product_sales.sql |
| processed_events | PRIMARY(order_id) | 0005_processed_events.sql |

新增 opsdoctor_inventory_test、opsdoctor_seed_test 专用库由 deploy/mysql/init/002_shop_test_databases.sql 创建，旧卷显式执行一次；没有删除卷。库存和 seed 测试在清理前验证精确库名。

### 3.5 接口和端口

| 接口 | 输入 / 输出 | 端口 / 状态 |
|---|---|---|
| GetStock | sku_id int64 → sku_id int64、available int32 | gRPC 9001 |
| Deduct / Release | sku_id int64、quantity int32 → 空响应 | gRPC 9001；成功 OK |
| GET /health | 数据库 Ping；文本 ok / unavailable | HTTP 6062；200 / 503 |

参数错误 InvalidArgument，不存在 NotFound，库存不足 FailedPrecondition，超时 DeadlineExceeded，主动取消 Canceled，暂时数据库故障 Unavailable，其余 Internal。外部只得到固定安全说明，不泄露驱动原始错误。没有指标/pprof/订单 HTTP 接口。

### 3.6 配置变更

| 变量 | 含义 | 默认值 / 本阶段变化 |
|---|---|---|
| MYSQL_DSN | shop 连接凭据 | 必填；原有变量本阶段接线，禁止日志输出 |
| DB_MAX_OPEN_CONNS | 连接上限 | 20，允许 1–100 |
| DB_MAX_IDLE_CONNS | 空闲上限 | 10，必须不超过 open |
| DB_CONN_MAX_LIFETIME | 单连接寿命 | 5m，允许 1s–1h |
| GRPC_ADDR | 库存监听 | :9001 |
| ADMIN_ADDR | 库存管理监听 | :6062，替换阶段 0 示例里的占位 :6061 |
| INVENTORY_GRPC_TARGET | 宿主机 CLI 目标 | 127.0.0.1:9001 |
| TEST_INVENTORY_MYSQL_DSN | 库存测试连接 | 空则跳过；新增，专用库 |
| TEST_SEED_MYSQL_DSN | 造数测试连接 | 空则跳过；新增，专用库 |

新 TEST 变量已补入计划书第 5.8.4 节。种子预算 10 分钟、服务启动 30 秒、停机 10 秒在代码中有解释，不是假装可配置的环境变量。

### 3.7 部署变更

Compose 新增 inventory-service，依赖健康 MySQL；9001/6062 只映射 127.0.0.1，内存上限 128MiB，容器 stop_grace_period 15 秒覆盖应用 10 秒预算。多阶段 Dockerfile 按 CMD_NAME 编译，运行时使用非 root 用户；.dockerignore 排除 .env、Git 和个人文件。Redis/Kafka 仍只运行基础实例。

CI 保持 ubuntu-24.04 / Go 1.27.1，新增库存构建、四组隔离数据库测试、small 两次造数和 get/deduct/get 冒烟；所有生成物随源码提交，CI 不依赖在线协议生成。

## 四、测试

### 4.1 新增测试

<!-- TESTS_START -->
| 测试名 | 文件 | 验证场景 |
|---|---|---|
| TestHealth | [internal/platform/adminserver/server_test.go](../../internal/platform/adminserver/server_test.go) | TestHealth 使用 Recorder 直接调用 handler，不启动 TCP 服务，验证状态和探测上下文 |
| TestInterceptors | [internal/platform/grpcx/interceptor_test.go](../../internal/platform/grpcx/interceptor_test.go) | TestInterceptors 验证日志在恢复层外面，因此成功、普通错误、panic 都会记录最终状态且不泄密 |
| TestShutdownDeadline | [internal/platform/grpcx/shutdown_test.go](../../internal/platform/grpcx/shutdown_test.go) | TestShutdownDeadline 证明预算耗尽后 Stop 会取消在途 RPC，并且 Shutdown 等待自己的 goroutine 退出 |
| TestDatabaseConfig | [internal/shop/app/config_test.go](../../internal/shop/app/config_test.go) | TestDatabaseConfig 验证默认连接预算和互相约束的 open/idle，错误里不得回显 DSN |
| TestParseStock | [internal/shop/app/shopctl/stock_client_test.go](../../internal/shop/app/shopctl/stock_client_test.go) | TestParseStock 在无网络环境检查 CLI 边界，尤其防止 int64 到 int32 的静默截断 |
| TestGRPCErrorMapping | [internal/shop/inventory/grpc_server_test.go](../../internal/shop/inventory/grpc_server_test.go) | TestGRPCErrorMapping 覆盖契约全部错误码；包装后的错误也必须能映射，且不能泄漏底层文字 |
| TestGRPCRoundTrip | [internal/shop/inventory/grpc_server_test.go](../../internal/shop/inventory/grpc_server_test.go) | TestGRPCRoundTrip 用内存连接运行真实序列化与服务分派；单元测试不监听 TCP、不依赖外部服务 |
| TestIntegrationDeadline | [internal/shop/inventory/repository_integration_test.go](../../internal/shop/inventory/repository_integration_test.go) | TestIntegrationDeadline 验证真实 gRPC deadline 穿过 service，令驱动查询等待和服务端 handler 提前返回 |
| TestIntegrationConcurrentDeduct | [internal/shop/inventory/repository_integration_test.go](../../internal/shop/inventory/repository_integration_test.go) | TestIntegrationConcurrentDeduct 让 100 个 goroutine 同时争抢 50 件；真实 InnoDB 锁与条件更新决定胜负 |
| TestIntegrationStockBoundaries | [internal/shop/inventory/repository_integration_test.go](../../internal/shop/inventory/repository_integration_test.go) | TestIntegrationStockBoundaries 检查不存在、归还和整数上界，覆盖单条 UPDATE 的零影响行数分支 |
| TestDatabaseErrorClassification | [internal/shop/inventory/repository_test.go](../../internal/shop/inventory/repository_test.go) | TestDatabaseErrorClassification 区分暂时不可用和永久配置错误，防止所有 SQL 错误都变成可重试 |
| TestServiceValidation | [internal/shop/inventory/service_test.go](../../internal/shop/inventory/service_test.go) | TestServiceValidation 检查三种入口的边界，并证明仓储错误不会被业务层吞掉 |
| TestServiceCancellationAndError | [internal/shop/inventory/service_test.go](../../internal/shop/inventory/service_test.go) | TestServiceCancellationAndError 确认取消不访问仓储、仓储错误保持可识别，不被重新制造成普通字符串 |
| TestIntegrationSeed | [internal/shop/seed/seed_integration_test.go](../../internal/shop/seed/seed_integration_test.go) | TestIntegrationSeed 验证从已提交批次继续、重复跳过、保持用户改过的库存及五表数量一致 |
| TestIntegrationSeedRollback | [internal/shop/seed/seed_integration_test.go](../../internal/shop/seed/seed_integration_test.go) | TestIntegrationSeedRollback 故意制造第三张表的主键冲突，证明前面两张表不会留下半批数据 |
| TestMakeBatches | [internal/shop/seed/seed_test.go](../../internal/shop/seed/seed_test.go) | TestMakeBatches 验证两个 SKU 的五表基线互相对应；此测试只生成内存参数，不连接数据库 |
| TestCommandPackageDoc | [tools/commentcheck/check_test.go](../../tools/commentcheck/check_test.go) | TestCommandPackageDoc 验证 cmd 单文件入口的包说明例外，不放宽普通包或缺注释入口 |
| TestFileDescriptions | [tools/commentcheck/files_test.go](../../tools/commentcheck/files_test.go) | TestFileDescriptions 区分真正的文件导航、贴在 package 上的包文档，以及只有空标题的占位注释 |
| TestPackageInventory | [tools/commentcheck/files_test.go](../../tools/commentcheck/files_test.go) | TestPackageInventory 验证完整文件名匹配；测试文件与生成物不强制列出，但仍可出现在人工说明中 |
| TestMapMode | [tools/commentcheck/files_test.go](../../tools/commentcheck/files_test.go) | TestMapMode 核对所有文件都进入地图、相对链接能解析、生成物有标记，并拒绝相互冲突的输出模式 |
<!-- TESTS_END -->

既有 TestLocations 新增根 cmd/README 正例与子目录 README 反例；doc.go、extra.go 拒绝用例保留。既有注释负例补齐新的文件导航外壳，仍然各自只破坏被测规则。

### 4.2 命令与结果

`gofmt -l cmd gen internal tools` 无输出；`go vet ./...`、`staticcheck ./...` 通过；commentcheck 通过 70 个手写 Go 文件；archcheck 通过。加载四个 TEST DSN 后，`go test -race ./... -count=1 -timeout=180s -json` 统计 43 个顶层测试、15 个有测试的包全部通过，无测试函数跳过；另 5 个包显示 no test files 并正常构建。buf lint/generate 已通过，生成结果可重现。

真实数据库结果：100 个 goroutine 争抢 50 件，50 成功、50 不足、最终 0；100ms deadline 使原本 5 秒查询的客户端等待缩短为 100.5017ms，服务端返回 context canceled。small 基线 100/400，重跑保留修改后的余额 7；故意制造主键冲突证明一批五表回滚。完整命令和输出见 [notes](../notes.md#phase-01)。

## 五、工程验收结果

| 阶段 1 验收项 | 结论 | 证据 |
|---|---|---|
| 一条命令初始化并记录耗时 | 通过 | seed：50 万商品/库存、200 万订单，总耗时 75.651 秒；重跑 538ms 且跳过 |
| 有索引和删索引后的 EXPLAIN | 通过 | const / uk_stock_sku / rows=1 → ALL / NULL / rows=498780；恢复后再确认索引存在 |
| 并发扣减在 -race 下通过 | 通过 | 50 成功、50 不足、余额 0 |
| 追加：三层说明与代码地图 | 通过 | 70 个手写文件检查、72 文件地图；目录 README、doc.go 清单和两阶段回放齐全 |
| Compose 当前源码构建 | 通过 | 四个容器 healthy，/health 与库存 RPC 可用 |

本地结果的环境和版本见 notes；远端 CI 运行链接在推送后补入同一记录。学习验收不由这些测试代替。

## 六、偏离计划之处

没有扩大业务范围。唯一需要额外授权的阶段 0 逻辑调整是根 `cmd/README.md` 的精确放行，用户已明确批准，并要求同步计划书第 3.4 节；已完成。commentcheck 升级、三层说明和开发回放是用户明确追加的要求。原因、被放弃方案及范围详见 [决定记录](../decisions.md)。

## 七、已知问题与遗留事项

- 当前是本机明文 gRPC 靶场，没有业务认证或生产高可用；不能原样暴露公网。
- Deduct/Release 没有操作幂等键，超时可能伴随已提交写入；CLI 不作应用层重试。不能把 context 取消当成远端 SQL 已瞬间停止的证明。
- seed 要离线独占运行，只检查行数与连续编号，不是任意业务库的内容审计/修复工具；后续新订单运行后不要用它重建基线。
- 迁移 DDL 隐式提交；IF NOT EXISTS 处理可重入，不承诺整批原子性。
- 订单、缓存、消息消费与治理属于阶段 2；指标和调用链属于阶段 3，均未提前实现。
- 连接池和内存上限是本机初始配置，尚无吞吐/P99 结论。archcheck 是静态规则工具，不是运行时沙箱。

## 八、注释自查结论

按第 3.1/3.2 节逐项检查：目录说明交代职责、边界和阅读路线；所有普通手写 Go 文件首部解释用途、拆分和调用链，和 package 间留空行；17 个 doc.go 列出包内文件，包括测试及相关 SQL。新增 19 份分组目录 README，cmd 程序仍只含 main.go。生成物明确标出回源 proto 修改。

代码层补充了 SQL 索引/锁/事务、错误映射理由、context 传播、channel 的发送接收与关闭责任、参数转换范围、资源清理顺序。测试表格案例说明各自破坏什么；不再用“不阻塞、并发安全”等固定句式填满简单构造函数。人工复读又删掉了检查器和配置读取处多余的安全套话，保留确实影响使用方式的约束。

commentcheck 通过（70 个手写文件），它只证明结构，不替代人工判断。阶段 0 的 26 个 Go 文件经去注释/位置 AST 与 Go 指令比较一致；排除用户要求扩展的 commentcheck 原 4 文件和获准调整的 archcheck 2 文件。阶段 0 其余逻辑未改。

## 九、学习指引

阅读[阶段 1 学习材料](../learn/phase-01-inventory.md)：先跑环境与 RPC，再从 cmd → app → proto/接口 → 业务 → SQL 读通调用链，最后做索引、取消和说明检查实验。约 10 小时只是建议安排，是否学会以能独立解释必答问题为准。阶段 0 新增的开发回放也保留真实失败和返工过程。

## 十、下一阶段预告

用户学习并明确同意后，阶段 2 才实现订单 HTTP、缓存、Kafka 消费、补偿和服务治理。目前不需要申请大模型密钥，也没有启动阶段 2 开发。本分支完成后停下，合并决定由用户作出。
