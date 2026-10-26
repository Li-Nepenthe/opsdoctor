# 阶段 5 汇报书：医生接入层与工具层

## 一、总览

**一句话总结**：医生能够按系统接收告警、保存 pending 任务，并让登录用户通过同一套只读工具完成 S01 人工取证。

本阶段从 main `dcbfe7a` 创建 phase-05-doctor-tools。先用独立提交 `d4641f0` 修复四项阶段 4 验收遗留问题。随后实现 doctor、doctorctl、独立数据库、多系统接入和十六项取证工具。Redis 原先无认证的问题在用户明确批准后补齐，S04、S05 完整回归通过。没有接大模型、Agent 循环、页面、写工具或 MCP，没有合并 main，也没有勾学习验收。

| 阶段 5 的“做” | 完成情况 |
|---|---|
| Gin、登录、doctor-mysql、迁移 | 已完成；复用 auth，初始化账号不会覆盖已有密码 |
| 全部 TARGETS_DIR 配置、profile、多系统隔离 | 已完成；真实 shop、虚构 warehouse 测试；变量名及实际账号冲突均拒绝 |
| source 接口与适配器 | 已完成；Prometheus、Jaeger、Loki、MySQL、Redis、pprof |
| 全部只读取证工具 | 已完成；16 项统一校验、10s 超时、4096 字节结果；submit_report 由计划书明确列入阶段 6 |
| MySQL/Redis 最小查看权限 | 已完成；实际写入拒绝、连接额度、Redis 匿名与 key 访问拒绝均验证 |
| 每系统独立令牌、分组去重、pending 任务 | 已完成；真实 Alertmanager 到医生，任务绑定 shop |
| doctorctl 三个命令 | 已完成；通过医生 HTTP 使用同一个 Registry，不另开数据源池 |

## 二、新引入的技术与依赖

没有新增 Go 模块依赖，go.mod/go.sum 不变。下面列出本阶段新增用途和新增运行实例使用的既有版本。

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| Go / 标准库 testing/synctest、httptest | 1.27.1 | 运行程序、虚拟时钟、HTTP 契约测试 | 复用现有工具链，避免测试依赖调度速度 | 真等1ms后赌是否来得及 |
| Gin | v1.12.0 | doctor HTTP | 计划内框架，沿用公共中间件 | 另换Web框架 |
| go-yaml | v1.19.2 | 接入配置与部署检查 | 已有依赖，可拒绝未知配置字段 | 通过文本猜测YAML结构 |
| go-sql-driver/mysql | v1.10.1 | 医生库与只读池 | 原生 context、database/sql 池限制 | 每个CLI自行建池 |
| go-redis | v9.23.0 | 独立Redis只读客户端 | 已有驱动，明确配置账号和超时 | 运行redis-cli子进程 |
| JWT / bcrypt | v5.3.1 / x/crypto v0.58.0 | 登录 | 复用阶段0认证规则 | 自写密码算法 |
| MySQL 镜像 | 8.4.13 | 新 doctor-mysql | 与靶场同版本、独立实例和数据卷 | 与shop共用业务实例 |
| Go/Alpine 构建镜像 | 1.27.1-alpine3.24 / 3.24.2 | doctor和认证后的业务镜像 | 沿用固定构建链 | latest标签 |
| Redis 镜像 | 8.10.2 | 为现有实例补ACL认证 | 不换存储、不清数据 | 保留匿名全权限 |
| Jaeger 查询接口 | 2.20.0 | `/api/traces`搜索与详情 | 固定版本实际提供并已做契约测试 | 未验证就切换新版API |
| staticcheck / promtool | v0.8.2 / Prometheus v3.15.1 | 静态与规则测试 | 沿用固定版本 | 跳过规则测试 |
| doctor / 业务正常镜像 / S09缺陷镜像 | 20261026.1 / 20261026.1 / 20261026.2 | 本机验收 | 认证改动不能退回旧客户端 | 继续使用不认识Redis密码的旧实验镜像 |

Jaeger 仍是用户此前批准的唯一版本例外。本阶段没有引入计划外组件或付费调用。

## 三、代码明细

### 3.1 新增和修改的包

| 包路径 | 新增/修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| cmd/doctor、cmd/doctorctl | 新增 | 服务与终端入口 | 启动、退出码 |
| internal/doctor/app | 新增 | 配置、装配、关闭、CLI请求 | Run、RunCLI |
| internal/doctor/httpapi | 新增 | 登录、工具、任务、告警HTTP | New、Repository、Options |
| internal/doctor/target | 新增 | 加载全部系统，独立账号和工具 | Load、Catalog、System |
| internal/doctor/tools | 新增 | 16项菜单、校验、预算、摘要 | New、Registry、Definition、Result |
| internal/doctor/source 及六类适配器、httpbackend | 新增 | 数据形状与后端只读查询 | 六种接口、固定用途客户端 |
| internal/doctor/alerts | 新增 | 通知格式和保存入口 | Service、Receiver、Notification |
| internal/doctor/store | 新增 | 用户、任务、告警事务和迁移 | Store、Migrate、列表与详情 |
| internal/doctor/auth | 仅说明修改 | 复用认证 | 原有JWT、密码、邮箱接口不变 |
| internal/shop/app/orderservice | 修改 | 业务Redis认证配置 | 原有启动流程增加必填密码 |
| internal/shop/order、product | 仅测试修改 | 密码缺项明确跳过 | 生产逻辑不变 |
| internal/chaos/action、app、scenario | 修改 | Redis认证及认证版实验镜像 | 原有实验操作使用密码 |
| internal/platform/resilience | 仅测试修改 | 可控时钟修复1ms测试 | 业务重试策略不变 |
| internal/platform/alertsink | 修改 | 历史独立入口 | 令牌变量同步SHOP_ALERT_TOKEN，不再由Compose运行 |
| tools/archcheck | 修改 | 医生与实验隔离 | 导入、文件访问、Compose挂载检查 |

### 3.2 新增和修改的文件

以下覆盖本分支相对 main 的全部文件，行数为最终文件总行数，增删数按 Git 差异。原工作区已有的阶段 0 学习材料修改不属于本任务，未纳入提交。汇报书自身也列出；生成时同步自己的行数。

<!-- PHASE_FILES_START -->

