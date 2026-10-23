# 阶段 0 汇报书：地基与仓库整理

## 一、总览

**一句话总结**：完成可独立测试的 Go 基础设施、认证迁移、仓库规则检查与三组件 Compose 环境。

**做了什么**：建立一个 Go 模块和统一的文件规范。六个 platform 包提供环境配置、日志、HTTP 防护、MySQL 连接、嵌入式迁移和停机流程，认证包已独立迁入但未接线。两个源码检查器把注释与依赖方向规则变成可重复执行的检查。Compose 只包含 MySQL、Redis、Kafka，业务功能尚未开始。

2026-10-23 学习前返工：按用户要求将 main 的计划书更新 `6f92d0e` 合入本分支，重写和补充中文注释，在学习材料中新增六种 Go 写法的最小例子，并删除 `.env.example` 中废弃的 `CHAOS_ADDR`、`CHAOS_TOKEN`。全部 Go 文件的逻辑与返工前一致，详细核对和复验记录见[实测记录](../notes.md#2026-10-23注释可读性返工复验)。

**计划完成情况**：

| 阶段 0 的“做”清单 | 状态 | 说明 |
|---|---|---|
| 1. 仓库基础文件 | 已完成 | 模块、忽略规则、换行、编辑规范、环境示例与 README |
| 2. 本阶段目录 | 已完成 | 仅创建有文件的目录，没有预建后续阶段空包 |
| 3. 六个 platform 包 | 已完成 | 有中文注释、单元测试及可选 MySQL 集成测试 |
| 4. 复制认证 | 已完成 | 来源 ai-backend，去除旧 domain 依赖、更新 issuer/audience，保留拒绝场景测试 |
| 5. commentcheck / archcheck | 已完成 | 包含故意违规样例和 -list 清单输出 |
| 6. Compose 骨架 | 已完成 | 三个依赖实际 healthy，均限制内存和本机映射端口 |
| 7. CI | 已完成 | ubuntu-24.04 上静态检查、race、全新 Compose 与 MySQL 集成测试均通过 |
| 8. 文档 | 已完成 | 本报告、学习材料、设计决定和实测记录 |
| 9. AGENTS 常用命令 | 已完成 | 包括启动、检查、测试和清理 |

## 二、新引入的技术与依赖

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| Go 标准库 | 1.27.1 | context、slog、SQL、AST、测试等 | 计划指定，基础能力足够 | 不额外引入配置/日志/测试框架 |
| Gin | v1.12.0 | httpx | 计划技术栈与旧代码一致 | 其他 Web 框架 |
| go-sql-driver/mysql | v1.10.1 | mysqlx | 配合 database/sql，直接理解连接与 SQL | ORM |
| jwt/v5 | v5.3.1 | auth | 成熟的算法与 claims 校验 | 自己写 JWT 签名 |
| x/crypto | v0.58.0 | bcrypt | 沿用旧认证方案 | 自制密码哈希 |
| staticcheck | v0.8.2 | 本机与 CI | 稳定版本且支持本阶段 Go 代码 | 使用不固定的 latest |
| MySQL 镜像 | 8.4.13 | shop-mysql | 8.4 LTS 当前补丁 | 其他数据库大版本 |
| Redis 镜像 | 8.10.2 | redis | 稳定版本，基础缓存实例 | 增加持久化/集群复杂度 |
| apache/kafka 镜像 | 4.4.0 | kafka | 官方单节点 KRaft | ZooKeeper、第三方镜像 |
| Docker Engine / Compose | 本机 29.9.2 / 5.6.1 | 容器验收 | 用户已准备的运行环境 | 不自行安装系统软件 |
| GCC | 本机 16.2.0 | Windows race | 用户准备的 MinGW-w64 编译器 | 仅依赖远端竞态检查 |
| checkout / setup-go | v7.0.1 / v7.0.0 | GitHub Actions | 查询稳定版本后固定 | 浮动主版本引用 |
| CI runner | ubuntu-24.04 | Linux 验证 | 计划要求固定系统 | ubuntu-latest |

间接依赖逐项固定在根目录 go.mod/go.sum；例如 Gin 的编码器依赖不代表本项目实现了 MongoDB 或 HTTP/3 服务。版本来源与理由见[设计决定](../decisions.md)。

## 三、代码明细

### 1. 新增和修改的包

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| internal/platform/config | 新增 | 有类型的环境读取与校验 | Reader、New、NewReader |
| internal/platform/logging | 新增 | UTC JSON 日志与上下文关联 | New、Fields、WithFields、FromContext |
| internal/platform/httpx | 新增 | Gin 公共防护与响应 | RequestIDs、Recovery、BodyLimit、BindJSON、Success、Failure |
| internal/platform/mysqlx | 新增 | MySQL 池与 UTC 会话 | Config、Open |
| internal/platform/migrate | 新增 | 文件迁移、校验和、命名锁 | Apply |
| internal/platform/lifecycle | 新增 | 信号与关闭预算 | Notify、Hook、Shutdown |
| internal/doctor/auth | 新增（迁入） | JWT、邮箱规范化、bcrypt | Tokens、认证函数、Error 分类 |
| tools/commentcheck | 新增 | 注释检查和阅读清单 | Go 命令行程序 |
| tools/archcheck | 新增 | 目录、依赖与静态路径检查 | Go 命令行程序 |

复用来源：ai-backend main `095337690a9a881cba7a8eced3aae950b6016582` 的 `internal/auth`、`internal/store/store.go`、`internal/httpapi/api.go` 和 `.github/workflows/ci.yml`。源仓库只读，未修改。

### 2. 新增和修改的文件

下表由 Git 差异生成并在提交后对照 `git diff --stat main...phase-00-foundation`；“行数”是当前文件总行数，“差异”是新增/删除行数。包含本文及所有文档，不隐藏生成物或配置。

<!-- FILES_START -->
| 文件 | 作用 | 行数 | 差异（+ / -） |
|---|---|---:|---:|
| [.editorconfig](../../.editorconfig) | 编辑器统一 UTF-8、LF 与缩进 | 16 | +16 / -0 |
| [.env.example](../../.env.example) | 演示环境变量与后续契约预留 | 43 | +43 / -0 |
| [.gitattributes](../../.gitattributes) | Git 文本 LF 规范 | 4 | +4 / -0 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 固定 Linux 环境的全套检查与 Compose 集成测试 | 68 | +68 / -0 |
| [.gitignore](../../.gitignore) | 忽略本机凭据、Obsidian 和构建产物 | 13 | +13 / -0 |
| [AGENTS.md](../../AGENTS.md) | 补充阶段 0 常用命令 | 104 | +15 / -1 |
| [README.md](../../README.md) | 当前能力、双 shell 启动和检查说明 | 106 | +106 / -0 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | MySQL、Redis、Kafka 的固定版本、健康检查与资源限制 | 78 | +78 / -0 |
| [deploy/mysql/init/001_test_databases.sql](../../deploy/mysql/init/001_test_databases.sql) | 隔离测试库与权限初始化，不建业务表 | 9 | +9 / -0 |
| [docs/decisions.md](../../docs/decisions.md) | 版本选择、复用来源与实现取舍 | 53 | +53 / -0 |
| [docs/learn/phase-00-foundation.md](../../docs/learn/phase-00-foundation.md) | 逐文件阅读、设计解释、实验和必答问题 | 400 | +400 / -0 |
| [docs/notes.md](../../docs/notes.md) | 真实运行命令、失败修复和验收证据 | 128 | +128 / -0 |
| [docs/reports/phase-00-foundation.md](../../docs/reports/phase-00-foundation.md) | 本阶段完整变更与验收报告 | 348 | +348 / -0 |
| [go.mod](../../go.mod) | 模块路径、Go 与依赖版本 | 42 | +42 / -0 |
| [go.sum](../../go.sum) | 模块内容校验和 | 95 | +95 / -0 |
| [internal/doctor/auth/auth.go](../../internal/doctor/auth/auth.go) | 独立 JWT、邮箱和 bcrypt 能力 | 126 | +126 / -0 |
| [internal/doctor/auth/auth_test.go](../../internal/doctor/auth/auth_test.go) | 保留旧认证拒绝场景并补配置测试 | 122 | +122 / -0 |
| [internal/doctor/auth/doc.go](../../internal/doctor/auth/doc.go) | auth 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/config/config.go](../../internal/platform/config/config.go) | 环境读取函数注入和类型范围验证 | 91 | +91 / -0 |
| [internal/platform/config/config_test.go](../../internal/platform/config/config_test.go) | 配置缺省、空值、格式与边界表格测试 | 73 | +73 / -0 |
| [internal/platform/config/doc.go](../../internal/platform/config/doc.go) | config 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/httpx/doc.go](../../internal/platform/httpx/doc.go) | httpx 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/httpx/httpx.go](../../internal/platform/httpx/httpx.go) | Request ID、JSON 响应、体积限制与安全恢复 | 141 | +141 / -0 |
| [internal/platform/httpx/httpx_test.go](../../internal/platform/httpx/httpx_test.go) | 内存 HTTP 请求、超限与敏感信息不泄露测试 | 102 | +102 / -0 |
| [internal/platform/lifecycle/doc.go](../../internal/platform/lifecycle/doc.go) | lifecycle 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/lifecycle/lifecycle.go](../../internal/platform/lifecycle/lifecycle.go) | 信号 context 和有总预算的资源关闭 | 59 | +59 / -0 |
| [internal/platform/lifecycle/lifecycle_test.go](../../internal/platform/lifecycle/lifecycle_test.go) | 取消、关闭顺序和超时测试 | 66 | +66 / -0 |
| [internal/platform/logging/doc.go](../../internal/platform/logging/doc.go) | logging 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/logging/logging.go](../../internal/platform/logging/logging.go) | UTC JSON 日志、context 关联和字段分组 | 146 | +146 / -0 |
| [internal/platform/logging/logging_test.go](../../internal/platform/logging/logging_test.go) | 日志字段、UTC 与分组关联测试 | 60 | +60 / -0 |
| [internal/platform/migrate/doc.go](../../internal/platform/migrate/doc.go) | migrate 包的职责、调用方、依赖与边界 | 5 | +5 / -0 |
| [internal/platform/migrate/files.go](../../internal/platform/migrate/files.go) | 迁移命名、顺序和文件校验和 | 60 | +60 / -0 |
| [internal/platform/migrate/files_test.go](../../internal/platform/migrate/files_test.go) | 内存文件系统的加载与历史保护测试 | 56 | +56 / -0 |
| [internal/platform/migrate/migrate.go](../../internal/platform/migrate/migrate.go) | 同连接命名锁、SQL 执行与历史校验 | 224 | +224 / -0 |
| [internal/platform/migrate/migrate_integration_test.go](../../internal/platform/migrate/migrate_integration_test.go) | 真实 MySQL 并发迁移、取消和历史校验 | 118 | +118 / -0 |
| [internal/platform/migrate/testdata/0001_probe.sql](../../internal/platform/migrate/testdata/0001_probe.sql) | 嵌入式迁移测试探针表 | 5 | +5 / -0 |
| [internal/platform/mysqlx/doc.go](../../internal/platform/mysqlx/doc.go) | mysqlx 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [internal/platform/mysqlx/mysql.go](../../internal/platform/mysqlx/mysql.go) | 连接池、UTC 与驱动参数验证 | 92 | +92 / -0 |
| [internal/platform/mysqlx/mysql_integration_test.go](../../internal/platform/mysqlx/mysql_integration_test.go) | 真实 MySQL 会话时区验证 | 38 | +38 / -0 |
| [internal/platform/mysqlx/mysql_test.go](../../internal/platform/mysqlx/mysql_test.go) | 无网络连接参数验证 | 37 | +37 / -0 |
| [tools/archcheck/check.go](../../tools/archcheck/check.go) | 目录、导入、分层与静态路径规则 | 381 | +381 / -0 |
| [tools/archcheck/check_test.go](../../tools/archcheck/check_test.go) | 允许和禁止依赖、布局、答案读取负例 | 134 | +134 / -0 |
| [tools/archcheck/doc.go](../../tools/archcheck/doc.go) | archcheck 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [tools/archcheck/main.go](../../tools/archcheck/main.go) | 架构检查命令入口 | 33 | +33 / -0 |
| [tools/commentcheck/check.go](../../tools/commentcheck/check.go) | AST 注释规则与类型函数清单 | 248 | +248 / -0 |
| [tools/commentcheck/check_test.go](../../tools/commentcheck/check_test.go) | 缺注释、生成文件、长函数和列表测试 | 108 | +108 / -0 |
| [tools/commentcheck/doc.go](../../tools/commentcheck/doc.go) | commentcheck 包的职责、调用方、依赖与边界 | 4 | +4 / -0 |
| [tools/commentcheck/main.go](../../tools/commentcheck/main.go) | 注释检查参数、输出和失败退出 | 53 | +53 / -0 |
| [进度.md](../../进度.md) | 只更新阶段 0 交付与验收状态 | 163 | +14 / -14 |
| [项目计划书.md](../../项目计划书.md) | 按 5.8.4 登记新增 Compose/测试环境变量 | 1766 | +3 / -0 |
<!-- FILES_END -->

