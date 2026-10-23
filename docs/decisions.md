# 设计决定

## 2026-10-23：阶段 0 的范围与验收环境

- 决定：只实现计划书阶段 0；从 main 的 `3ebd194` 创建 `phase-00-foundation`。不建立空业务包。
- 原因：先让共享基础设施和检查工具可学习、可验证，再按阶段增加业务。
- 放弃的方案：提前搭建库存服务、医生 HTTP 服务或后续阶段目录。
- 环境决定：用户在本次对话明确同意“继续写代码；我来准备 Docker，本机 race 暂由固定版本 Linux CI 验证”。CI 固定为 `ubuntu-24.04`。
- 实测限制：初次检查未找到 Docker 和 C 编译器，CGO_ENABLED=0。Docker 健康状态必须实际验证，不能把未执行当成通过。

## 2026-10-23：复用来源

- 只读来源：`D:\Code\GO项目\ai-backend`，main 提交 `095337690a9a881cba7a8eced3aae950b6016582`。
- 复用 JWT/bcrypt、MySQL 连接池与 UTC 会话、命名锁迁移、HTTP 中间件及 CI 思路。
- 按本项目拆包、补充中文注释；不复制文档问答业务、业务表或领域依赖。

## 2026-10-23：阶段 0 版本锁定

Go 库用 `go list -m -json <模块>@latest` 查询稳定版本后固定；不在 CI 中使用 `@latest`。直接依赖之外的版本由 go.mod/go.sum 记录，不主动引入其功能。

| 组件 | 固定版本 | 原因与放弃的替代方案 |
|---|---|---|
| Go | 1.27.1 | 按计划书和本机版本，不随 CI runner 自动升级 |
| Gin | v1.12.0 | 按技术栈复用旧中间件；不另选 HTTP 框架 |
| go-sql-driver/mysql | v1.10.1 | database/sql 驱动；不使用 ORM |
| golang-jwt/jwt/v5 | v5.3.1 | 复用已验证的 JWT 实现；不自行实现签名协议 |
| golang.org/x/crypto | v0.58.0 | 使用 bcrypt；不自行实现密码哈希 |
| staticcheck / honnef.co/go/tools | v0.8.2 | 查询时的稳定版本；不沿用旧仓库 v0.8.1 |
| MySQL | mysql:8.4.13 | 计划指定 8.4 LTS，选该系列已发布补丁；不切换其他大版本 |
| Redis | redis:8.10.2 | 查询时稳定版本；这里只启用基础缓存实例 |
| Kafka | apache/kafka:4.4.0 | 官方 KRaft 镜像；manifest 已实际查询，无 ZooKeeper |
| actions/checkout | v7.0.1 | GitHub releases/latest 查询结果，固定完整版本 |
| actions/setup-go | v7.0.0 | 同上，Go 版本显式固定为 1.27.1 |
| CI 系统 | ubuntu-24.04 | 按计划固定系统，不用 ubuntu-latest |