| 文件 | 总行数 | Git 增删 | 作用 |
|---|---|---|---|
| `.env.example` | 124 | +32 / -12 | 全部环境变量说明与演示假值 |
| `.github/workflows/ci.yml` | 277 | +49 / -2 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `AGENTS.md` | 116 | +1 / -1 | AGENTS.md |
| `README.md` | 459 | +89 / -15 | OpsDoctor |
| `cmd/README.md` | 32 | +9 / -0 | cmd：程序入口 |
| `cmd/doctor/main.go` | 21 | +21 / -0 | 医生服务入口，只负责把启动结果变成进程退出码。 |
| `cmd/doctorctl/main.go` | 22 | +22 / -0 | 医生手动取证命令的入口。 |
| `deploy/README.md` | 25 | +2 / -0 | deploy：本机部署 |
| `deploy/alertmanager/README.md` | 12 | +2 / -2 | deploy/alertmanager：告警通知 |
| `deploy/alertmanager/alertmanager.yml` | 18 | +4 / -4 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/alloy/README.md` | 9 | +1 / -1 | deploy/alloy：日志收集 |
| `deploy/alloy/config.alloy` | 46 | +1 / -1 | // 只发现当前 Compose 项目的容器，避免顺带收集用户其他项目的日志。 |
| `deploy/docker-compose.yml` | 477 | +71 / -23 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/doctor/README.md` | 10 | +10 / -0 | 医生的部署配置 |
| `deploy/doctor/mysql-init/001_test_database.sh` | 11 | +11 / -0 | #!/bin/bash |
| `deploy/doctor/mysql-init/README.md` | 5 | +5 / -0 | 医生数据库的首次初始化 |
| `deploy/doctor/targets/README.md` | 7 | +7 / -0 | 系统接入目录 |
| `deploy/doctor/targets/shop.profile.md` | 43 | +43 / -0 | shop 的系统说明书 |
| `deploy/doctor/targets/shop.yaml` | 13 | +13 / -0 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/mysql/init/006_doctor_readonly.sh` | 40 | +40 / -0 | #!/bin/bash |
| `deploy/mysql/init/README.md` | 25 | +3 / -0 | deploy/mysql/init：首次启动脚本 |
| `deploy/prometheus/README.md` | 20 | +4 / -2 | deploy/prometheus：指标采集和告警规则 |
| `deploy/prometheus/prometheus.yml` | 36 | +5 / -2 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/prometheus/rules/README.md` | 63 | +14 / -1 | deploy/prometheus/rules：告警规则 |
| `deploy/prometheus/rules/goroutine_alert.test.yaml` | 71 | +71 / -0 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/prometheus/rules/shop.yml` | 95 | +7 / -1 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `deploy/redis/README.md` | 9 | +9 / -0 | Redis 的账号和启动方式 |
| `deploy/redis/start-with-acl.sh` | 29 | +29 / -0 | #!/bin/sh |
| `docs/decisions.md` | 324 | +42 / -0 | 设计决定 |
| `docs/learn/README.md` | 29 | +2 / -0 | docs/learn：逐阶段学习材料 |
| `docs/learn/phase-05-doctor-tools.md` | 478 | +478 / -0 | 阶段 5 学习材料：医生接入层与工具层 |
| `docs/notes.md` | 1024 | +134 / -0 | 实测记录 |
| `docs/reports/README.md` | 16 | +2 / -0 | docs/reports：阶段汇报书 |
| `docs/reports/phase-05-doctor-tools.md` | 678 | +678 / -0 | 阶段汇报、验收证据与代码清单 |
| `docs/代码地图.md` | 312 | +66 / -2 | OpsDoctor 代码地图 |
| `docs/数据台账.md` | 274 | +26 / -0 | 数据台账 |
| `eval/scenarios/S09.yaml` | 19 | +1 / -1 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `internal/README.md` | 27 | +1 / -1 | internal：项目的全部实现代码 |
| `internal/chaos/action/runner.go` | 97 | +8 / -7 | 把已检查的场景动作交给真正的外部系统执行。 |
| `internal/chaos/action/storage.go` | 125 | +4 / -1 | 改动明确允许的索引，以及商品缓存的有效时间。 |
| `internal/chaos/app/app.go` | 217 | +2 / -1 | 把 chaos 命令行接到场景、执行器和自动验收流程。 |
| `internal/chaos/scenario/validate.go` | 134 | +1 / -1 | 把场景操作限制在计划书规定的服务和资源内。 |
| `internal/doctor/README.md` | 19 | +13 / -11 | doctor：接收告警，提供查看工具 |
| `internal/doctor/alerts/doc.go` | 11 | +11 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/alerts/service.go` | 127 | +127 / -0 | 检查 Alertmanager 通知，并把合法的分组交给仓储保存。 |
| `internal/doctor/app/app.go` | 92 | +92 / -0 | 按顺序准备医生自己的数据库、接入系统和 HTTP 服务。 |
| `internal/doctor/app/cli.go` | 197 | +197 / -0 | 实现 doctorctl 的三个命令，通过 HTTP 使用正在运行的医生。 |
| `internal/doctor/app/cli_test.go` | 77 | +77 / -0 | 验证命令行始终把工具参数交给同一个医生接口。 |
| `internal/doctor/app/config.go` | 62 | +62 / -0 | 在医生开放端口前检查必填配置和初始登录账号。 |
| `internal/doctor/app/doc.go` | 15 | +15 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/auth/auth.go` | 141 | +1 / -1 | 医生登录要用的令牌与密码规则。 |
| `internal/doctor/auth/doc.go` | 22 | +3 / -3 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/httpapi/doc.go` | 14 | +14 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/httpapi/handlers.go` | 171 | +171 / -0 | 实现登录、告警接收、就绪检查和任务查询的 HTTP 细节。 |
| `internal/doctor/httpapi/router.go` | 120 | +120 / -0 | 把登录、系统工具和告警入口接到同一个 Gin 服务。 |
| `internal/doctor/httpapi/router_test.go` | 199 | +199 / -0 | 在内存 HTTP 请求里检查登录、系统令牌和工具入口。 |
| `internal/doctor/source/README.md` | 17 | +17 / -0 | source：把不同后端的证据交给医生 |
| `internal/doctor/source/doc.go` | 17 | +17 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/httpbackend/client.go` | 99 | +99 / -0 | 为只读 HTTP 数据源固定地址、等待时间和响应大小。 |
| `internal/doctor/source/httpbackend/client_test.go` | 72 | +72 / -0 | 验证监控 HTTP 请求不会逃到其他地址，也不会无限读取。 |
| `internal/doctor/source/httpbackend/doc.go` | 13 | +13 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/jaeger/client.go` | 179 | +179 / -0 | 读取本项目固定的 Jaeger 2.20.0 查询接口。 |
| `internal/doctor/source/jaeger/client_test.go` | 122 | +122 / -0 | 用 Jaeger 2.20.0 的真实样例检查查询契约，并补异常边界。 |
| `internal/doctor/source/jaeger/doc.go` | 16 | +16 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/jaeger/testdata/README.md` | 7 | +7 / -0 | Jaeger 契约样例 |
| `internal/doctor/source/jaeger/testdata/detail.json` | 1306 | +1306 / -0 | 真实 Jaeger 返回结构的契约测试样例（连接字符串已去除） |
| `internal/doctor/source/jaeger/testdata/search.json` | 1306 | +1306 / -0 | 真实 Jaeger 返回结构的契约测试样例（连接字符串已去除） |
| `internal/doctor/source/loki/client.go` | 104 | +104 / -0 | 按结构化条件搜索一个系统的 Loki 日志。 |
| `internal/doctor/source/loki/client_test.go` | 59 | +59 / -0 | 验证日志查询中的文字不会被解释成新的 LogQL。 |
| `internal/doctor/source/loki/doc.go` | 13 | +13 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/mysqlinspect/client.go` | 191 | +191 / -0 | 用一个最多三条连接的只读账号检查 MySQL。 |
| `internal/doctor/source/mysqlinspect/client_test.go` | 190 | +190 / -0 | 用 database/sql 的假驱动检查查询模板和连接排队预算。 |
| `internal/doctor/source/mysqlinspect/doc.go` | 17 | +17 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/mysqlinspect/permissions_integration_test.go` | 142 | +142 / -0 | 用真实 diag_ro 账号证明“能看执行计划，不能写业务表”。 |
| `internal/doctor/source/pprof/client.go` | 131 | +131 / -0 | 读取配置中某个服务的 goroutine 文本，并按堆栈合并数量。 |
| `internal/doctor/source/pprof/client_test.go` | 52 | +52 / -0 | 验证只查已配置的服务，并把相同函数栈的数量合并。 |
| `internal/doctor/source/pprof/doc.go` | 13 | +13 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/prometheus/client.go` | 153 | +153 / -0 | 把医生的指标和告警查询转换成 Prometheus HTTP API 请求。 |
| `internal/doctor/source/prometheus/client_test.go` | 74 | +74 / -0 | 用固定 HTTP 响应检查 Prometheus 适配器的契约。 |
| `internal/doctor/source/prometheus/doc.go` | 13 | +13 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/redisinspect/client.go` | 129 | +129 / -0 | 通过独立 Redis ACL 账号读取运行信息。 |
| `internal/doctor/source/redisinspect/client_test.go` | 161 | +161 / -0 | 用一个只会回复固定消息的 Redis 协议小服务器检查适配器。 |
| `internal/doctor/source/redisinspect/doc.go` | 14 | +14 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/source/redisinspect/permissions_integration_test.go` | 102 | +102 / -0 | 检查真实 Redis 的认证和只读账号权限。 |
| `internal/doctor/source/select.go` | 108 | +108 / -0 | 限制执行计划工具接受的 SELECT 文本。 |
| `internal/doctor/source/select_test.go` | 47 | +47 / -0 | 验证执行计划工具接受的是单条简单 SELECT，而不是任意 SQL。 |
| `internal/doctor/source/source.go` | 147 | +147 / -0 | 约定医生能从外部系统读取哪些证据。 |
| `internal/doctor/store/alerts.go` | 91 | +91 / -0 | 在一个事务中去重告警分组、创建任务、保存告警快照。 |
| `internal/doctor/store/diagnoses.go` | 94 | +94 / -0 | 读取已经创建的诊断任务，供验收和后续页面使用。 |
| `internal/doctor/store/doc.go` | 16 | +16 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/store/migrations/0001_users.sql` | 10 | +10 / -0 | 延续原项目 M1 的用户表结构，只保存 bcrypt 摘要，不保存明文密码。 |
| `internal/doctor/store/migrations/0002_diagnoses.sql` | 26 | +26 / -0 | 这里只创建 pending 任务。模型、报告、租约等字段保留计划书格式，但阶段 5 不执行它们。 |
| `internal/doctor/store/migrations/0003_diagnosis_alerts.sql` | 9 | +9 / -0 | 指纹在同一个任务中唯一。重复通知更新该告警快照，不复制任务或堆积相同记录。 |
| `internal/doctor/store/migrations/README.md` | 7 | +7 / -0 | 医生自己的表 |
| `internal/doctor/store/store.go` | 46 | +46 / -0 | 建立医生仓储并应用医生自己的迁移。 |
| `internal/doctor/store/store_integration_test.go` | 168 | +168 / -0 | 在专用医生测试库验证迁移、通知事务和分组去重。 |
| `internal/doctor/store/users.go` | 36 | +36 / -0 | 保存首次登录账号，并按邮箱查找密码摘要。 |
| `internal/doctor/target/catalog.go` | 248 | +248 / -0 | 为每个系统分别创建数据源和工具注册表。 |
| `internal/doctor/target/catalog_test.go` | 175 | +175 / -0 | 证明两个系统的工具和告警令牌不会串用。 |
| `internal/doctor/target/config.go` | 129 | +129 / -0 | 读取一个目录里的全部系统配置，并检查系统之间有没有共用凭据。 |
| `internal/doctor/target/doc.go` | 14 | +14 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/target/testdata/README.md` | 5 | +5 / -0 | 第二个系统的测试配置 |
| `internal/doctor/target/testdata/warehouse.profile.md` | 4 | +4 / -0 | Warehouse（虚构的测试系统） |
| `internal/doctor/target/testdata/warehouse.yaml` | 5 | +5 / -0 | 部署/测试/场景的固定配置；用途见相邻 README 与本报告 |
| `internal/doctor/tools/catalog.go` | 93 | +93 / -0 | 根据一个系统实际配置的数据源，建立它自己的只读工具菜单。 |
| `internal/doctor/tools/catalog_logs.go` | 32 | +32 / -0 | 登记由代码构造查询语句的日志搜索。 |
| `internal/doctor/tools/catalog_metrics.go` | 49 | +49 / -0 | 登记告警、指标、消息积压和变更查询。 |
| `internal/doctor/tools/catalog_mysql.go` | 52 | +52 / -0 | 登记六种用途固定的 MySQL 查看工具。 |
| `internal/doctor/tools/catalog_runtime.go` | 39 | +39 / -0 | 登记 Redis 运行信息和 goroutine 堆栈摘要。 |
| `internal/doctor/tools/catalog_test.go` | 64 | +64 / -0 | 验证完整工具菜单的共同边界，以及调用链摘要遇到坏数据时的行为。 |
| `internal/doctor/tools/catalog_traces.go` | 33 | +33 / -0 | 登记慢请求搜索与单条调用链树。 |
| `internal/doctor/tools/doc.go` | 25 | +25 / -0 | 包背景、边界、完整文件清单和阅读路线 |
| `internal/doctor/tools/registry.go` | 202 | +202 / -0 | 让所有工具共用参数校验、十秒预算和四千字节结果限制。 |
| `internal/doctor/tools/registry_test.go` | 162 | +162 / -0 | 验证工具统一入口的参数、超时、截断和系统绑定。 |
| `internal/doctor/tools/schema.go` | 153 | +153 / -0 | 用同一份字段描述生成 JSON Schema，并校验实际工具参数。 |
| `internal/doctor/tools/summary.go` | 222 | +222 / -0 | 把长时间线和重复的调用步骤整理成可阅读的证据摘要。 |
| `internal/platform/alertsink/app.go` | 75 | +2 / -2 | 启动临时告警接收器，并在退出时排空已接收的请求。 |
| `internal/platform/resilience/resilience_test.go` | 140 | +26 / -20 | 验证入口限流、重试边界和熔断恢复。 |
| `internal/shop/app/orderservice/config.go` | 129 | +2 / -1 | 读取订单进程的配置，并检查各层时间预算。 |
| `internal/shop/app/orderservice/config_test.go` | 99 | +10 / -0 | 验证订单服务的治理参数和时间预算关系。 |
| `internal/shop/app/orderservice/dependencies.go` | 154 | +3 / -3 | 准备订单进程需要的连接，并明确由谁关闭它们。 |
| `internal/shop/order/cache_integration_test.go` | 175 | +6 / -1 | 用真实 Redis 验证幂等占位与原子更新。 |
| `internal/shop/product/cache_integration_test.go` | 78 | +6 / -1 | 在真实 Redis 中检查商品缓存的内容和过期规则。 |
| `tools/archcheck/check.go` | 417 | +17 / -2 | 按目录、导入和文件访问规则检查架构边界。 |
| `tools/archcheck/check_test.go` | 157 | +8 / -0 | 用故意违规的假模块检验架构规则。 |
| `tools/archcheck/doc.go` | 25 | +7 / -5 | 包背景、边界、完整文件清单和阅读路线 |
| `tools/archcheck/doctor.go` | 58 | +58 / -0 | 检查医生容器能从宿主机拿到什么。 |
| `tools/archcheck/doctor_test.go` | 48 | +48 / -0 | 证明容器不能通过部署配置拿到故障答案或 Docker 控制权。 |
| `进度.md` | 163 | +14 / -14 | OpsDoctor 进度看板 |
| `项目计划书.md` | 2073 | +13 / -2 | OpsDoctor 项目计划书：Go 故障诊断 Agent + 可复现故障靶场 |

<!-- PHASE_FILES_END -->

### 3.3 类型与函数清单

初稿来自 `commentcheck -list ./...`，按本阶段改动文件筛选，再补调用方。为便于顺着阅读，修改文件中仍保留的导出入口和关键内部步骤也列在表里，并不意味着每个列出的函数都是新写的。测试函数另见第四节。

<!-- PHASE_SYMBOLS_START -->

| 包 | 名称 | 位置（文件:行:列） | 作用 | 被谁调用 |
|---|---|---|---|---|
| cmd/doctor (main) | `main` | `cmd/doctor/main.go:16:1` | main 把 os.Exit 放在 Run 返回之后，避免跳过数据库和 HTTP 的清理 | Go 运行时入口 |
| cmd/doctorctl (main) | `main` | `cmd/doctorctl/main.go:17:1` | main 不接收密码参数，登录令牌和医生地址都由环境变量提供 | Go 运行时入口 |
| internal/chaos/action (action) | `Runner` | `internal/chaos/action/runner.go:31:6` | Runner 只保存配置，不长时间占用数据库连接 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `*Runner.Execute` | `internal/chaos/action/runner.go:43:1` | Execute 再次检查参数，防止调用方绕过 YAML 加载器后执行任意操作 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `*Runner.Command` | `internal/chaos/action/runner.go:72:1` | Command 使用参数数组，YAML 里的值不会被 PowerShell 或 bash 当成代码 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `*Runner.compose` | `internal/chaos/action/runner.go:94:1` | compose 固定配置文件与项目目录，只接受内部代码构造的子命令 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `*Runner.index` | `internal/chaos/action/storage.go:28:1` | index 不接受任意 SQL | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `expectedIndex` | `internal/chaos/action/storage.go:63:1` | expectedIndex 不只检查名字，还核对列顺序和唯一性，避免把同名但结构不同的索引误删 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/action (action) | `*Runner.cache` | `internal/chaos/action/storage.go:86:1` | cache 使用 pipeline 一次发送多个独立命令，减少宿主机逐次往返造成的到期时间差 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/app (app) | `*Engine.operate` | `internal/chaos/app/app.go:122:1` | operate 用已有记录匹配 verify 和 restore，防止把另一场景的效果误当成当前场景 | app.Run 及本包装配步骤 |
| internal/chaos/app (app) | `overrides` | `internal/chaos/app/app.go:168:1` | overrides 只覆盖对应动作已有的参数；没有这种动作的场景会拒绝无效的覆盖选项 | app.Run 及本包装配步骤 |
| internal/chaos/app (app) | `Run` | `internal/chaos/app/app.go:33:1` | Run 不隐式启动流量 | cmd/doctor.main |
| internal/chaos/app (app) | `newEngine` | `internal/chaos/app/app.go:77:1` | newEngine 只读取密码，不输出它 | app.Run 及本包装配步骤 |
| internal/chaos/app (app) | `*Engine.general` | `internal/chaos/app/app.go:98:1` | general 处理无需场景编号的命令 | app.Run 及本包装配步骤 |
| internal/chaos/scenario (scenario) | `validateInteger` | `internal/chaos/scenario/validate.go:116:1` | validateInteger 提前使用服务端同样的单项边界，避免明知无效还去重建容器 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/scenario (scenario) | `ValidateStep` | `internal/chaos/scenario/validate.go:21:1` | ValidateStep 先限制动作类型，再检查必要参数 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/chaos/scenario (scenario) | `ValidateEnvironment` | `internal/chaos/scenario/validate.go:81:1` | ValidateEnvironment 不接受 DSN、口令或任意环境变量，只允许实验所需的公开配置 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/doctor/alerts (alerts) | `validLabels` | `internal/doctor/alerts/service.go:101:1` | validLabels 限制单字段和数量，整体正文另由 HTTP 的一 MiB 上限限制 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Title` | `internal/doctor/alerts/service.go:114:1` | Title 只用于列表展示，数据库身份始终是系统名与分组键，不用可变的标题去重 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Alert` | `internal/doctor/alerts/service.go:23:6` | Alert 保存证据快照；Labels、Annotations 是外部文字，以后也不能把它们当指令 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Notification` | `internal/doctor/alerts/service.go:33:6` | Notification 的 GroupKey 是推送方的分组键，仓储还会加上系统名形成去重身份 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Receipt` | `internal/doctor/alerts/service.go:41:6` | Receipt 说明通知已保存或被忽略，不表示诊断已经执行 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Receiver` | `internal/doctor/alerts/service.go:48:6` | Receiver 定义在通知处理这一使用方，MySQL 仓储只需实现它，不需要显式 implements | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Service` | `internal/doctor/alerts/service.go:53:6` | Service 不保留某个“默认系统”，每次身份都由经过认证的接口传入 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `New` | `internal/doctor/alerts/service.go:56:1` | New 让测试可以替换仓储，同时生产路径仍只有同一个通知校验入口 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `*Service.Receive` | `internal/doctor/alerts/service.go:59:1` | Receive 校验错误与存储错误分开，HTTP 层才能分别返回 400 和可重试的 503 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Invalid` | `internal/doctor/alerts/service.go:70:6` | Invalid 表示通知格式错误；错误文字由代码生成，不包含请求正文 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `Invalid.Error` | `internal/doctor/alerts/service.go:73:1` | Error 让 Invalid 可作为标准 error 返回，接口层通过 errors.As 判断分类 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/alerts (alerts) | `validate` | `internal/doctor/alerts/service.go:76:1` | validate 不信任推送内容的条数和字段长度，认证通过也不意味着可以无限占用数据库 | httpapi.receiveAlert、store.ReceiveAlerts |
| internal/doctor/app (app) | `Run` | `internal/doctor/app/app.go:35:1` | Run 只有装配完全成功才监听端口，启动失败也会关闭已经创建的资源 | cmd/doctor.main |
| internal/doctor/app (app) | `serve` | `internal/doctor/app/app.go:76:1` | serve 停止接新请求后等待在途请求；工具最多十秒，停机预算再多留两秒做响应收尾 | Run |
| internal/doctor/app (app) | `request` | `internal/doctor/app/cli.go:133:1` | request 固定相对路径，不允许医生接口通过重定向把登录令牌带到另一个服务 | RunCLI |
| internal/doctor/app (app) | `printList` | `internal/doctor/app/cli.go:174:1` | printList 从服务端给出的菜单选择，不根据本机接入文件猜测可用工具 | RunCLI |
| internal/doctor/app (app) | `cliCommand` | `internal/doctor/app/cli.go:33:6` | cliCommand 不允许 URL、主机、令牌等命令参数；这些是操作者的进程环境配置 | app.Run 及本包装配步骤 |
| internal/doctor/app (app) | `RunCLI` | `internal/doctor/app/cli.go:41:1` | RunCLI 的输出可以重定向保存；凭据只放 Authorization 头，不打印请求转储 | cmd/doctorctl.main |
| internal/doctor/app (app) | `parseCommand` | `internal/doctor/app/cli.go:82:1` | parseCommand 自己读少量固定选项，允许 --args 放在工具名之后，符合文档示例 | RunCLI |
| internal/doctor/app (app) | `settings` | `internal/doctor/app/config.go:26:6` | settings 不能用于结构化日志；database 和 jwtSecret 都包含敏感配置 | app.Run 及本包装配步骤 |
| internal/doctor/app (app) | `loadConfig` | `internal/doctor/app/config.go:36:1` | loadConfig 一次收集所有缺项；密码错误只解释长度要求，不输出输入 | app.Run |
| internal/doctor/auth (auth) | `NormalizeEmail` | `internal/doctor/auth/auth.go:106:1` | NormalizeEmail 返回规范化邮箱 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `HashPassword` | `internal/doctor/auth/auth.go:122:1` | HashPassword 用 bcrypt 的默认成本计算摘要 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `VerifyPassword` | `internal/doctor/auth/auth.go:136:1` | VerifyPassword 比较摘要和输入 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `Error` | `internal/doctor/auth/auth.go:28:6` | Error 是可比较的领域错误 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `Error.Error` | `internal/doctor/auth/auth.go:32:1` | Error 让这个具名字符串类型满足标准库 error 接口 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `Tokens` | `internal/doctor/auth/auth.go:51:6` | Tokens 保存 JWT 签名配置 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `NewTokens` | `internal/doctor/auth/auth.go:57:1` | NewTokens 在程序启动时拒绝过短密钥或不合理有效期，避免请求来了才发现签名配置不能用 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `*Tokens.Issue` | `internal/doctor/auth/auth.go:69:1` | Issue 把已经确认的用户 ID 写进签名令牌 | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/auth (auth) | `*Tokens.Parse` | `internal/doctor/auth/auth.go:91:1` | Parse 同时校验签名算法、签发方、受众、有效期和用户标识，成功返回 userID | httpapi 登录/鉴权及 app 配置 |
| internal/doctor/httpapi (httpapi) | `*api.ready` | `internal/doctor/httpapi/handlers.go:107:1` | ready 只检查医生自己的持久化依赖，靶场正在故障不能让接收告警的医生也变成不可用 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.listDiagnoses` | `internal/doctor/httpapi/handlers.go:118:1` | listDiagnoses 限制页长与偏移量；未知系统不能静默返回全部系统的任务 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.diagnosis` | `internal/doctor/httpapi/handlers.go:153:1` | diagnosis 只接受本服务生成的 ID，查询失败时不泄露 SQL 或数据库连接细节 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.login` | `internal/doctor/httpapi/handlers.go:34:1` | login 对未知用户也执行一次摘要比较，避免用响应快慢直接枚举邮箱 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.receiveAlert` | `internal/doctor/httpapi/handlers.go:76:1` | receiveAlert 的 target 来自路径并通过令牌验证，不接受正文中另一个系统名作为覆盖 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.runTool` | `internal/doctor/httpapi/router.go:106:1` | runTool 是 doctorctl 与未来其他入口共用的调用路径，不能直接调用适配器 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `Repository` | `internal/doctor/httpapi/router.go:33:6` | Repository 定义在接口使用方，测试可以观察持久化是否被调用，而不启动 MySQL | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `Options` | `internal/doctor/httpapi/router.go:41:6` | Options 只接收已经装配的对象，不读取环境变量或创建第二套数据库池 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `api` | `internal/doctor/httpapi/router.go:52:6` | api 持有服务范围的登录限流器，不能让猜密码请求无限占用 bcrypt 的 CPU | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `New` | `internal/doctor/httpapi/router.go:59:1` | New 不挂载页面或 Agent 路由，阶段 5 只开放用户手动取证需要的接口 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.requireLogin` | `internal/doctor/httpapi/router.go:78:1` | requireLogin 不把解析失败的细节回显给客户端，令牌原文也不会写入日志 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/httpapi (httpapi) | `*api.listTargets` | `internal/doctor/httpapi/router.go:92:1` | listTargets 只返回名称和工具说明，不把接入地址、账号变量或令牌发给客户端 | Gin 路由及 httpapi.New 组装的处理函数 |
| internal/doctor/source (source) | `SelectStatement` | `internal/doctor/source/select.go:21:1` | SelectStatement 返回规范化后的单条 SELECT；工具和 MySQL 适配器都要调用 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `sqlWords` | `internal/doctor/source/select.go:42:1` | sqlWords 识别引号边界，同时拒绝任意函数调用，避免让存储函数在优化阶段产生副作用 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `quotedEnd` | `internal/doctor/source/select.go:89:1` | quotedEnd 跳过一段带引号的内容，避免把字符串里的词误当 SQL 指令 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Span` | `internal/doctor/source/source.go:102:6` | Span 把后端不同的引用格式统一成一个父节点，便于工具展示调用树 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Trace` | `internal/doctor/source/source.go:113:6` | Trace 不要求只有一个根：异步消息或采样丢失都可能留下不完整关系 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `LogQuery` | `internal/doctor/source/source.go:119:6` | LogQuery 描述要找的日志，而不让调用者直接拼查询语言 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `LogEntry` | `internal/doctor/source/source.go:130:6` | LogEntry 保留原始日志文本 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Table` | `internal/doctor/source/source.go:137:6` | Table 只用于执行计划和元数据 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `StackGroup` | `internal/doctor/source/source.go:144:6` | StackGroup 把停在同一处的 goroutine 合并，避免几万个相同堆栈挤满结果 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `MetricsSource` | `internal/doctor/source/source.go:21:6` | MetricsSource 把“查指标”和“看告警”放在同一个系统绑定的数据源上 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `TraceSource` | `internal/doctor/source/source.go:27:6` | TraceSource 既能找慢请求，也能沿同一个请求继续查看上下游 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `LogSource` | `internal/doctor/source/source.go:33:6` | LogSource 接受结构化条件；适配器自己拼查询语言，不让调用者直接写 LogQL | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `MySQLInspector` | `internal/doctor/source/source.go:38:6` | MySQLInspector 刻意没有 Query 或 Exec 这种万能入口，每个方法只做一种查看操作 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `RedisInspector` | `internal/doctor/source/source.go:48:6` | RedisInspector 不提供按 key 读取内容的方法，避免诊断变成查看业务数据 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `ProfileSource` | `internal/doctor/source/source.go:53:6` | ProfileSource 只能访问配置中的服务管理端口，不能由参数选择主机 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `MetricQuery` | `internal/doctor/source/source.go:58:6` | MetricQuery 使用绝对时间，便于在测试里固定同一个查询窗口 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Point` | `internal/doctor/source/source.go:66:6` | Point 的值保留字符串，因为 Prometheus 也可能返回 NaN 或无穷大 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Series` | `internal/doctor/source/source.go:72:6` | Series 是同一组标签对应的一条时间线 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `MetricResult` | `internal/doctor/source/source.go:78:6` | MetricResult 说明是否碰到后端返回数量上限，避免把局部结果说成全量 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `Alert` | `internal/doctor/source/source.go:84:6` | Alert 只留下定位症状的内容，不把后端提供的链接当成可执行地址 | tools 菜单与各后端适配器 |
| internal/doctor/source (source) | `TraceQuery` | `internal/doctor/source/source.go:92:6` | TraceQuery 限定在哪个服务中寻找近期的慢请求 | tools 菜单与各后端适配器 |
| internal/doctor/source/httpbackend (httpbackend) | `Client` | `internal/doctor/source/httpbackend/client.go:29:6` | Client 的地址只在构造时写入，工具请求不能更改它 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/httpbackend (httpbackend) | `New` | `internal/doctor/source/httpbackend/client.go:35:1` | New 不接受配置中的账号、查询串或片段；这些内容不属于监控服务的根地址 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/httpbackend (httpbackend) | `*Client.Get` | `internal/doctor/source/httpbackend/client.go:51:1` | Get 在有界读取后解码 JSON | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/httpbackend (httpbackend) | `*Client.Bytes` | `internal/doctor/source/httpbackend/client.go:63:1` | Bytes 只允许适配器提供相对 API 路径，不使用会把绝对 URL 当新地址的 ResolveReference | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/httpbackend (httpbackend) | `*Client.Close` | `internal/doctor/source/httpbackend/client.go:99:1` | Close 释放空闲连接，测试和停机时都不应把后端连接留给垃圾回收来碰运气 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/jaeger (jaeger) | `*Client.read` | `internal/doctor/source/jaeger/client.go:108:1` | read 同时检查 HTTP 和业务错误 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `decode` | `internal/doctor/source/jaeger/client.go:131:1` | decode 不把 process ID 当成服务名，也不把 FOLLOWS_FROM 误当成树的直接父节点 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `tagAllowed` | `internal/doctor/source/jaeger/client.go:171:1` | tagAllowed 只保留定位服务、SQL 模板、RPC 和错误状态所需的字段 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `Client` | `internal/doctor/source/jaeger/client.go:29:6` | Client 始终使用创建时绑定的 Jaeger，trace ID 不参与选择后端 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `New` | `internal/doctor/source/jaeger/client.go:32:1` | New 在启动时发现地址错误，避免等诊断开始才暴露配置问题 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/jaeger (jaeger) | `*Client.Close` | `internal/doctor/source/jaeger/client.go:41:1` | Close 释放空闲 HTTP 连接，停机后不继续占用后端资源 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/jaeger (jaeger) | `wireTrace` | `internal/doctor/source/jaeger/client.go:44:6` | wireTrace 只解码用得上的字段 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `*Client.Search` | `internal/doctor/source/jaeger/client.go:70:1` | Search 的起止时间和最小耗时分别使用 Jaeger 约定的微秒和时长字符串 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/jaeger (jaeger) | `*Client.Get` | `internal/doctor/source/jaeger/client.go:93:1` | Get 只接受十六或三十二位十六进制 ID，不能把路径、主机或查询串塞进这里 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/loki (loki) | `Client` | `internal/doctor/source/loki/client.go:27:6` | Client 绑定一个日志后端，工具不能切换租户头或服务器地址 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/loki (loki) | `New` | `internal/doctor/source/loki/client.go:30:1` | New 复用有界 HTTP 客户端，日志后端故障不能让请求无限等待 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/loki (loki) | `*Client.Close` | `internal/doctor/source/loki/client.go:39:1` | Close 在停机时释放空闲连接 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/loki (loki) | `*Client.Search` | `internal/doctor/source/loki/client.go:42:1` | Search 不使用调用者的字符串作为完整查询，且不会请求 Loki 的管理或写入接口 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/loki (loki) | `expression` | `internal/doctor/source/loki/client.go:89:1` | expression 用 Quote 把用户文字放进字符串字面量，不能让它变成查询运算符 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.TopQueries` | `internal/doctor/source/mysqlinspect/client.go:107:1` | TopQueries 只读把具体值换成问号的 DIGEST_TEXT，避免泄露样例 SQL 里的业务内容 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.LockWaits` | `internal/doctor/source/mysqlinspect/client.go:118:1` | LockWaits 直接读等待关系，不读取 LOCK_DATA 或正在执行的原始 SQL | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.Status` | `internal/doctor/source/mysqlinspect/client.go:128:1` | Status 只返回诊断需要的连接、线程和行锁计数，不开放任意 SHOW 命令 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.query` | `internal/doctor/source/mysqlinspect/client.go:134:1` | query 的 context 同时覆盖等池连接和读结果；最多十秒，不额外给排队续时 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `safeError` | `internal/doctor/source/mysqlinspect/client.go:177:1` | safeError 保留错误号方便排查授权，不回显可能带 SQL 或账号信息的数据库原文 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `tableName` | `internal/doctor/source/mysqlinspect/client.go:189:1` | tableName 接受常规表名；真正的库名只来自构造配置 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `Client` | `internal/doctor/source/mysqlinspect/client.go:29:6` | Client 不暴露底层 sql.DB，调用者只能使用下面六种查看方法 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `New` | `internal/doctor/source/mysqlinspect/client.go:35:1` | New 不立即联网 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `prepare` | `internal/doctor/source/mysqlinspect/client.go:53:1` | prepare 把连接串中的选项收窄到查看工具需要的范围，防止部署误配打开多语句执行 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.Close` | `internal/doctor/source/mysqlinspect/client.go:77:1` | Close 由整体装配负责调用，工具执行完只归还连接，不关闭共享池 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.Explain` | `internal/doctor/source/mysqlinspect/client.go:80:1` | Explain 永远自己加前缀，调用者不能选择 EXPLAIN ANALYZE 去实际执行查询 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.Indexes` | `internal/doctor/source/mysqlinspect/client.go:89:1` | Indexes 的表名是绑定参数，不拼成 SQL 标识符，名字里有反引号也无法改变语句 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.TableStats` | `internal/doctor/source/mysqlinspect/client.go:98:1` | TableStats 读取的是估计行数和空间用量，不对业务表执行 COUNT(*) 扫描 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/pprof (pprof) | `sortedGroups` | `internal/doctor/source/pprof/client.go:119:1` | sortedGroups 把最常见的堆栈排在前面，同数量时按文本排序，方便比较两次采样 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/pprof (pprof) | `Client` | `internal/doctor/source/pprof/client.go:30:6` | Client 复制配置映射，调用方之后修改原 map 也不能改变已绑定的端点 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/pprof (pprof) | `New` | `internal/doctor/source/pprof/client.go:35:1` | New 遇到任一地址错误就关闭已创建的客户端，不留下半套配置 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/pprof (pprof) | `*Client.Close` | `internal/doctor/source/pprof/client.go:49:1` | Close 释放全部服务的连接；空映射也能正常关闭 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/pprof (pprof) | `*Client.Goroutines` | `internal/doctor/source/pprof/client.go:56:1` | Goroutines 必须先找到服务，不能把未知名称当成主机名尝试连接 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/pprof (pprof) | `parse` | `internal/doctor/source/pprof/client.go:69:1` | parse 使用 debug=1 已带的组内数量，再把仅地址不同的函数栈合并 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/prometheus (prometheus) | `point` | `internal/doctor/source/prometheus/client.go:116:1` | point 不假设后端一定返回两个元素，坏响应不能造成越界 panic | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/prometheus (prometheus) | `*Client.Alerts` | `internal/doctor/source/prometheus/client.go:126:1` | Alerts 读取实际告警状态，不根据某个指标值自行猜测规则是否已经触发 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/prometheus (prometheus) | `Client` | `internal/doctor/source/prometheus/client.go:28:6` | Client 只持有一个后端，Query 没有临时换地址的入口 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/prometheus (prometheus) | `New` | `internal/doctor/source/prometheus/client.go:31:1` | New 在启动时检查地址；它不需要先查一次数据才能建立只读客户端 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/prometheus (prometheus) | `*Client.Close` | `internal/doctor/source/prometheus/client.go:40:1` | Close 关闭空闲连接，方便整体停止医生时释放各系统的资源 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/prometheus (prometheus) | `*Client.Query` | `internal/doctor/source/prometheus/client.go:43:1` | Query 同时在后端请求和本地解码限制数量，不能只相信调用方已经限过一次 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/prometheus (prometheus) | `decodeResult` | `internal/doctor/source/prometheus/client.go:72:1` | decodeResult 把不同查询结果转成相同结构，同时限制返回的序列和采样点 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `parseInfo` | `internal/doctor/source/redisinspect/client.go:103:1` | parseInfo 把每行 key:value 转成字段，保留值中其余的冒号（例如复制节点地址） | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `readError` | `internal/doctor/source/redisinspect/client.go:124:1` | readError 不回显 Redis 的命令错误原文，避免把被拒绝的参数或账号细节带给调用者 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `Client` | `internal/doctor/source/redisinspect/client.go:26:6` | Client 的底层 Redis 客户端不导出，调用者无法借它执行任意命令 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `New` | `internal/doctor/source/redisinspect/client.go:29:1` | New 只接受带独立 ACL 用户的 URL，凭据始终留在配置里，不出现在错误中 | target.bindHTTP/bindNative（HTTP公共客户端由各适配器调用） |
| internal/doctor/source/redisinspect (redisinspect) | `*Client.Close` | `internal/doctor/source/redisinspect/client.go:50:1` | Close 由系统装配统一调用，单次工具执行只归还连接 | target.Catalog.Close 或构造失败清理 |
| internal/doctor/source/redisinspect (redisinspect) | `*Client.Info` | `internal/doctor/source/redisinspect/client.go:53:1` | Info 不把 section 拼成命令文本；未知分区在联网前被拒绝 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `slowSummary` | `internal/doctor/source/redisinspect/client.go:72:6` | slowSummary 只保留排查耗时需要的字段，类型里没有业务参数的位置 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/source/redisinspect (redisinspect) | `*Client.slowlog` | `internal/doctor/source/redisinspect/client.go:81:1` | slowlog 在结果进入通用工具层之前就删掉参数，不能依赖最后的 4KB 截断来保密 | 对应 source 接口的工具闭包，或本包查询/解码步骤 |
| internal/doctor/store (store) | `*Store.ReceiveAlerts` | `internal/doctor/store/alerts.go:27:1` | ReceiveAlerts 的事务失败会整体回滚；发送方重试仍受唯一分组约束，不会多建任务 | alerts.Service.Receive |
| internal/doctor/store (store) | `pendingGroup` | `internal/doctor/store/alerts.go:50:1` | pendingGroup 只在 firing 时创建任务，单独收到 resolved 不代表现在需要诊断 | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `saveAlerts` | `internal/doctor/store/alerts.go:79:1` | saveAlerts 在同一事务里按指纹保存最新状态；恢复通知不会删除仍待人工查看的任务 | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `Diagnosis` | `internal/doctor/store/diagnoses.go:23:6` | Diagnosis 是列表所需的最小摘要，不包含账号或工具凭据 | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `Detail` | `internal/doctor/store/diagnoses.go:33:6` | Detail 的嵌入字段使摘要与列表保持一致，避免维护两套字段映射 | httpapi.diagnosis |
| internal/doctor/store (store) | `*Store.List` | `internal/doctor/store/diagnoses.go:40:1` | List 只把筛选值绑定到参数；排序和 SQL 结构由代码固定 | httpapi.listDiagnoses |
| internal/doctor/store (store) | `*Store.Detail` | `internal/doctor/store/diagnoses.go:74:1` | Detail 只按固定 ID 查一次任务，再取相关告警，不允许调用者指定表或字段 | httpapi.diagnosis |
| internal/doctor/store (store) | `Store` | `internal/doctor/store/store.go:31:6` | Store 不向 HTTP 层暴露 sql.DB，接口不能随意拼接 SQL | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `New` | `internal/doctor/store/store.go:34:1` | New 只装配已有池，连接检查和生命周期由 app 统一负责 | app.Run |
| internal/doctor/store (store) | `Migrate` | `internal/doctor/store/store.go:37:1` | Migrate 复用命名锁和校验和，两个误启动的进程也不会同时执行同一迁移 | app.Run |
| internal/doctor/store (store) | `newID` | `internal/doctor/store/store.go:42:1` | newID 用随机 16 字节编码为 32 位十六进制；ID 不包含账号或告警内容 | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `User` | `internal/doctor/store/users.go:18:6` | User 只在服务器内部用于验证登录，不能把整个结构体编码成 HTTP 响应 | 本包用户/任务事务与查询方法 |
| internal/doctor/store (store) | `*Store.EnsureUser` | `internal/doctor/store/users.go:26:1` | EnsureUser 使用唯一邮箱处理重复启动，不把初始化配置当成重置密码功能 | app.Run |
| internal/doctor/store (store) | `*Store.UserByEmail` | `internal/doctor/store/users.go:32:1` | UserByEmail 不区分大小写的规则由 auth 统一处理，这里不再改写输入 | httpapi.login |
| internal/doctor/target (target) | `credentials` | `internal/doctor/target/catalog.go:117:1` | credentials 除了变量名，还检查账号身份；换一个变量名不能把同一个账号重复接进来 | Load |
| internal/doctor/target (target) | `credentialIdentity` | `internal/doctor/target/catalog.go:144:1` | credentialIdentity 不包含密码；只要主机和用户名一样，就已经是同一个账号 | credentials |
| internal/doctor/target (target) | `build` | `internal/doctor/target/catalog.go:169:1` | build 把具体后端适配器装进接口；未配置的数据源不会生成对应工具 | Load |
| internal/doctor/target (target) | `*System.bindHTTP` | `internal/doctor/target/catalog.go:191:1` | bindHTTP 装配指标、调用链和日志查询，关闭函数与客户端一起登记 | build |
| internal/doctor/target (target) | `*System.bindNative` | `internal/doctor/target/catalog.go:221:1` | bindNative 装配需要独立账号的数据库，以及配置里逐个列出的进程管理端口 | build |
| internal/doctor/target (target) | `System` | `internal/doctor/target/catalog.go:35:6` | System 不导出令牌与后端连接，接口层只能使用固定注册表 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `Catalog` | `internal/doctor/target/catalog.go:43:6` | Catalog 不公开内部 map，调用方无法在运行中替换系统 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `*System.Name` | `internal/doctor/target/catalog.go:46:1` | Name 返回稳定系统名，不把架构说明或凭据混到任务标识里 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `*System.Tools` | `internal/doctor/target/catalog.go:49:1` | Tools 返回已经固定数据源的注册表；注册表本身没有运行时增删接口 | HTTP 工具路由 |
| internal/doctor/target (target) | `*System.Authenticate` | `internal/doctor/target/catalog.go:52:1` | Authenticate 对两个摘要作定长比较，错误令牌不会因为碰巧是另一个系统的令牌而通过 | httpapi.receiveAlert |
| internal/doctor/target (target) | `*Catalog.Get` | `internal/doctor/target/catalog.go:58:1` | Get 精确匹配系统名，不把不存在的名字退回默认 shop | HTTP 工具/告警路由 |
| internal/doctor/target (target) | `*Catalog.Names` | `internal/doctor/target/catalog.go:64:1` | Names 排序后返回副本，CLI 输出不会随着 map 的遍历顺序变化 | httpapi.listTargets |
| internal/doctor/target (target) | `*Catalog.Close` | `internal/doctor/target/catalog.go:74:1` | Close 即使一条连接关闭失败，也继续关闭其他系统的连接 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `Load` | `internal/doctor/target/catalog.go:85:1` | Load 先检查所有凭据冲突，再创建任何客户端，防止装配一半才发现系统共用身份 | app.Run |
| internal/doctor/target (target) | `Config.variables` | `internal/doctor/target/config.go:127:1` | variables 只包含本阶段真正使用的凭据，不提前向医生传递写账号 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `Config` | `internal/doctor/target/config.go:26:6` | Config 只保存地址和变量名；阶段 7 的写入口尚未开放，也不能提前填进来 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `readConfigs` | `internal/doctor/target/config.go:39:1` | readConfigs 按文件名顺序读取全部 YAML，目录为空也报错，避免“启动成功但什么都接不上” | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `readFile` | `internal/doctor/target/config.go:68:1` | readFile 只接受目录里的普通文件，配置不能通过 ../ 或符号链接读到实验答案 | target.Load 及其装配/清理步骤 |
| internal/doctor/target (target) | `validateConfigs` | `internal/doctor/target/config.go:85:1` | validateConfigs 先查跨系统冲突，再查前缀，错误能直接说明两个系统共用了哪个变量 | target.Load 及其装配/清理步骤 |
| internal/doctor/tools (tools) | `Dependencies` | `internal/doctor/tools/catalog.go:22:6` | Dependencies 在创建时绑定系统 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `New` | `internal/doctor/tools/catalog.go:33:1` | New 创建后菜单不变；缺少数据源对应工具就不存在，而不是调用时借用其他系统 | target.build |
| internal/doctor/tools (tools) | `*Registry.add` | `internal/doctor/tools/catalog.go:61:1` | add 统一登记公共限制 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `textProperty` | `internal/doctor/tools/catalog.go:72:1` | textProperty 统一字符串长度说明，避免某个工具忘记限制输入 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `nameProperty` | `internal/doctor/tools/catalog.go:77:1` | nameProperty 只接受名称字符，URL 中的冒号、斜杠和查询串均不匹配 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `integerProperty` | `internal/doctor/tools/catalog.go:84:1` | integerProperty 用同一处定义上下限和默认值，命令行不能另设更大的范围 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `tableProperty` | `internal/doctor/tools/catalog.go:89:1` | tableProperty 与 MySQL 标识符检查保持一致，错误表名应在联网前作为参数错误返回 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addLogs` | `internal/doctor/tools/catalog_logs.go:18:1` | addLogs 把筛选条件作为普通字段交给适配器，不能让参数变成任意 LogQL | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addMetrics` | `internal/doctor/tools/catalog_metrics.go:18:1` | addMetrics 指标查询先限制窗口和采样点，近期变更也只能从这些已采集指标推断 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addMySQL` | `internal/doctor/tools/catalog_mysql.go:18:1` | addMySQL 将六种固定查看操作登记在同一套入口，不暴露数据库客户端 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addRedis` | `internal/doctor/tools/catalog_runtime.go:17:1` | addRedis 只登记查看分区，调用者不能借运行状态查询执行 Redis 命令 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addProfiles` | `internal/doctor/tools/catalog_runtime.go:29:1` | addProfiles 只允许选择配置里已有的服务，并限制输出的堆栈组数 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `*Registry.addTraces` | `internal/doctor/tools/catalog_traces.go:17:1` | addTraces 先搜索慢请求拿到 ID，再查同一条链，避免把不同请求的步骤拼成一个故事 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `setFailure` | `internal/doctor/tools/registry.go:137:1` | setFailure 只公开固定分类；后端错误可能带地址和凭据，不能原样送给调用者 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `finish` | `internal/doctor/tools/registry.go:155:1` | finish 不截断 JSON 字节流，而是把保留部分放进 preview 字符串，仍返回合法 JSON | Registry.Run 的 defer |
| internal/doctor/tools (tools) | `encoded` | `internal/doctor/tools/registry.go:182:1` | encoded 把大小字段本身也计入总大小；数字位数改变后再算一次即可收敛 | finish |
| internal/doctor/tools (tools) | `prefix` | `internal/doctor/tools/registry.go:196:1` | prefix 不切开一个 UTF-8 字符，避免截断后出现乱码或无效 JSON 文本 | finish |
| internal/doctor/tools (tools) | `Definition` | `internal/doctor/tools/registry.go:35:6` | Definition 是公开的工具说明，不包含数据源地址、账号或执行函数 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `Result` | `internal/doctor/tools/registry.go:44:6` | Result 的失败也是普通结果 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `output` | `internal/doctor/tools/registry.go:56:6` | output 是内部结果，先保留结构化数据，最后才统一编码和截断 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `tool` | `internal/doctor/tools/registry.go:62:6` | tool 把公开说明与内部实现放在一起，调用者只会拿到 Definition 的副本 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `Registry` | `internal/doctor/tools/registry.go:68:6` | Registry 创建完成后不再增删工具，不能在一次调用中临时塞进高权限操作 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `*Registry.List` | `internal/doctor/tools/registry.go:71:1` | List 按名字排序，保证命令行输出和测试结果不受 Go map 遍历顺序影响 | httpapi.listTargets |
| internal/doctor/tools (tools) | `*Registry.Run` | `internal/doctor/tools/registry.go:85:1` | Run 的具名返回值让 defer 能统一记录耗时，也能把意外 panic 转成固定错误结果 | httpapi.runTool |
| internal/doctor/tools (tools) | `Schema.complete` | `internal/doctor/tools/schema.go:105:1` | complete 在格式校验后检查必填字段，缺省值只能取自工具定义 | Schema.validate |
| internal/doctor/tools (tools) | `Property.check` | `internal/doctor/tools/schema.go:121:1` | check 不做宽松类型转换 | Schema.validate |
| internal/doctor/tools (tools) | `Schema` | `internal/doctor/tools/schema.go:26:6` | Schema 只允许定义过的顶层字段，工具参数中从来没有系统名、URL 或主机 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `Property` | `internal/doctor/tools/schema.go:34:6` | Property 的限制同时用于对外说明和运行时校验，避免文档与行为各写一套 | tools.New 或对应 catalog 登记函数 |
| internal/doctor/tools (tools) | `arguments` | `internal/doctor/tools/schema.go:47:6` | arguments 只在校验成功后交给工具，取值时无需重复做类型猜测 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `arguments.text` | `internal/doctor/tools/schema.go:50:1` | text 读取可选字符串；未提供的字段使用空值 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `arguments.integer` | `internal/doctor/tools/schema.go:53:1` | integer 读取整数，默认值也已由校验器转换成 int | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `Schema.validate` | `internal/doctor/tools/schema.go:56:1` | validate 逐个读取字段，才能拒绝 encoding/json 默认会悄悄覆盖的重复键 | Registry.Run |
| internal/doctor/tools (tools) | `treeLine` | `internal/doctor/tools/summary.go:149:1` | treeLine 把一个步骤写成一行，缺失父节点和 SQL 模板都归到这个步骤上 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `recentChanges` | `internal/doctor/tools/summary.go:168:1` | recentChanges 只根据已采集到的启动时间和版本标签说明变化，不声称知道发布原因 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `restart` | `internal/doctor/tools/summary.go:210:6` | restart 是指标记录的启动，不代表我们知道是谁重启了服务 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `version` | `internal/doctor/tools/summary.go:217:6` | version 保留窗口内的采样范围，不把第一次采到当成精确发布时间 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `metricSummary` | `internal/doctor/tools/summary.go:28:6` | metricSummary 保留范围中的极值和最后值，不把几百个重复采样点全部打印出来 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `summarizeMetrics` | `internal/doctor/tools/summary.go:37:1` | summarizeMetrics 的 min/max 来自完整的有界采样，不从最终截断的预览反算 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `summarizeTable` | `internal/doctor/tools/summary.go:65:1` | summarizeTable 的说明明确这些是计划或元数据，不能把表统计中的估计值当精确行数 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `slowTrace` | `internal/doctor/tools/summary.go:70:6` | slowTrace 只保留最慢几步，其余步骤可通过同一个 ID 继续查 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `summarizeTraces` | `internal/doctor/tools/summary.go:78:1` | summarizeTraces 复制 span 切片再排序，不改动数据源可能复用的响应对象 | catalog 中注册的工具闭包与 Registry.Run |
| internal/doctor/tools (tools) | `summarizeTree` | `internal/doctor/tools/summary.go:99:1` | summarizeTree 给丢失父节点和环路留出明确标记，不能因为后端数据不完整而无限递归 | catalog 中注册的工具闭包与 Registry.Run |
| internal/platform/alertsink (alertsink) | `Run` | `internal/platform/alertsink/app.go:33:1` | Run 先验证令牌再开放端口，漏配环境变量时不能意外启动一个无认证接收器 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/platform/alertsink (alertsink) | `serve` | `internal/platform/alertsink/app.go:58:1` | serve 的结果通道只有一次写入，主流程退出前一定读到它，不遗留监听 goroutine | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `processingBudget` | `internal/shop/app/orderservice/config.go:112:1` | processingBudget 给幂等占位和优雅停机使用同一个上界，避免发送还在执行时就把请求判为待核对 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `kafkaBrokers` | `internal/shop/app/orderservice/config.go:119:1` | kafkaBrokers 要求每项都包含主机和端口，空列表项通常是配置多写了一个逗号 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `Config` | `internal/shop/app/orderservice/config.go:36:6` | Config 中的 Database 含密码，禁止把整个结构体写进日志 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `loadConfig` | `internal/shop/app/orderservice/config.go:57:1` | loadConfig 用局部读取函数收集错误，用户一次就能看到多个配置问题 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `openInventory` | `internal/shop/app/orderservice/dependencies.go:109:1` | openInventory 把插件放到每次真实 RPC 外层，重试发生几次就记录几次调用 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `inventoryDialOptions` | `internal/shop/app/orderservice/dependencies.go:123:1` | inventoryDialOptions 保留按主机名找 IP 的能力，但不从 DNS 读取本项目不用的服务配置 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `*dependencies.close` | `internal/shop/app/orderservice/dependencies.go:139:1` | close 既用于启动失败，也用于正常停机，所以只关闭实际打开过的资源 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `dependencies` | `internal/shop/app/orderservice/dependencies.go:39:6` | dependencies 只保存进程拥有的连接 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `openDependencies` | `internal/shop/app/orderservice/dependencies.go:48:1` | openDependencies 失败时自己清理；成功后把关闭责任交给 Run，避免半途失败留下连接 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| internal/shop/app/orderservice (orderservice) | `openRedis` | `internal/shop/app/orderservice/dependencies.go:90:1` | openRedis 不记录命令参数，因为幂等键和缓存正文可能包含请求数据 | 原有应用装配与同包调用；本阶段仅修改其中标注的部分 |
| tools/archcheck (main) | `modulePath` | `tools/archcheck/check.go:121:1` | modulePath 只读取 go.mod 的 module 行，不要求先下载依赖 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `locationProblem` | `tools/archcheck/check.go:138:1` | locationProblem 检查根目录 Go 文件、cmd 入口文件数量和含糊的包名 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `under` | `tools/archcheck/check.go:161:1` | under 按目录边界匹配路径，防止把 internal/doctorish 误当成 internal/doctor | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `importProblem` | `tools/archcheck/check.go:164:1` | importProblem 检查领域隔离和包内文件角色 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `layerProblem` | `tools/archcheck/check.go:200:1` | layerProblem 按文件角色限制传输和存储依赖 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `scenarioProblems` | `tools/archcheck/check.go:236:1` | scenarioProblems 检查文件读取调用和 go:embed | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `importAliases` | `tools/archcheck/check.go:276:1` | importAliases 记录标准库读取包的本地名字 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `isReadCall` | `tools/archcheck/check.go:295:1` | isReadCall 识别 os/io/fs/path/filepath 的文件入口 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `stringValues` | `tools/archcheck/check.go:325:1` | stringValues 收集可静态求值的字符串声明与赋值 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `stringExpression` | `tools/archcheck/check.go:371:1` | stringExpression 展开字符串、标识符、加法和 Join 调用 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `check` | `tools/archcheck/check.go:38:1` | check 从 go.mod 获取真实模块路径后遍历源码 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `isScenarioPath` | `tools/archcheck/check.go:407:1` | isScenarioPath 同时识别标准答案和注入状态，二者都可能直接泄露故障原因 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `inspectSource` | `tools/archcheck/check.go:95:1` | inspectSource 解析单个 Go 文件并分别检查依赖和答案读取 | archcheck.check、inspectSource 或对应测试 |
| tools/archcheck (main) | `doctorMountProblems` | `tools/archcheck/doctor.go:21:1` | doctorMountProblems 使用完整 YAML 解析，长格式挂载也不能绕过只接受固定字符串的规则 | archcheck.check、inspectSource 或对应测试 |