### 3. 类型与函数清单

初稿来自 `go run ./tools/commentcheck -list ./...`，随后补充“被谁调用”。列出全部类型和函数，包含测试与辅助函数，超出计划要求的导出/关键函数最小范围。未接线的 API 明确写为预留，不暗示已有业务调用。

<!-- SYMBOLS_START -->
| 包 | 名称 | 位置（文件:行号） | 作用 | 被谁调用 |
|---|---|---|---|---|
| internal/doctor/auth (auth) | HashPassword | internal/doctor/auth/auth.go:107:1 | HashPassword 用 bcrypt 的默认成本计算摘要；输入越界返回 ErrInvalid，底层错误保留原因 | 认证单元测试；阶段 5 登录接入预留 |
| internal/doctor/auth (auth) | VerifyPassword | internal/doctor/auth/auth.go:121:1 | VerifyPassword 比较摘要和输入；不区分错误密码与损坏摘要，统一返回 false 避免泄露细节 | 认证单元测试；阶段 5 登录接入预留 |
| internal/doctor/auth (auth) | Error | internal/doctor/auth/auth.go:14:6 | Error 是可比较的领域错误；用常量而不是可被重新赋值的包级变量，方便 errors.Is 判断 | 认证错误常量、errors.Is |
| internal/doctor/auth (auth) | Error.Error | internal/doctor/auth/auth.go:17:1 | Error 实现标准库 error 接口；不阻塞，不包含调用方的密码或令牌 | error 接口调用方 |
| internal/doctor/auth (auth) | Tokens | internal/doctor/auth/auth.go:36:6 | Tokens 保存 JWT 签名配置；字段构造后不变，Issue 和 Parse 可以并发调用 | NewTokens 与认证测试 |
| internal/doctor/auth (auth) | NewTokens | internal/doctor/auth/auth.go:42:1 | NewTokens 校验密钥长度和有效期；无效时返回配置错误，不回显密钥，不执行 IO | 认证单元测试；阶段 5 app 预留 |
| internal/doctor/auth (auth) | *Tokens.Issue | internal/doctor/auth/auth.go:54:1 | Issue 为非空 userID 签发令牌；返回签名字符串或带上下文的签名错误，不执行网络请求 | 认证单元测试 |
| internal/doctor/auth (auth) | *Tokens.Parse | internal/doctor/auth/auth.go:76:1 | Parse 同时校验签名算法、签发方、受众、有效期和用户标识，成功返回 userID | 认证单元测试 |
| internal/doctor/auth (auth) | NormalizeEmail | internal/doctor/auth/auth.go:91:1 | NormalizeEmail 返回规范化邮箱；展示名、非法地址和过长输入返回 ErrInvalid | 认证单元测试 |
| internal/doctor/auth (auth) | TestEmailAndPasswordValidation | internal/doctor/auth/auth_test.go:13:1 | TestEmailAndPasswordValidation 保留旧仓库的输入边界和密码摘要回归测试，避免迁移时弱化认证 | go test / testing 测试运行器 |
| internal/doctor/auth (auth) | TestTokenConfiguration | internal/doctor/auth/auth_test.go:50:1 | TestTokenConfiguration 验证启动时拒绝短密钥和无效有效期，防止签出无法使用的令牌 | go test / testing 测试运行器 |
| internal/doctor/auth (auth) | TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering | internal/doctor/auth/auth_test.go:69:1 | TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering 验证算法、签发方、受众、有效期和篡改均被拒绝 | go test / testing 测试运行器 |
| internal/platform/config (config) | Reader | internal/platform/config/config.go:13:6 | Reader 持有环境变量的读取方式；测试可以传入内存中的假环境，避免污染其他测试 | New、NewReader、读取方法 |
| internal/platform/config (config) | New | internal/platform/config/config.go:19:1 | New 使用操作系统环境变量构造读取器；它不缓存值，不阻塞，不会返回错误 | 后续 app 预留，阶段 0 未接线 |
| internal/platform/config (config) | NewReader | internal/platform/config/config.go:25:1 | NewReader 注入读取函数；lookup 必须非 nil，返回值遵循 os.LookupEnv 的约定 | New、TestReader |
| internal/platform/config (config) | Reader.String | internal/platform/config/config.go:31:1 | String 返回去除首尾空白的配置；未设置时采用 fallback，显式空值返回带变量名的错误 | Int、Duration、Bool、TestReader |
| internal/platform/config (config) | Reader.Int | internal/platform/config/config.go:47:1 | Int 读取闭区间 [minimum, maximum] 中的十进制整数；无效格式或越界都会返回错误 | TestReader |
| internal/platform/config (config) | Reader.Duration | internal/platform/config/config.go:62:1 | Duration 读取带单位的时长（例如 800ms、5m），并验证闭区间；单位统一为 time.Duration | TestReader |
| internal/platform/config (config) | Reader.Bool | internal/platform/config/config.go:77:1 | Bool 只接受 true 或 false，避免 1、t 等缩写让配置难以阅读；未设置时使用 fallback | TestReader |
| internal/platform/config (config) | TestReader | internal/platform/config/config_test.go:10:1 | TestReader 验证缺省、显式空值、格式和边界，避免错误配置带到服务运行以后才暴露 | go test / testing 测试运行器 |
| internal/platform/httpx (httpx) | BindJSON | internal/platform/httpx/httpx.go:108:1 | BindJSON 严格解析一个 JSON 值到非 nil 指针 target；失败时已写出安全响应并中断请求链 | HTTP 单元测试处理器 |
| internal/platform/httpx (httpx) | Success | internal/platform/httpx/httpx.go:133:1 | Success 写出契约规定的成功响应；status 由接口决定（例如查询 200、创建 201） | HTTP 单元测试处理器 |
| internal/platform/httpx (httpx) | Failure | internal/platform/httpx/httpx.go:139:1 | Failure 写出公开错误并中断当前处理链；code/message 必须是服务定义的安全文本，不能直接传 err.Error() | Recovery、BodyLimit、BindJSON |
| internal/platform/httpx (httpx) | RequestID | internal/platform/httpx/httpx.go:23:1 | RequestID 返回当前上下文里的标识；须先安装 RequestIDs 中间件，否则返回空字符串 | Success、Failure |
| internal/platform/httpx (httpx) | RequestIDs | internal/platform/httpx/httpx.go:29:1 | RequestIDs 校验或生成请求标识，并记录安全的访问摘要；logger 必须非 nil | testRouter，后续接口层预留 |
| internal/platform/httpx (httpx) | validRequestID | internal/platform/httpx/httpx.go:50:1 | validRequestID 只接受简单 ASCII 标识；无效输入会被替换，而不是原样带入日志或响应头 | RequestIDs、HTTP 单元测试 |
| internal/platform/httpx (httpx) | Recovery | internal/platform/httpx/httpx.go:66:1 | Recovery 捕获当前请求 goroutine 的 panic，输出不含内部细节的 500；logger 必须非 nil | testRouter |
| internal/platform/httpx (httpx) | BodyLimit | internal/platform/httpx/httpx.go:88:1 | BodyLimit 给每个请求安装有上限的 Reader；limit 必须大于 0，通常使用 DefaultBodyLimit | testRouter |
| internal/platform/httpx (httpx) | testRouter | internal/platform/httpx/httpx_test.go:17:1 | testRouter 组装纯内存路由，不监听端口；handler 注入每个用例要验证的行为 | TestRequestIDAndRecovery、TestJSONBodyLimit |
| internal/platform/httpx (httpx) | TestRequestIDAndRecovery | internal/platform/httpx/httpx_test.go:27:1 | TestRequestIDAndRecovery 验证合法标识保留、非法标识替换，以及 panic 不泄露原始内容 | go test / testing 测试运行器 |
| internal/platform/httpx (httpx) | TestJSONBodyLimit | internal/platform/httpx/httpx_test.go:60:1 | TestJSONBodyLimit 检查未知字段、多个 JSON、已知长度和流式超限，防止只检查 Content-Length | go test / testing 测试运行器 |
| internal/platform/lifecycle (lifecycle) | Hook | internal/platform/lifecycle/lifecycle.go:14:6 | Hook 表示一个关闭步骤；顺序由组装层确定，例如先 HTTP，再数据库 | Shutdown 调用方、停机测试 |
| internal/platform/lifecycle (lifecycle) | Notify | internal/platform/lifecycle/lifecycle.go:21:1 | Notify 在 Ctrl+C、SIGTERM 或父上下文取消时取消返回的 context | TestNotifyParentCancel，后续 app 预留 |
| internal/platform/lifecycle (lifecycle) | Shutdown | internal/platform/lifecycle/lifecycle.go:30:1 | Shutdown 按顺序执行 hooks，共用 timeout 这一总预算；返回所有关闭错误的合并结果 | 停机单元测试，后续 app 预留 |
| internal/platform/lifecycle (lifecycle) | TestShutdownOrderAndCancellation | internal/platform/lifecycle/lifecycle_test.go:12:1 | TestShutdownOrderAndCancellation 验证正常工作取消后仍有清理时间，并且失败不会跳过后续关闭 | go test / testing 测试运行器 |
| internal/platform/lifecycle (lifecycle) | TestShutdownDeadline | internal/platform/lifecycle/lifecycle_test.go:40:1 | TestShutdownDeadline 用响应取消的假服务验证总预算，不创建真实服务器或后台线程 | go test / testing 测试运行器 |
| internal/platform/lifecycle (lifecycle) | TestNotifyParentCancel | internal/platform/lifecycle/lifecycle_test.go:53:1 | TestNotifyParentCancel 验证无需向测试进程发系统信号也能停止监听，避免影响并行测试 | go test / testing 测试运行器 |
| internal/platform/logging (logging) | handler.WithGroup | internal/platform/logging/logging.go:103:1 | WithGroup 创建带字段分组的派生 Handler，保留标准库的分组语义，不执行 IO | slog.Logger.WithGroup |
| internal/platform/logging (logging) | groupAttributes | internal/platform/logging/logging.go:114:1 | groupAttributes 从内到外包装业务属性；不修改传入切片，避免派生日志器影响父日志器 | Handle、WithAttrs |
| internal/platform/logging (logging) | contextKey | internal/platform/logging/logging.go:11:6 | contextKey 是本包专用的键类型，防止其他包用相同字符串覆盖日志上下文 | WithFields、FromContext |
| internal/platform/logging (logging) | mergeGroups | internal/platform/logging/logging.go:125:1 | mergeGroups 合并固定属性和当前属性中同名的组，避免 JSON 出现重复组键后被解析器覆盖 | Handle、自身递归 |
| internal/platform/logging (logging) | Fields | internal/platform/logging/logging.go:15:6 | Fields 是一次请求的关联信息；字符串为空表示当前上下文没有该项 | httpx 和日志测试 |
| internal/platform/logging (logging) | WithFields | internal/platform/logging/logging.go:23:1 | WithFields 返回带关联信息的派生上下文，不改变父上下文；不会阻塞，也不会返回错误 | RequestIDs、日志测试 |
| internal/platform/logging (logging) | FromContext | internal/platform/logging/logging.go:28:1 | FromContext 返回上下文中的关联信息；不存在时返回零值，不阻塞且可并发调用 | httpx、handler.Handle |
| internal/platform/logging (logging) | handler | internal/platform/logging/logging.go:36:6 | handler 在 JSON Handler 外包一层，自动补齐每次请求的标识，不创建 goroutine | New、WithAttrs、WithGroup |
| internal/platform/logging (logging) | New | internal/platform/logging/logging.go:44:1 | New 创建固定 service 字段的 JSON 日志器；writer 必须非 nil，level 控制最低级别 | testRouter、日志测试 |
| internal/platform/logging (logging) | utcTime | internal/platform/logging/logging.go:53:1 | utcTime 只规范日志顶层时间，不改业务字段；保证不同机器的记录能按 UTC 对齐 | slog JSON Handler 的 ReplaceAttr |
| internal/platform/logging (logging) | handler.Enabled | internal/platform/logging/logging.go:62:1 | Enabled 委托下层决定级别是否启用，不提前构造字符串，减少被过滤日志的开销 | slog.Logger |
| internal/platform/logging (logging) | handler.Handle | internal/platform/logging/logging.go:68:1 | Handle 从当前 context 补充非空标识，并返回下层写出错误；写入可能阻塞 | slog.Logger |
| internal/platform/logging (logging) | handler.WithAttrs | internal/platform/logging/logging.go:95:1 | WithAttrs 创建带固定字段的派生 Handler，不修改原对象，因此多个日志器可以安全共享 | slog.Logger.With |
| internal/platform/logging (logging) | TestJSONFields | internal/platform/logging/logging_test.go:13:1 | TestJSONFields 检查日志格式、UTC 时间和 context 关联信息，避免跨服务排查时无法串联请求 | go test / testing 测试运行器 |
| internal/platform/logging (logging) | TestGroupedFieldsStayCorrelated | internal/platform/logging/logging_test.go:43:1 | TestGroupedFieldsStayCorrelated 确保业务字段分组后关联标识仍在顶层，日志查询不需要猜分组路径 | go test / testing 测试运行器 |
| internal/platform/migrate (migrate) | migration | internal/platform/migrate/files.go:13:6 | migration 是一份已加载的迁移，不导出，避免调用方绕过文件校验后直接执行任意片段 | load、Apply、verifyHistory |
| internal/platform/migrate (migrate) | load | internal/platform/migrate/files.go:21:1 | load 按文件系统字典序读取迁移；格式、重复序号、空文件或读取失败都会返回带文件名的错误 | Apply、文件单元测试 |
| internal/platform/migrate (migrate) | TestLoadRejectsInvalidFiles | internal/platform/migrate/files_test.go:41:1 | TestLoadRejectsInvalidFiles 检查不合法命名、重复序号和空文件，保证失败发生在执行 SQL 之前 | go test / testing 测试运行器 |
| internal/platform/migrate (migrate) | TestLoadAndHistory | internal/platform/migrate/files_test.go:9:1 | TestLoadAndHistory 验证排序、校验和和只追加规则；用内存文件系统，不需要数据库 | go test / testing 测试运行器 |
| internal/platform/migrate (migrate) | acquire | internal/platform/migrate/migrate.go:103:1 | acquire 根据当前库名隔离命名锁；返回锁名或明确的获取错误，不记录连接串 | Apply、verifyCancellation |
| internal/platform/migrate (migrate) | readHistory | internal/platform/migrate/migrate.go:129:1 | readHistory 读取已执行版本与校验和；同步查询，所有读取和关闭错误都返回给调用方 | Apply |
| internal/platform/migrate (migrate) | verifyHistory | internal/platform/migrate/migrate.go:153:1 | verifyHistory 拒绝改写、移除或在旧版本前插入迁移；不联网，不修改输入数据 | Apply、TestLoadAndHistory |
| internal/platform/migrate (migrate) | release | internal/platform/migrate/migrate.go:180:1 | release 使用独立超时释放命名锁；失败时丢弃物理连接，避免把仍持锁的会话放回连接池 | Apply 的 defer、verifyCancellation |
| internal/platform/migrate (migrate) | discard | internal/platform/migrate/migrate.go:199:1 | discard 通知连接池丢弃这条连接，避免别的请求复用一条可能仍持锁的会话 | acquire、release |
| internal/platform/migrate (migrate) | closeConnection | internal/platform/migrate/migrate.go:214:1 | closeConnection 归还租用连接；已经被 discard 关闭是预期状态，其他清理错误保留 | Apply、verifyCancellation |
| internal/platform/migrate (migrate) | Apply | internal/platform/migrate/migrate.go:56:1 | Apply 在同一数据库连接上持锁、检查历史、执行新迁移并记录结果 | 集成测试，后续服务启动预留 |
| internal/platform/migrate (migrate) | integrationDatabase | internal/platform/migrate/migrate_integration_test.go:24:1 | integrationDatabase 连接专用测试库；缺少开关时跳过，避免普通 go test 访问网络 | TestApplyIntegration |
| internal/platform/migrate (migrate) | TestApplyIntegration | internal/platform/migrate/migrate_integration_test.go:56:1 | TestApplyIntegration 验证两个并发启动者只记一次迁移，改写被拒绝，取消后锁仍能正确释放 | go test / testing 测试运行器 |
| internal/platform/migrate (migrate) | verifyCancellation | internal/platform/migrate/migrate_integration_test.go:91:1 | verifyCancellation 持锁后取消调用方，再释放并重新获取，验证清理没有复用已取消的 context | TestApplyIntegration |
| internal/platform/mysqlx (mysqlx) | Config | internal/platform/mysqlx/mysql.go:21:6 | Config 描述一个服务独占的连接池，实际环境变量在服务 app 包读取 | Open、prepare、MySQL/迁移测试 |
| internal/platform/mysqlx (mysqlx) | Open | internal/platform/mysqlx/mysql.go:30:1 | Open 创建连接池并 Ping 验证可达性；最多等待 connectTimeout 或更短的 ctx deadline | MySQL/迁移集成测试，后续 app 预留 |
| internal/platform/mysqlx (mysqlx) | prepare | internal/platform/mysqlx/mysql.go:60:1 | prepare 校验连接池参数并规范驱动设置，不执行 IO；解析失败不保留可能包含密码的底层错误 | Open、TestPrepare |
| internal/platform/mysqlx (mysqlx) | TestOpenIntegration | internal/platform/mysqlx/mysql_integration_test.go:11:1 | TestOpenIntegration 验证真实会话使用 UTC；没有专门的测试 DSN 时明确跳过，普通测试不联网 | go test / testing 测试运行器 |
| internal/platform/mysqlx (mysqlx) | TestPrepare | internal/platform/mysqlx/mysql_test.go:10:1 | TestPrepare 验证 Go 日期解码和数据库会话都使用 UTC，并禁止多语句查询 | go test / testing 测试运行器 |
| tools/archcheck (main) | locationProblem | tools/archcheck/check.go:111:1 | locationProblem 检查根目录 Go 文件、cmd 入口文件数量和含糊的包名；返回空字符串表示通过 | check、TestLocations |
| tools/archcheck (main) | under | tools/archcheck/check.go:133:1 | under 按目录边界匹配路径，防止把 internal/doctorish 误当成 internal/doctor | 导入、分层、场景规则 |
| tools/archcheck (main) | importProblem | tools/archcheck/check.go:136:1 | importProblem 检查领域隔离和包内文件角色；不限制合法的 platform 依赖或消费者声明的接口 | inspectSource |
| tools/archcheck (main) | layerProblem | tools/archcheck/check.go:168:1 | layerProblem 按文件角色限制传输和存储依赖；领域流程只应认识接口，不直接认识具体后端 | importProblem |
| tools/archcheck (main) | check | tools/archcheck/check.go:18:1 | check 从 go.mod 获取真实模块路径后遍历源码；读取或语法错误直接返回，避免把漏检当作通过 | main、工具测试 |
| tools/archcheck (main) | scenarioProblems | tools/archcheck/check.go:204:1 | scenarioProblems 检查文件读取调用和 go:embed；允许 chaos/eval 读取答案，其余包只要可静态识别就报错 | inspectSource |
| tools/archcheck (main) | importAliases | tools/archcheck/check.go:244:1 | importAliases 记录标准库读取包的本地名字；别名导入不能绕过读取检查 | scenarioProblems |
| tools/archcheck (main) | isReadCall | tools/archcheck/check.go:263:1 | isReadCall 识别 os/io/fs/path/filepath 的文件入口；对点导入也检查，避免改导入写法绕过规则 | scenarioProblems |
| tools/archcheck (main) | stringValues | tools/archcheck/check.go:293:1 | stringValues 收集可静态求值的字符串声明与赋值；多轮展开支持文件内的常量引用链 | scenarioProblems |
| tools/archcheck (main) | stringExpression | tools/archcheck/check.go:339:1 | stringExpression 展开字符串、标识符、加法和 Join 调用；未知表达式返回空值，不猜测运行时结果 | 场景读取分析、自身递归 |
| tools/archcheck (main) | isScenarioPath | tools/archcheck/check.go:375:1 | isScenarioPath 统一 Windows 分隔符并清理相对路径，识别标准答案目录及其子文件 | scenarioProblems |
| tools/archcheck (main) | inspectSource | tools/archcheck/check.go:68:1 | inspectSource 解析单个 Go 文件并分别检查依赖和答案读取；语法错误不会被当成无违规 | check 的目录遍历 |
| tools/archcheck (main) | modulePath | tools/archcheck/check.go:94:1 | modulePath 只读取 go.mod 的 module 行，不要求先下载依赖；缺失或不合法时明确失败 | check |
| tools/archcheck (main) | TestScenarioReads | tools/archcheck/check_test.go:101:1 | TestScenarioReads 验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案 | go test / testing 测试运行器 |
| tools/archcheck (main) | fixture | tools/archcheck/check_test.go:11:1 | fixture 创建独立的假模块，检查器只解析这些文件，不需要下载被禁止的依赖 | 工具测试 |
| tools/archcheck (main) | TestForbiddenImports | tools/archcheck/check_test.go:28:1 | TestForbiddenImports 覆盖领域隔离和分层违规，包括不能靠“包名前缀相似”产生误报的边界 | go test / testing 测试运行器 |
| tools/archcheck (main) | TestLocations | tools/archcheck/check_test.go:84:1 | TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报 | go test / testing 测试运行器 |
| tools/archcheck (main) | main | tools/archcheck/main.go:10:1 | main 检查指定模块根目录；默认 ./...，违规时返回非零退出码供 CI 使用 | Go 程序入口 |
| tools/commentcheck (main) | inspectFunction | tools/commentcheck/check.go:124:1 | inspectFunction 检查文档和长函数内部注释，并记录方法接收者，防止同名方法在清单里混淆 | inspectFile |
| tools/commentcheck (main) | receiverName | tools/commentcheck/check.go:157:1 | receiverName 直接读取 AST 类型表达式；支持指针和泛型接收者，不引入反射或类型加载依赖 | inspectFunction、自身递归 |
| tools/commentcheck (main) | inspectDeclaration | tools/commentcheck/check.go:178:1 | inspectDeclaration 检查包级类型、常量、变量；常量和变量允许组注释，局部变量不在机器检查范围 | inspectFile |
| tools/commentcheck (main) | symbol | tools/commentcheck/check.go:19:6 | symbol 是 -list 的一行；字段只保存源码信息，不执行或反射被检查的对象 | 声明检查与 writeList |
| tools/commentcheck (main) | hasDoc | tools/commentcheck/check.go:211:1 | hasDoc 不把空白注释或单独的 go: 指令当成说明，防止只有形式却没有可读内容 | 文档和声明检查 |
| tools/commentcheck (main) | firstSentence | tools/commentcheck/check.go:214:1 | firstSentence 以换行或中文句号结束第一句；清单是初稿，不替代完整文档注释 | 声明检查 |
| tools/commentcheck (main) | writeList | tools/commentcheck/check.go:229:1 | writeList 按包和位置稳定排序，输出 Markdown 表格；写入错误会返回，避免生成不完整的汇报书 | run |
| tools/commentcheck (main) | scanResult | tools/commentcheck/check.go:27:6 | scanResult 累积一次扫描结果；每次调用独占它，不存在跨扫描共享的可变全局数据 | scan、声明检查 |
| tools/commentcheck (main) | scan | tools/commentcheck/check.go:34:1 | scan 遍历 patterns 指定的目录（支持 ./...）；语法错误和读取错误直接返回，不静默漏检 | run、工具测试 |
| tools/commentcheck (main) | inspectFile | tools/commentcheck/check.go:88:1 | inspectFile 解析单个文件并检查声明；标准生成标记由 ast.IsGenerated 识别，生成物跳过 | scan 的目录遍历 |
| tools/commentcheck (main) | fixture | tools/commentcheck/check_test.go:12:1 | fixture 把源码样例写进独立临时目录；t.TempDir 负责清理，不修改真实仓库文件 | 工具测试 |
| tools/commentcheck (main) | TestMissingComments | tools/commentcheck/check_test.go:28:1 | TestMissingComments 逐一删除不同位置的注释，证明检查器会失败且给出文件和行号 | go test / testing 测试运行器 |
| tools/commentcheck (main) | TestPackageAndGenerated | tools/commentcheck/check_test.go:64:1 | TestPackageAndGenerated 验证空 doc.go 不能通过，标准生成文件则不要求手写注释 | go test / testing 测试运行器 |
| tools/commentcheck (main) | TestListAndGroupedComments | tools/commentcheck/check_test.go:77:1 | TestListAndGroupedComments 验证分组声明、15 行边界、长函数内部注释和清单输出 | go test / testing 测试运行器 |
| tools/commentcheck (main) | TestSyntaxErrorAndExit | tools/commentcheck/check_test.go:97:1 | TestSyntaxErrorAndExit 检查语法错误和违规不会在 CLI 入口被吞掉，确保 CI 能收到失败 | go test / testing 测试运行器 |
| tools/commentcheck (main) | main | tools/commentcheck/main.go:11:1 | main 把命令行参数交给 run，并把失败转换为非零退出码，供 CI 阻止缺注释的提交 | Go 程序入口 |
| tools/commentcheck (main) | run | tools/commentcheck/main.go:20:1 | run 解析 -list 与目录参数；检查错误或文件读取错误都会返回，输出写入调用方提供的 writer | main、CLI 单元测试 |
<!-- SYMBOLS_END -->

