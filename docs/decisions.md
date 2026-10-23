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