<!-- PHASE_SYMBOLS_END -->

### 3.4 数据库变更

独立 doctor-mysql 的三份迁移：0001_users.sql 保存32位ID、唯一邮箱、bcrypt摘要和账号状态；0002_diagnoses.sql 保存绑定系统的任务、输入通知和状态，active_group 唯一索引按系统加分组键去重，target/status/created_at 索引用于分页；0003_diagnosis_alerts.sql 以 diagnosis_id/fingerprint 为联合主键，保存每条告警最新快照。

diagnoses 的模型、报告、计费、租约字段仅按计划结构预留，当前都不执行。没有创建 diagnosis_steps，因此也没有遗漏它的 model 列。复用迁移校验和与命名锁；测试只操作专用 opsdoctor_doctor_test，未清理业务库。

### 3.5 接口变更

| 接口 | 身份与用途 |
|---|---|
| POST /api/v1/auth/login | 邮箱密码，返回一小时JWT；每秒2次、短时4次登录限流 |
| GET /api/v1/targets | JWT；系统名和各自工具说明，不返回凭据或后端地址 |
| POST /api/v1/targets/{target}/tools/{tool}/run | JWT；同一个Registry，工具错误作为Result返回 |
| POST /api/v1/targets/{target}/alerts | 该系统独立Bearer令牌；提交后202；A令牌不能推B |
| GET /api/v1/diagnoses、/api/v1/diagnoses/{id} | JWT；按系统/状态分页或查看任务与快照 |
| /health/live、/health/ready、/metrics | 存活、医生数据库就绪、通用进程指标 |