### 4. 数据库变更

无业务表。Compose 初始化空 `shop` 库及 `opsdoctor_mysql_test`、`opsdoctor_migrate_test` 两个隔离测试库，见 `deploy/mysql/init/001_test_databases.sql`。migrate 运行时维护 `schema_migrations(version 主键, checksum, applied_at)`；嵌入式测试文件只在测试库创建 `migration_probe(id 主键)`。测试会清理这两个技术表，代码强制验证测试库名。

### 5. 接口变更

无对外业务 HTTP/gRPC 接口。httpx 提供统一响应构造函数；测试路由仅存在于内存中。基础设施端口：127.0.0.1:3306、6379、9092；Kafka 容器内监听 29092，controller 9093 不映射到宿主机。

### 6. 配置变更

| 变量 | 含义 | 默认值 / 本阶段使用情况 |
|---|---|---|
| MYSQL_ROOT_PASSWORD / MYSQL_PASSWORD | Compose 初始化数据库密码 | 无代码默认值，必须环境注入；示例是假值 |
| TEST_MYSQL_DSN | 启用真实连接测试 | 未设置时跳过 |
| TEST_MIGRATE_MYSQL_DSN | 启用可清理的迁移测试库 | 未设置时跳过；只允许 opsdoctor_migrate_test |
| MYSQL_DSN | 后续服务连接串 | 本阶段只有示例，服务尚未接线 |
| DB_MAX_OPEN_CONNS / DB_MAX_IDLE_CONNS / DB_CONN_MAX_LIFETIME | 后续 app 连接池配置 | 示例 20 / 10 / 5m；本阶段由调用方显式构造 Config |
| JWT_SECRET | 后续医生签名密钥 | 无运行时默认值；构造函数要求至少 32 字节 |