版本来源：[MySQL 官方镜像清单](https://github.com/docker-library/official-images/blob/master/library/mysql)、[Redis 发布页](https://github.com/redis/redis/releases)、[Kafka 官方下载目录](https://downloads.apache.org/kafka/)、[Gin 发布页](https://github.com/gin-gonic/gin/releases)、[MySQL 驱动发布页](https://github.com/go-sql-driver/mysql/releases)、[JWT 发布页](https://github.com/golang-jwt/jwt/releases)、[Staticcheck 发布说明](https://staticcheck.dev/changes/)。镜像存在性最终以实际成功拉取为证。

## 2026-10-23：基础设施的实现取舍

- config 注入读取函数，标准库足够；不增加 dotenv/Viper。服务自己的配置结构体留到对应 app 出现时再写。
- logging 使用独立 slog.Logger 和不变的 context 字段，不修改全局 logger；trace/span 仅承载已提供的标识，不提前接入阶段 3。
- HTTP 保留旧代码的请求标识校验和安全恢复，按新契约统一为 `INVALID_ARGUMENT`；不迁入旧业务错误分类。
- 迁移 API 接收 fs.FS，生产调用方可以传 embed.FS。一文件一条 SQL，禁用 multiStatements；不继承旧实现按分号拆 SQL 的做法，因为字符串内也能包含分号。执行前检查历史校验和，命名锁始终使用同一个 sql.Conn；DDL 仍需可重入，不承诺整批回滚。这是阶段 0 迁移基础设施的细化，不新增业务迁移。
- 退出流程拆为 Notify 和 Shutdown，由后续 app 决定如何停止接收请求和工作者；不提前写服务入口。关闭函数必须响应 context，通用层不能强杀任意阻塞代码。
- commentcheck 使用标准生成标记跳过生成代码，外部测试包沿用同目录被测包的 doc.go。archcheck 对可静态识别的文件读取做检查；它不能穷尽运行时拼接、函数别名和跨文件数据流，后续仍须依靠配置与最小权限。
- 仅创建计划书列出的本阶段目录；没有空业务包，也没有新增技术栈组件。

## 2026-10-23：Compose 首次启动修正

- 默认 `.env` 查找未找到仓库根目录文件，因此 README 显式使用 `--env-file .env`。先注入 shell 环境时，计划书原样的 Compose 命令也实测通过。
- Kafka 数据路径使用官方镜像预建且属于 appuser 的 `/var/lib/kafka/data`。首次尝试 `/tmp/kraft-combined-logs` 的空卷属主不匹配，导致启动失败；已修复并保留失败记录。
- MySQL 只初始化空 shop 库和两个隔离测试库；本阶段没有业务表。迁移测试强制校验专用库名，避免清理业务库。
- 用户随后安装并启动 Docker、安装 gcc。本机 CGO 已启用；最终同时跑本机 race 和 Linux CI，不再依赖暂时的替代验收安排。

## 2026-10-23：阶段 1 的范围与版本

- 决定：用户核实阶段 0 返工后，明确授权合并、推送 main，并进入阶段 1；据此从 main 创建 `phase-01-inventory`。学习验收仍留给用户勾选。
- 原因：阶段 1 只交付库存 gRPC、五张业务表迁移、造数和手动调用命令；订单服务、缓存和消息处理留到阶段 2。
- 放弃的方案：提前实现下单、消费历史订单，或给库存扣减添加未经计划定义的幂等字段。
- 协议文件采用第 5.1 节的 `api/proto/inventory/v1/inventory.proto`，第 6 节的 `proto/inventory/v1/inventory.proto` 按协议子目录的简写理解；没有另建顶层 proto 目录。

| 新组件 | 固定版本 | 决定原因 / 放弃的方案 |
|---|---|---|
| google.golang.org/grpc | v1.84.0 | 查询稳定版本后写入 go.mod；不用另一种 RPC 协议 |
| google.golang.org/protobuf | v1.36.12 | 与生成插件一致；不手写协议序列化 |
| buf | v1.75.0 | 按计划统一 lint 和生成；不用人工拼接 protoc 参数 |
| protoc-gen-go | v1.36.12 | 本地固定插件；不使用浮动远程插件 |
| protoc-gen-go-grpc | v1.6.2 | 同上，生成客户端和服务端接口 |
| 构建镜像 | golang:1.27.1-alpine3.24 | 固定 Go 和 Alpine 系列，实际拉取成功 |
| 运行镜像 | alpine:3.24.2 | 非 root 运行，保留 wget 健康检查；不携带编译工具链 |

现有 MySQL、Redis、Kafka 和 CI 版本沿用阶段 0。间接依赖精确版本以 go.mod/go.sum 为准。

## 2026-10-23：库存与造数的边界

- 决定：用带 `available >= quantity` 条件的单条 UPDATE 扣库存，借助 `uk_stock_sku` 定位；更新失败后才查询存在性来选择错误码。
- 原因：先读余额再写余额会留下并发窗口；进程内互斥锁也保护不了多个服务实例。放弃先查后改，不额外引入分布式锁。
- 决定：默认连接池 20 个打开连接、10 个空闲连接、5 分钟连接寿命。这是本机靶场的起始配置，并非压测得出的最优值；MySQL 最大连接数为 100，需要给迁移、CLI 和以后其他服务留余量。
- 决定：seed 每批最多 1000 商品和 4000 订单，五张表在同一事务提交；50 万商品对应 200 万历史订单，同时写入销量和已处理事件基线。
- 原因：中断时不会出现本批只写了部分表，后续消费也不会把历史数据重复计算。放弃逐条提交、全量大事务和向 Kafka 重放历史订单。
- 限制：seed 是独占运行的离线初始化工具，按行数和连续 ID 检查已有基线，不能验证任意业务库的内容一致性。重跑不重置已变动库存；新订单上线后不要将它当成修复工具。
- 决定：库存测试和 seed 测试各用独立数据库，并在清理前严格校验库名。原因是 `go test ./...` 会并行运行不同包，不能让两个测试互相清表。
- 决定：RPC 的 context 一直传到 QueryRowContext/ExecContext；客户端关闭业务重试。超时表示调用方停止等待，不保证 UPDATE 没有提交，也不保证远端 SQL 在同一瞬间终止。

## 2026-10-23：三层说明规范与 archcheck 的精确例外

- 决定：按用户要求，把 main 的 `3aa6c99` 合入阶段 1，再扩展 commentcheck 的文件说明、doc.go 文件清单校验和 `-map` 模式；同时补齐阶段 0、1 的目录与文件说明。
- 原因：目录说明帮助找入口，文件说明解释拆分理由，代码注释解释关键判断；函数注释不再机械罗列“不阻塞、并发安全”。放弃用统一模板替代实际讲解。
- 发现的冲突：第 5.1.8 节要求 `cmd/README.md`，旧 archcheck 把 cmd 下所有非 main.go 文件都判错。
- 用户授权：在对话中明确同意只放行精确路径 `cmd/README.md`，并要求在计划书第 3.4 节写明“cmd/ 目录本身可以放 README.md”。两处已同步。
- 最小调整：只修改 `locationProblem` 的这一条路径判断，新增对应正例，并继续拒绝 `cmd/demo/README.md`、`cmd/demo/doc.go` 和 `cmd/demo/extra.go`。
- 放弃的方案：放宽整个 cmd 子树、取消薄入口约束，或为了绕过工具把必需的目录说明省掉。除此项与用户要求的 commentcheck 扩展，阶段 0 的 Go 文件只改说明；以忽略注释和位置的 AST 比较及 Go 指令逐行比较核验。