新增本机端口 127.0.0.1:8090、127.0.0.1:3307。没有新增 gRPC 方法或客户端。后续若加入OTLP，必须沿用禁用DNS服务配置的选项。

### 3.6 配置变更

| 变量 | 含义、默认值 |
|---|---|
| DOCTOR_ADDR | 监听地址，默认 :8090 |
| DOCTOR_MYSQL_ROOT_PASSWORD / DOCTOR_MYSQL_PASSWORD | 独立实例初始化密码，示例假值，真实环境必填 |
| DOCTOR_MYSQL_DSN | doctor自己的库，必须是doctor账号和doctor库，无生产默认 |
| DOCTOR_ADMIN_EMAIL / DOCTOR_ADMIN_PASSWORD | 初始化登录用户，必填；已有用户不覆盖 |
| DOCTOR_URL / DOCTOR_TOKEN | CLI地址默认http://127.0.0.1:8090；登录JWT必填 |
| JWT_SECRET | 沿用变量，至少32字节；示例假值 |
| TARGETS_DIR | 宿主默认deploy/doctor/targets；容器/etc/doctor/targets，替换TARGET_CONFIG |
| SHOP_ALERT_TOKEN | shop告警令牌，至少32字节；替换ALERT_WEBHOOK_TOKEN |
| SHOP_MYSQL_RO_PASSWORD / SHOP_MYSQL_RO_DSN | 初始化只读身份 / 医生使用的连接串 |
| REDIS_PASSWORD / SHOP_REDIS_RO_PASSWORD | 业务与医生两个不同密码，只有初始化Redis拿到两者 |
| SHOP_REDIS_RO_URL | 医生仅拿diag_ro URL，DB 0 |
| TEST_REDIS_PASSWORD | 新增业务测试密码，缺少时明确Skip |
| TEST_SHOP_MYSQL_RO_DSN / TEST_SHOP_REDIS_RO_URL / TEST_DOCTOR_MYSQL_DSN | 三项医生权限和仓储测试配置；宿主地址、专用身份与测试库 |