其余 `.env.example` 变量是计划书 5.8.4 的明确预留，暂未消费。新增四个 Compose/测试变量已同步登记计划书；没有更改业务接口契约。

### 7. 部署变更

新增 Compose 和 MySQL 初始化 SQL。MySQL/Redis/Kafka 内存上限分别为 768/256/1024 MiB（配置预算，非实测内存）；各有健康检查。MySQL 与 Kafka 使用命名卷；Redis 缓存关闭持久化。未创建 Go 服务镜像，因为本阶段没有服务入口。

## 四、测试

### 1. 新增测试清单

<!-- TESTS_START -->
| 测试名 | 所在文件 | 验证场景 |
|---|---|---|
| TestEmailAndPasswordValidation | internal/doctor/auth/auth_test.go | TestEmailAndPasswordValidation 保留旧仓库的输入边界和密码摘要回归测试，避免迁移时弱化认证 |
| TestTokenConfiguration | internal/doctor/auth/auth_test.go | TestTokenConfiguration 验证启动时拒绝短密钥和无效有效期，防止签出无法使用的令牌 |
| TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering | internal/doctor/auth/auth_test.go | TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering 验证算法、签发方、受众、有效期和篡改均被拒绝 |
| TestReader | internal/platform/config/config_test.go | TestReader 验证缺省、显式空值、格式和边界，避免错误配置带到服务运行以后才暴露 |
| TestRequestIDAndRecovery | internal/platform/httpx/httpx_test.go | TestRequestIDAndRecovery 验证合法标识保留、非法标识替换，以及 panic 不泄露原始内容 |
| TestJSONBodyLimit | internal/platform/httpx/httpx_test.go | TestJSONBodyLimit 检查未知字段、多个 JSON、已知长度和流式超限，防止只检查 Content-Length |
| TestShutdownOrderAndCancellation | internal/platform/lifecycle/lifecycle_test.go | TestShutdownOrderAndCancellation 验证正常工作取消后仍有清理时间，并且失败不会跳过后续关闭 |
| TestShutdownDeadline | internal/platform/lifecycle/lifecycle_test.go | TestShutdownDeadline 用响应取消的假服务验证总预算，不创建真实服务器或后台线程 |
| TestNotifyParentCancel | internal/platform/lifecycle/lifecycle_test.go | TestNotifyParentCancel 验证无需向测试进程发系统信号也能停止监听，避免影响并行测试 |
| TestJSONFields | internal/platform/logging/logging_test.go | TestJSONFields 检查日志格式、UTC 时间和 context 关联信息，避免跨服务排查时无法串联请求 |
| TestGroupedFieldsStayCorrelated | internal/platform/logging/logging_test.go | TestGroupedFieldsStayCorrelated 确保业务字段分组后关联标识仍在顶层，日志查询不需要猜分组路径 |
| TestLoadRejectsInvalidFiles | internal/platform/migrate/files_test.go | TestLoadRejectsInvalidFiles 检查不合法命名、重复序号和空文件，保证失败发生在执行 SQL 之前 |
| TestLoadAndHistory | internal/platform/migrate/files_test.go | TestLoadAndHistory 验证排序、校验和和只追加规则；用内存文件系统，不需要数据库 |
| TestApplyIntegration | internal/platform/migrate/migrate_integration_test.go | TestApplyIntegration 验证两个并发启动者只记一次迁移，改写被拒绝，取消后锁仍能正确释放 |
| TestOpenIntegration | internal/platform/mysqlx/mysql_integration_test.go | TestOpenIntegration 验证真实会话使用 UTC；没有专门的测试 DSN 时明确跳过，普通测试不联网 |
| TestPrepare | internal/platform/mysqlx/mysql_test.go | TestPrepare 验证 Go 日期解码和数据库会话都使用 UTC，并禁止多语句查询 |
| TestForbiddenImports | tools/archcheck/check_test.go | TestForbiddenImports 覆盖领域隔离和分层违规，包括不能靠“包名前缀相似”产生误报的边界 |
| TestLocations | tools/archcheck/check_test.go | TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报 |
| TestScenarioReads | tools/archcheck/check_test.go | TestScenarioReads 验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案 |
| TestMissingComments | tools/commentcheck/check_test.go | TestMissingComments 逐一删除不同位置的注释，证明检查器会失败且给出文件和行号 |
| TestPackageAndGenerated | tools/commentcheck/check_test.go | TestPackageAndGenerated 验证空 doc.go 不能通过，标准生成文件则不要求手写注释 |
| TestListAndGroupedComments | tools/commentcheck/check_test.go | TestListAndGroupedComments 验证分组声明、15 行边界、长函数内部注释和清单输出 |
| TestSyntaxErrorAndExit | tools/commentcheck/check_test.go | TestSyntaxErrorAndExit 检查语法错误和违规不会在 CLI 入口被吞掉，确保 CI 能收到失败 |
<!-- TESTS_END -->