`.env.example` 全是假值。医生没有接收 SHOP_MYSQL_DDL_DSN、OPS_TOKEN、REDIS_PASSWORD 或 CHAOS_*。LLM 配置继续预留，不读取、不调用。

### 3.7 部署变更

新增 doctor（256 MiB）与 doctor-mysql（512 MiB、buffer pool 64 MiB）。移除默认运行的 alertsink，所以从18个变为19个容器。Alertmanager推送、Prometheus采集、Alloy日志筛选均指向doctor。镜像和本机端口继续固定。

Redis 启动从环境生成内存ACL文件，umask077，目录0700、文件0600，启动参数只有文件路径。健康检查用REDISCLI_AUTH，不用-a。曾执行 down 后 up，未使用-v，数据卷保留。

## 四、测试

### 4.1 新增与修改的测试清单

<!-- PHASE_TESTS_START -->

| 测试名 | 位置 | 验证内容 |
|---|---|---|
| `TestCLIUsesServer` | `internal/doctor/app/cli_test.go:23:1` | TestCLIUsesServer 验证 CLI 不会自行吞掉或接受非法参数，最终结果由注册表所在的接口决定 |
| `TestCLIRejectsRedirect` | `internal/doctor/app/cli_test.go:52:1` | TestCLIRejectsRedirect 让第二个服务记录请求数，证明拒绝发生在令牌被转发之前 |
| `TestCLICommand` | `internal/doctor/app/cli_test.go:65:1` | TestCLICommand 只测入口语法；工具字段的校验留给服务端注册表 |
| `TestTargetAuthentication` | `internal/doctor/httpapi/router_test.go:129:1` | TestTargetAuthentication 把路径切换和令牌类型混用都当作未经授权处理 |
| `TestLoginAndToolRoute` | `internal/doctor/httpapi/router_test.go:153:1` | TestLoginAndToolRoute 从真实登录响应取令牌，再通过 HTTP 进入统一工具校验 |
| `TestNotificationValidation` | `internal/doctor/httpapi/router_test.go:186:1` | TestNotificationValidation 证明无效通知不会留下半条任务记录 |
| `TestSelectStatement` | `internal/doctor/source/select_test.go:15:1` | TestSelectStatement 不放宽输入去兼容所有 SQL；诊断工具宁可要求改写成更简单的查询 |
| `TestHTTPBoundary` | `internal/doctor/source/httpbackend/client_test.go:23:1` | TestHTTPBoundary 既检查返回错误，也检查不能发生的外部访问确实没有发生 |
| `TestRealContract` | `internal/doctor/source/jaeger/client_test.go:26:1` | TestRealContract 保留真实三服务结构，防止只用手写的小 JSON 漏掉实际字段关系 |
| `TestMalformedAndEmpty` | `internal/doctor/source/jaeger/client_test.go:94:1` | TestMalformedAndEmpty 检查真实成功样例覆盖不到的布尔属性、空结果和错误路径 |
| `TestSearchBoundary` | `internal/doctor/source/loki/client_test.go:24:1` | TestSearchBoundary 同时检查请求和返回，避免工具把其他服务的日志混进结论 |
| `TestQueries` | `internal/doctor/source/mysqlinspect/client_test.go:106:1` | TestQueries 核验每一种查看入口，同时证明表名是参数，EXPLAIN 是工具自己加的前缀 |
| `TestPoolDeadline` | `internal/doctor/source/mysqlinspect/client_test.go:153:1` | TestPoolDeadline 用虚拟时间等待真实连接池，不依赖机器在几毫秒内被调度 |
| `TestOptions` | `internal/doctor/source/mysqlinspect/client_test.go:176:1` | TestOptions 即使部署者误开了多语句或本地文件读取，医生也会在构造时覆盖它 |
| `TestIntegrationMySQLConnectionLimit` | `internal/doctor/source/mysqlinspect/permissions_integration_test.go:106:1` | TestIntegrationMySQLConnectionLimit 绕过客户端池再建第四条连接，证明限制也落在账号侧 |
| `TestIntegrationMySQLPermissions` | `internal/doctor/source/mysqlinspect/permissions_integration_test.go:54:1` | TestIntegrationMySQLPermissions 每个拒绝都检查 MySQL 错误号，语法错误不能冒充权限证明 |
| `TestGoroutines` | `internal/doctor/source/pprof/client_test.go:20:1` | TestGoroutines 确认未知服务在联网前被拒绝，而不只是最后返回 404 |
| `TestQueries` | `internal/doctor/source/prometheus/client_test.go:22:1` | TestQueries 确认工具给出的范围真的被发送到后端，pending 也不会混入 firing 列表 |
| `TestReadContract` | `internal/doctor/source/redisinspect/client_test.go:27:1` | TestReadContract 保留耗时证据，同时确认缓存键和值没有进入输出 |
| `TestConfiguration` | `internal/doctor/source/redisinspect/client_test.go:60:1` | TestConfiguration 在联网前拒绝危险配置，非法 INFO 分区也不能被拼成别的命令 |
| `TestIntegrationRedisPermissions` | `internal/doctor/source/redisinspect/permissions_integration_test.go:40:1` | TestIntegrationRedisPermissions 连接失败会报错，不能把无法验证的权限算作通过 |
| `TestIntegrationAlertRollback` | `internal/doctor/store/store_integration_test.go:133:1` | TestIntegrationAlertRollback 直接调用仓储制造子表失败，证明任务不会独自留在库里 |
| `TestIntegrationUserBootstrap` | `internal/doctor/store/store_integration_test.go:147:1` | TestIntegrationUserBootstrap 第二次初始化不能悄悄改掉用户已经在用的密码摘要 |
| `TestIntegrationAlertDeduplication` | `internal/doctor/store/store_integration_test.go:74:1` | TestIntegrationAlertDeduplication 先按要求顺序推三次，再加并发推送检查竞态 |
| `TestCredentialReuse` | `internal/doctor/target/catalog_test.go:105:1` | TestCredentialReuse 覆盖“变量名不同，但实际值仍共用”的部署错误 |
| `TestIsolation` | `internal/doctor/target/catalog_test.go:28:1` | TestIsolation 使用真正的 HTTP 适配器与工具注册表，不用假 Registry 代替接线 |
| `TestConfigFailures` | `internal/doctor/target/catalog_test.go:81:1` | TestConfigFailures 在创建连接之前报错，而且错误不会带出密码或完整连接串 |
| `TestEveryToolRejectsDestination` | `internal/doctor/tools/catalog_test.go:29:1` | TestEveryToolRejectsDestination 覆盖完整菜单，不能只拿指标工具代表其余十五个工具 |
| `TestTraceSummaryBoundaries` | `internal/doctor/tools/catalog_test.go:50:1` | TestTraceSummaryBoundaries 检查摘要明确标记不完整信息，而不是递归不结束或假造父节点 |
| `TestBoundSources` | `internal/doctor/tools/registry_test.go:139:1` | TestBoundSources 同样的工具名在两个系统里仍使用各自构造时绑定的对象 |
| `TestArguments` | `internal/doctor/tools/registry_test.go:50:1` | TestArguments 检查错误参数不会触碰后端，尤其不能多塞一个 URL 或系统名 |
| `TestLimits` | `internal/doctor/tools/registry_test.go:82:1` | TestLimits 的超时断言使用精确虚拟时间，不把调度快慢混进预算规则 |
| `TestRetryCancellation` | `internal/platform/resilience/resilience_test.go:100:1` | TestRetryCancellation 让第一次失败同时取消上下文，等待器必须立即结束 |
| `TestBreakerRecovery` | `internal/platform/resilience/resilience_test.go:115:1` | TestBreakerRecovery 用短窗口观察完整状态变化，确认打开期间没有偷偷执行下游回调 |
| `TestRateLimit` | `internal/platform/resilience/resilience_test.go:31:1` | TestRateLimit 用一个几乎不会在测试期间补满的桶，明确看到“放行一次，再拒绝一次” |
| `TestRetryPolicy` | `internal/platform/resilience/resilience_test.go:53:1` | TestRetryPolicy 固定时钟和随机增量，让重试预算只取决于输入，不取决于机器快慢 |
| `TestConfigBudgets` | `internal/shop/app/orderservice/config_test.go:21:1` | TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动 |
| `TestInventoryIdleConfig` | `internal/shop/app/orderservice/config_test.go:69:1` | TestInventoryIdleConfig 保留正常的 30 分钟默认值，只在实验时允许缩短，不把修复变成关闭空闲回收 |
| `TestIntegrationReservationOwnership` | `internal/shop/order/cache_integration_test.go:120:1` | TestIntegrationReservationOwnership 删除安全失败的占位后，新请求得到新的所有者，旧请求不能再修改它 |
| `TestIntegrationExpiredProcessing` | `internal/shop/order/cache_integration_test.go:151:1` | TestIntegrationExpiredProcessing 模拟进程退出留下的占位，不用真实等待十秒 |
| `TestIntegrationReservationCompetition` | `internal/shop/order/cache_integration_test.go:70:1` | TestIntegrationReservationCompetition 验证 SET NX 的竞争结果，再检查写回结果是否保留原 TTL |
| `TestIntegrationRedisCache` | `internal/shop/product/cache_integration_test.go:24:1` | TestIntegrationRedisCache 先写正常商品再覆盖为空值，两种状态都必须能被下一次读取识别 |
| `TestScenarioReads` | `tools/archcheck/check_test.go:120:1` | TestScenarioReads 验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案 |
| `TestForbiddenImports` | `tools/archcheck/check_test.go:39:1` | TestForbiddenImports 覆盖领域隔离和分层违规，包括不能靠“包名前缀相似”产生误报的边界 |
| `TestLocations` | `tools/archcheck/check_test.go:99:1` | TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报 |
| `TestDoctorMounts` | `tools/archcheck/doctor_test.go:16:1` | TestDoctorMounts 直接调用 archcheck 使用的检查函数，不另写一份较宽松的测试判断 |