### 2. 运行命令与结果

gofmt、vet、staticcheck、commentcheck、archcheck 全部通过；本机 `go test -race ./... -count=1 -timeout=180s` 的 9 个包全部通过，其中显式开启两个真实 MySQL 集成测试。完整命令、负例输出、环境版本和首次失败记录见[实测记录](../notes.md)。尚无性能或诊断准确率数据。

## 五、工程验收结果

| 计划验收项 | 结果 | 证据 |
|---|---|---|
| Compose 三个依赖 healthy | 通过 | notes 的 Compose 健康验收；原样命令在已注入环境变量时也成功 |
| 两个检查器通过 | 通过 | commentcheck 扫描 32 个非生成 Go 文件；archcheck 无违规 |
| 删除函数注释能定位报错 | 通过 | `config.go:18:1`、退出状态 1；恢复后通过 |
| CI 通过 | 通过 | [运行 39929219914](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39929219914)，验证提交 de4459208791899131d5f67cbc8d42310b3948d7 |

首次工程验收的源码提交为 `de4459208791899131d5f67cbc8d42310b3948d7`，后续文档提交为 `d9b24f6`。上表中的首次验收证据保留；本次学习前返工重新运行了全部本机检查，结果通过，见 notes 的返工复验记录。`main` 已合入阶段分支，阶段分支没有合入 `main`。

## 六、偏离计划之处

无业务范围或技术栈偏离。最初因本机缺 Docker/gcc，用户明确同意暂用 Linux CI 验证 race；之后用户补齐环境，本机 race 与 Compose 已真实验收。Compose 文档增加显式 `--env-file`，属于启动参数补全。迁移一文件一条语句、历史校验和以及静态检查的能力边界详见[设计决定](../decisions.md)。

## 七、已知问题与遗留事项

- 阶段 1 才增加库存服务、业务表、protobuf 和服务组装；目前无业务请求入口。
- 阶段 3 才接 OpenTelemetry、指标和管理端口；不能声称已有完整可观测性。
- 阶段 5 才把认证接到医生服务；目前无登录接口。
- 迁移不提供 DDL 全批回滚，迁移作者必须使用可重入语句；若 DDL 成功但记录失败，重试会再执行该文件。
- archcheck 不做完整跨文件/运行时数据流分析，不是访问沙箱。后续医生仍必须通过账号、配置和参数层限制读取范围。
- Shutdown 依赖关闭回调响应 context；它不能强行停止不合作的回调。通用包不暗中启动后台 goroutine。
- 本机单节点 Compose 无高可用；尚未做阶段 2 的负载验收。

## 八、注释自查结论

2026-10-23 按用户抽查意见完成第二轮自查：commentcheck 通过，检查 32 个非生成 Go 文件。第一版虽然满足机器的注释存在性规则，但 Go 特有写法解释不够、两个检查器的分支理由太少、测试表格逐例说明不足，部分 SQL 注释也不够自然；这些问题已在本轮修正。按计划书 3.1 逐项核对如下：