<!-- PHASE_TESTS_END -->

HTTP适配器使用httptest；原生MySQL/Redis使用驱动替身与RESP假服务，并另做真实权限集成测试。虚构第二系统使用假后端，测试不产生费用。

### 4.2 运行命令和结果

```text
gofmt -l cmd gen internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
```

以上全部通过。commentcheck覆盖243个非生成Go文件。无外部环境轮136个顶层测试通过、22个集成测试明确跳过，42个有测试包通过；这不作为真实集成验收。按README注入全部11项TEST变量后，十个包执行 `-race -run Integration -count=1 -timeout=90s`：22个顶层、含子用例28个通过，0 skip。两套promtool规则测试通过。

第一轮staticcheck发现错误字符串格式与数字状态码，修正后完整重跑，失败记录保留。测试时暂停流量、医生与通知，完成后恢复医生与200 QPS，healthy通过后才恢复通知。Linux CI：[40296137783](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40296137783) 全部通过，验证提交 `5b26ed0`。环境固定 ubuntu-24.04 / Go 1.27.1，包含全新数据卷启动、真实权限集成、业务及观测接线、医生CLI与三次通知去重。

## 五、工程验收结果

| 验收项 | 本地结果与证据 |
|---|---|
| 注入S01，doctorctl完整取证 | 通过，两轮inject→verify→工具证据→restore→healthy；Deduct的UPDATE 796.733ms，EXPLAIN全表扫描、缺索引、恢复后const计划，见notes |
| archcheck与参数目的地址限制 | 通过；完整十六菜单拒绝url/host/system/target；源码、容器挂载检查通过 |
| Alertmanager创建并绑定任务 | 通过；真实新增三个shop pending分组，第二轮重复通知复用 |
| CLI按系统显示菜单 | 通过；真实shop十六项；双系统测试各自后端返回11/22，独立菜单与凭据 |
| 实际数据库/Redis权限逐项对照 | 通过，见下方实际输出与测试错误号 |

新增数据台账条目：**P5-1–P5-18**。完整命令、失败轮次、两轮逐工具耗时与大小见 [notes](../notes.md#phase-05)，关键数字见[数据台账](../数据台账.md)。本次复验参数明确是 **商品范围10000、目标200 QPS、工作者64**，不能外推到默认五十万商品工作集。

### 实际 MySQL 授权

以下是本机 `SHOW GRANTS FOR 'diag_ro'@'%'` 的实际输出，不包含密码摘要：

```sql
GRANT USAGE ON *.* TO `diag_ro`@`%`
GRANT SELECT (`BLOCKING_THREAD_ID`, `ENGINE`, `REQUESTING_ENGINE_LOCK_ID`, `REQUESTING_THREAD_ID`) ON `performance_schema`.`data_lock_waits` TO `diag_ro`@`%`
GRANT SELECT (`ENGINE`, `ENGINE_LOCK_ID`, `INDEX_NAME`, `LOCK_MODE`, `LOCK_TYPE`, `OBJECT_NAME`, `OBJECT_SCHEMA`) ON `performance_schema`.`data_locks` TO `diag_ro`@`%`
GRANT SELECT (`AVG_TIMER_WAIT`, `COUNT_STAR`, `DIGEST_TEXT`, `SCHEMA_NAME`, `SUM_ROWS_EXAMINED`, `SUM_ROWS_SENT`, `SUM_TIMER_WAIT`) ON `performance_schema`.`events_statements_summary_by_digest` TO `diag_ro`@`%`
GRANT SELECT (`VARIABLE_NAME`, `VARIABLE_VALUE`) ON `performance_schema`.`global_status` TO `diag_ro`@`%`
GRANT SELECT ON `shop`.`orders` TO `diag_ro`@`%`
GRANT SELECT ON `shop`.`processed_events` TO `diag_ro`@`%`
GRANT SELECT ON `shop`.`product_sales` TO `diag_ro`@`%`
GRANT SELECT ON `shop`.`products` TO `diag_ro`@`%`
GRANT SELECT ON `shop`.`stock` TO `diag_ro`@`%`
```

另查 mysql.user 得到 MAX_USER_CONNECTIONS=3。

### 实际 Redis ACL

本机 ACL GETUSER 经过过滤，仅保留规则字段，不保存密码哈希：

```text
user: diag_ro
flags: on sanitize-payload
commands: -@all +hello +ping +info +slowlog|get +slowlog|len
keys: 空
channels: 空
selectors: []
```

| 5.5.9读边界 | 实现与实际验证 |
|---|---|
| 业务只读、能看performance_schema | 五张表SELECT，监控表只授实际列；INSERT/UPDATE/DELETE均1142 |
| 不能读样例SQL原文 | 不授QUERY_SAMPLE_TEXT，真实查询返回1143；工具只读DIGEST_TEXT |
| 不执行任意SQL | EXPLAIN前缀由工具添加；非SELECT、多语句、EXPLAIN/ANALYZE输入拒绝；DSN强制multiStatements=false，原生多语句1064 |
| 三连接额度 | 第四条原生连接1226；池上限3，满池排队10.00s返回deadline |
| Redis只查看 | INFO/SLOWLOG成功；GET/HGETALL/KEYS/SET/FLUSHALL/CONFIG/SLOWLOG RESET全部NOPERM，先DRYRUN再验证；匿名NOAUTH |
| 不拿业务数据行或key内容 | 工具只返回计划/元数据，慢日志删去key/value；账号本身有SELECT能力，不把工具限制说成数据库完全禁止读行 |
| 不读取答案或控制Docker | 仅挂只读targets；无仓库、场景、状态、socket；不import shop/chaos，不启动os/exec |
| 多系统隔离 | 各自客户端、菜单、令牌；共享凭据或重名启动失败；A令牌推B401且未进入仓储 |

### 有流量时的内存与连接

S01两轮总占用3411.730 / 3587.000 MiB，均低于4.5 GiB。下表为第二轮实际docker stats快照，包含上限。不能把一次快照称为峰值。

| 容器 | 实际内存 / 上限 |
|---|---|
| opsdoctor-alertmanager-1 | 12.69MiB / 64MiB |
| opsdoctor-alloy-1 | 69.94MiB / 256MiB |
| opsdoctor-doctor-1 | 18.64MiB / 256MiB |
| opsdoctor-doctor-mysql-1 | 200.8MiB / 512MiB |
| opsdoctor-grafana-1 | 211MiB / 256MiB |
| opsdoctor-inventory-service-1 | 16.76MiB / 128MiB |
| opsdoctor-jaeger-1 | 350.4MiB / 1GiB |
| opsdoctor-kafka-1 | 809.8MiB / 1GiB |
| opsdoctor-kafka-exporter-1 | 13.61MiB / 64MiB |
| opsdoctor-loadgen-1 | 21.32MiB / 128MiB |
| opsdoctor-loki-1 | 390.4MiB / 512MiB |
| opsdoctor-mysqld-exporter-1 | 11.64MiB / 64MiB |
| opsdoctor-order-service-1 | 22.26MiB / 256MiB |
| opsdoctor-order-worker-1 | 15.23MiB / 128MiB |
| opsdoctor-prometheus-1 | 67.34MiB / 512MiB |
| opsdoctor-redis-1 | 385.2MiB / 768MiB |
| opsdoctor-redis-exporter-1 | 12.54MiB / 64MiB |
| opsdoctor-shop-mysql-1 | 935.7MiB / 1.5GiB |
| opsdoctor-toxiproxy-1 | 21.73MiB / 128MiB |

两轮只读连接各采样153/122次，观测最大1；同一工具入口的120次/12路status及24次/8路top_queries均ok。采样间隔包含docker exec耗时，会漏掉很短的并发连接，因此不把1当精确连续峰值。账号与池的上限3另由集成测试证明。

## 六、偏离计划之处

1. **已获用户同意：补Redis认证。** 阶段3起default一直无密码全权限，阶段5检查才发现。用户批准同步业务客户端、chaos、exporter、Compose、CI、环境变量与文档。做了保留数据重建和S04/S05全流程回归。[决定记录](../decisions.md#2026-10-26redis-补认证复验固定在-1-万商品基线)。
2. **用户决定暂不修新发现的CacheLoadsHigh适用范围。** 按一万商品基线复验，不改规则或代码默认值。这是限定验收范围，不是修复误报。
3. 原生数据库协议不能用httptest直接模拟，所以MySQL用database/sql假驱动，Redis用RESP假服务，真实权限另验。仍使用计划技术栈，没有新增组件。[适配器决定](../decisions.md#2026-10-26jaeger-2200-查询契约与只读-mysql-列权限)。
4. doctorctl采用HTTP同一路径，增加手动工具运行接口；用户明确授权自行选择并记录。[入口决定](../decisions.md#2026-10-26医生接管告警与命令行入口)。

## 七、已知问题与遗留事项

- **五十万商品工作集的CacheLoadsHigh误报仍未修复。** 200 QPS下回源快照43.8727%高于35%，115127请求错误为0。工作集变大后正常未命中率很高，单一比例无法区分这种情况和S05。用户指定另做告警修复；本阶段没有改规则或默认值。重建前必须明确设置商品数、QPS、工作者数，P4-16和P5-8都记录过这个问题。
- pending只保存，不执行。没有模型、报告提交、页面、写工具或MCP；阶段6/7再按计划实现。学习验收由用户勾选。
- Jaeger旧内部查询接口和内存保留窗口均有限，升级需同时回归Grafana与医生适配器。
- 只读MySQL账号具有业务SELECT权限，工具限制不能保护泄露后被人直连使用的凭据。尚未实现阶段7统一脱敏，当前没有任何数据发给大模型。
- 摘要与采样会丢失信息。截断、limited、空锁快照、累计SQL统计都不能被当作全量事实；学习材料逐项解释。
- 原有Deduct超时结果不确定和无Outbox的消息窗口仍存在。本阶段未根治这些一致性问题。

## 八、注释自查结论

commentcheck通过，243个非生成Go文件。新增包都有doc.go，文件清单同时包含测试文件；新增分组目录有README。每个Go文件写了用途、主要内容和阅读建议。内部注释解释为什么要拒绝重复JSON字段、绑定系统、隐藏原始后端错误、限制池等待、先DRYRUN，以及为什么快照不代表历史。

人工自查时发现部分登记函数说明重复、八处生产函数较长，已改成各组具体理由并拆分步骤；几处旧auth/README仍写“未接线”，已更新。代码地图重新生成。学习材料按初学者顺序说明概念，包含真实失败记录、权限边界、完整S01命令和八个必答问题；四段 Go 示例实际运行通过，十二段 PowerShell 示例语法检查通过，相对文件链接存在；没有为检查语法重复执行实验。

## 九、学习指引

读[学习材料](../learn/phase-05-doctor-tools.md)，先用doctorctl手动走一次S01，再从入口、source接口、Registry、MySQL适配器往里读，最后看告警事务与测试。计划学习量约9小时，可拆为取证、代码路径、权限测试三段。这是学习建议，不是实际测得的耗时。

## 十、下一阶段预告

阶段6才会接模型、Agent循环、调度和实时页面。当前分支完成后停下，等Claude验收与用户学习；没有开始相关实现或付费调用。