| 项目 | 结论 |
|---|---|
| 包 | 9 个包都有 doc.go，说明职责、调用方、依赖和边界 |
| 类型与字段 | 声明处有中文含义；配置字段注明变量名、范围或单位 |
| 函数与方法 | 在使用处解释具名返回值与 defer、errors.Join、WithoutCancel、NotifyContext、NullInt64、Raw 与 ErrBadConn；学习材料各附最小例子 |
| 函数内部 | archcheck/commentcheck 按扫描、解析、分支判断、诊断输出解释理由；补充路径推导的局限、AST 节点含义及其与 Java 类型判断的联系；其余基础包补充配置、清理、日志和 HTTP 边界理由 |
| 测试 | 每个 Test 说明场景；表格逐项说明缺省、边界、无效输入、合法对照、路径拼接等在验证什么，保留原用例数据和断言 |
| 常量与数字 | 请求预算、超时、密钥/密码边界使用具名常量并说明理由；测试数据中的数字是用例输入 |
| SQL | 改写“只读会话状态，没有表索引或行锁”等简写：明确 DATABASE() 查询当前连接的库名、不访问表，所以不涉及表索引和数据行锁；命名锁、历史表和测试 SQL 分别说明 |
| goroutine | 仅迁移并发测试显式创建工作者，context 负责取消，WaitGroup 负责等其退出；signal 内部资源由 stop 释放 |
| 锁与 channel | 数据库命名锁的会话归属与失败释放已说明；测试结果 channel 两个写入者、一个读取者，容量 2，无需关闭 |
| 错误处理 | 说明 errors.Join 为什么保留主流程与清理错误、rows.Err 为什么不能省略，以及主动返回 ErrBadConn/遇到 ErrConnDone 时为什么视为清理已完成；配置错误仍不回显原文 |

无行为改动的核对：用 Go parser 解析返工前 `d9b24f6` 与当前全部 32 个 Go 文件，排除注释和源码位置后逐一比较 AST，结果一致；`go:` 等编译指令也一致。表格换行引入的尾逗号只影响排版。gofmt、vet、staticcheck、commentcheck、archcheck 和本机全部 race 测试已重跑通过，真实 MySQL 两个集成测试没有跳过。学习例子前五个实际运行，最后一个 Raw 函数片段只编译验证，没有声称执行了真实丢弃连接实验。注释仍欢迎逐行抽查，学习验收未勾选。

## 九、学习指引

阅读[阶段 0 学习材料](../learn/phase-00-foundation.md)。先用约 30 分钟跑环境与检查，再按“命令入口 → 配置/日志 → HTTP/认证 → 连接/迁移/停机 → CI”阅读。总计约 4 小时是学习安排建议，不是实测开发或阅读耗时。

## 十、下一阶段预告

阶段 1 将实现库存 gRPC 契约、MySQL 业务迁移、库存服务和数据初始化/手动调用工具。需要保持 Docker 和 Go 环境可用，不需要大模型 API Key。**尚未开始阶段 1，等待用户学习验收并明确同意。**
