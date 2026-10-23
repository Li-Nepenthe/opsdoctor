# 阶段 0 学习材料：先把地基讲清楚

## 1. 这个阶段做了什么

```mermaid
flowchart TD
    A[开发者 / CI] --> B[commentcheck / archcheck]
    A --> C[Go 单元测试与 race]
    A --> D[Docker Compose]
    D --> E[MySQL]
    D --> F[Redis]
    D --> G[Kafka]
    C --> H[config / logging / httpx / lifecycle]
    C --> I[独立 auth 包]
    C --> J[mysqlx / migrate]
    J -. 显式开启的集成测试 .-> E
```

我们先搭建以后服务都要依赖的基础能力，并让规则可以自动检查。此时没有库存、订单或医生业务进程，所以你还不能通过 HTTP 下单，也不会看到诊断页面。Redis、Kafka 目前仅用于把部署环境跑通；不代表已经写好了缓存或消息业务。先读[汇报书](../reports/phase-00-foundation.md)了解文件全貌，再逐个阅读源码。

## 2. 怎么跑起来、看什么

所有命令从仓库根目录运行。首次安装工具后重新打开终端；如果之前已经创建 `.env`，不要覆盖自己的配置。

1. 执行 `go version`、`docker version`、`docker compose version`、`gcc --version`、`go env CGO_ENABLED`。你应该看到 Go 1.27.1、Docker Client/Server 都可用，Windows 下 gcc 可见且 CGO 为 1。gcc 是 race 的构建依赖，本项目没有让你手写 C。
2. PowerShell：`if (-not (Test-Path .env)) { Copy-Item .env.example .env }`；bash：`test -f .env || cp .env.example .env`。你应该得到一个 Git 忽略的 `.env`，里面都是演示值；`git status --short` 不应列出它。
3. 执行 `docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait --wait-timeout 240`。你应该看到三个服务最终都 Healthy；首次下载镜像可能较慢。`--wait-timeout` 约束等待服务就绪的时间，不是镜像下载速度保证。
4. 执行 `docker compose --env-file .env -f deploy/docker-compose.yml ps`。你应该看到 MySQL、Redis、Kafka 的状态为 healthy，端口前缀是 127.0.0.1。
5. 执行 `go install honnef.co/go/tools/cmd/staticcheck@v0.8.2`。成功一般不输出内容；随后 `staticcheck -version` 应显示安装版本。如果命令不可见，把 GOPATH 下的 bin 加入 PATH。
6. 依次执行下列命令。gofmt、vet、staticcheck 正常时无输出；两个项目检查器应显示通过；测试应显示各包 ok。

```text
gofmt -l internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s
```

7. 执行 `go test ./internal/platform/mysqlx ./internal/platform/migrate -run Integration -v`，先不要设置 TEST 开头的 DSN。你应该看到 SKIP 和如何开启的说明；这保证平常测试不依赖你是否启动数据库。
8. 按 [README 的两种 shell 示例](../../README.md#运行真实-mysql-集成测试)把 `.env` 中两个 TEST DSN 注入当前终端，再执行 `go test -race ./internal/platform/mysqlx ./internal/platform/migrate -run Integration -count=1 -timeout=90s -v`。你应该看到 TestOpenIntegration 和 TestApplyIntegration 真正 PASS。后者只允许专用测试库名，并会清理其中的技术表。
9. 执行 `go run ./tools/commentcheck -list ./...`。你应该看到按包列出的名称、文件、行号和第一句注释；可以拿这份清单逐项对照汇报书。
10. 学习结束可执行 `docker compose --env-file .env -f deploy/docker-compose.yml down`。你应该看到容器和网络被移除，命名数据卷保留；不需要加 `-v`。

如果刚启动就失败，先看 `ps --all`。查看 Kafka 的日志可以用 `docker compose --env-file .env -f deploy/docker-compose.yml logs --tail 80 kafka`。不要为了排查配置而把完整 `.env` 或展开后的 Compose 配置贴到公开问题里。

## 3. 阅读顺序

本阶段尚无 `cmd/` 服务入口，不能为了满足阅读顺序而创建空服务。实际可执行入口是两个工具的 main.go；以后阶段再从 `cmd/<服务>/main.go` 进入 app。

| 顺序 | 文件 | 重点看什么 |
|---|---|---|
| 1 | [go.mod](../../go.mod)、[go.sum](../../go.sum) | 一个仓库一个模块；直接依赖与传递依赖的区别；sum 是校验记录 |
| 2 | [commentcheck/doc.go](../../tools/commentcheck/doc.go)、[main.go](../../tools/commentcheck/main.go) | 先了解职责，再看参数、输出和退出码，入口不混入扫描细节 |
| 3 | [commentcheck/check.go](../../tools/commentcheck/check.go)、[check_test.go](../../tools/commentcheck/check_test.go) | parser 如何得到 AST、如何定位源码行；用负例而不只是正例证明检查有效 |
| 4 | [archcheck/doc.go](../../tools/archcheck/doc.go)、[main.go](../../tools/archcheck/main.go) | 为什么必须从模块根扫描，入口如何把错误交给 CI |
| 5 | [archcheck/check.go](../../tools/archcheck/check.go)、[check_test.go](../../tools/archcheck/check_test.go) | 先看 importProblem/layerProblem，再看路径分析；区分静态检查与安全沙箱 |
| 6 | [config/doc.go](../../internal/platform/config/doc.go)、[config.go](../../internal/platform/config/config.go) | Reader 接收函数作为依赖；缺失与空字符串不是同一种情况 |
| 7 | [config_test.go](../../internal/platform/config/config_test.go) | 表格驱动测试如何把边界排列在一起；假环境如何代替 os.LookupEnv |
| 8 | [logging/doc.go](../../internal/platform/logging/doc.go)、[logging.go](../../internal/platform/logging/logging.go) | 先看 Fields/New，再看 Handler；关联字段如何跨函数传递且不改全局变量 |
| 9 | [logging_test.go](../../internal/platform/logging/logging_test.go) | 用 JSON 解码验证字段，不依赖整行文本顺序；分组不能隐藏请求标识 |
| 10 | [httpx/doc.go](../../internal/platform/httpx/doc.go)、[httpx.go](../../internal/platform/httpx/httpx.go) | 按 RequestIDs → Recovery → BodyLimit → BindJSON/响应阅读，理解中间件先后关系 |
| 11 | [httpx_test.go](../../internal/platform/httpx/httpx_test.go) | httptest 不必监听端口；未知 Content-Length 时也要限制读取 |
| 12 | [auth/doc.go](../../internal/doctor/auth/doc.go)、[auth.go](../../internal/doctor/auth/auth.go) | 错误分类、构造参数、claims 白名单和 bcrypt 的 72 字节限制 |
| 13 | [auth_test.go](../../internal/doctor/auth/auth_test.go) | 一份可解码的 JWT 为什么仍可能必须拒绝；哪些旧测试被保留 |
| 14 | [mysqlx/doc.go](../../internal/platform/mysqlx/doc.go)、[mysql.go](../../internal/platform/mysqlx/mysql.go) | Config 是输入契约；先看 prepare 再看 Open；sql.DB 是池，不是一条连接 |
| 15 | [mysql_test.go](../../internal/platform/mysqlx/mysql_test.go)、[mysql_integration_test.go](../../internal/platform/mysqlx/mysql_integration_test.go) | 不联网的配置验证与必须联网的会话验证如何分开 |
| 16 | [migrate/doc.go](../../internal/platform/migrate/doc.go)、[files.go](../../internal/platform/migrate/files.go) | fs.FS 接口、文件顺序、校验和，以及一文件一语句的约束 |
| 17 | [migrate.go](../../internal/platform/migrate/migrate.go) | Apply 是总流程；沿 acquire → readHistory → verifyHistory → release 阅读；解释 Conn 的所有权 |
| 18 | [files_test.go](../../internal/platform/migrate/files_test.go)、[migrate_integration_test.go](../../internal/platform/migrate/migrate_integration_test.go)、[探针 SQL](../../internal/platform/migrate/testdata/0001_probe.sql) | 内存文件系统和嵌入文件分别测什么；WaitGroup 如何保证工作者退出 |
| 19 | [lifecycle/doc.go](../../internal/platform/lifecycle/doc.go)、[lifecycle.go](../../internal/platform/lifecycle/lifecycle.go)、[lifecycle_test.go](../../internal/platform/lifecycle/lifecycle_test.go) | 取消正常工作与给清理留预算是两个不同的 context 生命周期 |
| 20 | [Compose](../../deploy/docker-compose.yml)、[初始化 SQL](../../deploy/mysql/init/001_test_databases.sql) | 健康检查是否真的操作依赖，端口、内存、数据卷和权限归谁管 |
| 21 | [CI](../../.github/workflows/ci.yml)、[环境示例](../../.env.example)、[Git 忽略规则](../../.gitignore) | 本机步骤如何搬到 Linux；为什么配置不能进入提交 |

每读一个函数，先用自己的话复述“调用方必须给什么、成功得到什么、失败怎么办、资源由谁关闭”。再进入下一层，避免只记语法。

## 4. 关键设计与 WHY

### 一个模块，按领域隔离

`internal` 是 Go 工具链认识的目录名。位于某个父目录下的 internal 包只能被该父目录树内的代码导入；其他仓库不能像导入公共库一样直接导入我们的实现。但同一仓库中的 doctor 仍然可能导入 shop，因此还需要 archcheck 检查项目自己的边界。前者是语言工具链规则，后者是工程规则。

放弃多个 go.mod，是为了让初学阶段可以一次跑完全部测试、共享基础包。代价是依赖仍会出现在同一模块图中，所以文件和包的职责需要额外约束。还没有消费者的接口和空目录不创建。

### 通过构造函数传依赖

Java 中你可能把一个 Environment 对象传给构造器。Go 的 Reader 直接接收 `func(string) (string, bool)`：函数也是值，可以保存并调用。生产传 os.LookupEnv，测试传闭包。闭包能读取创建它时的 test 值，因此测试不用改真实系统环境。

放弃读取全局可变配置和额外配置框架，换来可测试、显式的依赖关系。代价是 app 需要明确调用每项读取函数。读取失败返回 `(零值, error)`，调用方先检查 error，再使用值；这与 Java 的异常控制流不同。

### context、defer 与资源所有权

context 是沿调用链传递的取消信号、截止时间和少量请求附加信息。它不会自动停止任意函数，函数必须把它传给支持 context 的操作，或主动等 `ctx.Done()`。context 也不适合承载所有业务参数。

defer 类似保证在函数离开时执行的清理，按后进先出顺序运行。Apply 先登记归还连接，再登记释放命名锁，因此退出时先解锁、再归还连接。顺序错了，解锁可能拿不到原来的会话。

Notify 返回的 stop 必须被调用。Shutdown 用 `context.WithoutCancel` 保留请求值但去掉旧的取消状态，再添加新的超时。如果直接复用已经取消的工作 context，HTTP 排空可能刚开始就被取消。放弃一个框架包办全部生命周期，代价是后续 app 必须按依赖顺序提供关闭回调，而且回调必须合作响应超时。

### 日志与 HTTP 错误边界

Request ID 是单个请求的检索线索；trace ID 是跨服务调用链标识，两者不是同义词。阶段 0 只承载 context 中已有的标识，不会自动生成真实调用链。日志固定 JSON、UTC 和 service 字段，便于后续统一采集。

不能直接用 Gin 默认恢复日志打印请求转储：panic 字符串或请求内容可能包含密码。我们的恢复只写固定错误分类。客户端收到安全的 `INTERNAL` 和 request_id，后续可据此查日志。代价是开发者要显式记录安全的上下文，不能靠全部打印来排查。

BodyLimit 既检查 Content-Length，也使用 MaxBytesReader 包住实际读取过程。没有 Content-Length 的分块请求也不能无限读。BindJSON 再读一次，要求 EOF，防止请求末尾追加第二个 JSON 值。它只解析与校验格式；领域合法性仍由未来业务层判断。

### sql.DB、sql.Conn 与数据库会话

sql.DB 是并发安全的连接池句柄。对它调用两次 Exec，可能落到不同连接上。sql.Conn 是从池里暂时租到的一条连接；命名锁属于数据库会话，所以 GET_LOCK、迁移、RELEASE_LOCK 必须都通过同一条 Conn。

池参数是预算：MaxOpenConns 限制同时打开的连接，MaxIdleConns 控制能保留多少空闲连接，ConnMaxLifetime 控制连接最长寿命。上限过大可能拖垮数据库，过小可能让请求等池。阶段 0 的 20/10/5m 是后续配置示例，不能称为压测调优结果。

`parseTime=true` 和 `loc=UTC` 决定驱动把数据库时间读成什么 Go 值；MySQL 的会话 `time_zone` 决定数据库如何理解自己的时间计算。只设置前者不能保证数据库会话也是 UTC，所以同时设置两边。

### 迁移为什么不用“大事务包住一切”

MySQL 的很多 DDL 会隐式提交。第一条建表成功、第二步失败，并不会因为外面写了事务就自动恢复原样。我们的契约是一文件一条可重入 SQL，成功后才写 schema_migrations；失败重试必须安全，例如 CREATE TABLE IF NOT EXISTS。

校验和防止已执行文件被悄悄改写，新增变更靠追加文件。放弃按分号切字符串，因为 `'a;b'` 里的分号不是 SQL 边界。代价是文件数量更多，不支持存储过程/多语句脚本这种复杂格式。命名锁用于排队，不是 DDL 原子性的保证。

embed.FS 在编译时打包文件，实现标准库 fs.FS 接口。Go 不需要写 `implements`：类型只要有接口要求的方法，就能被当作该接口使用。因此内存 fstest.MapFS 可以替代 embed.FS 测加载规则，真实集成测试再验证 MySQL 会话行为。

### 自动规则与测试不是同一件事

commentcheck 解析语法树，而非用正则猜函数位置。它证明注释存在，不证明解释正确。archcheck 检查依赖方向和能推导出的路径，不执行任意代码，不可能仅靠这一层证明医生永远读不到答案。

正例证明合法代码能通过；负例证明违规会被拒绝。如果检查工具只测正例，一个永远输出“通过”的空实现也能通过测试。表格驱动测试把边界摆在同一个地方，便于你自己再补用例。

race 发现的是实际执行中存在的并发数据访问问题。没有报告不等于数学证明绝无竞态；没有运行到的路径仍需审查。迁移测试的 channel 容量是 2，因为两个工作者各写一次，主线程等待它们退出后才读；如果容量为 0，工作者等待接收、主线程等待工作者，便会互相等待。

### Compose 的就绪和持久化

容器进程“已启动”不等于数据库“已经能查询”。healthcheck 实际发查询/命令；后续服务可以用 `depends_on: condition: service_healthy` 等待依赖第一次就绪。本阶段三个基础组件彼此独立，因此没有为了示范而增加虚假的 depends_on。

depends_on 不会替运行中的服务自动处理所有断线，也不会把已失败的请求重放。客户端仍需超时与合理的重连策略。Kafka 给容器内外不同的 advertised listener，是因为容器里的 localhost 指向自己，不指向宿主机或 Kafka 容器。

命名卷在容器删除后保留，因此重启不会重新执行 MySQL 首次初始化脚本。不要以为改 `.env` 的密码就能修改已经建好的数据库账户密码。固定版本、内存上限与 localhost 端口映射便于复现，但单节点部署不能提供生产高可用。

## 5. 动手实验

每次只改一个地方，观察结果后撤销**自己本次的编辑**。不改 main、不推送实验破坏、不操作业务库。

### 实验一：让注释检查真的失败

删除 config.go 中 New 前的一行文档注释，运行 `go run ./tools/commentcheck ./...`。应该看到文件、行号和“New 缺少文档注释”，退出非零。撤销删除并重新运行，应该通过。思考：保留一句毫无内容的废话能否通过机器检查？为什么还要人工抽查？

### 实验二：故意跨越领域边界

在 config.go 的 import 组中临时加入 `_ "github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"`。运行 `go run ./tools/archcheck ./...`，应该看到 platform 不得依赖业务包。下划线导入只要求加载包而不使用名称，不能成为架构豁免。撤销这一行，重新检查。不要执行或提交这一改动。

### 实验三：破坏配置边界的期望

打开 config_test.go，把 maximum 用例的输入从 `100` 改为 `101`，但保留“应成功”的期望。运行 `go test ./internal/platform/config -run TestReader -v`，应该失败并指出变量与范围。撤销改动后通过。再读 too_large 用例：同样的 101，为什么这个测试本来就应通过？测试通过指的是“实现符合期望”，不代表输入被接受。

### 实验四：观察已启动与就绪的区别

执行 `docker compose --env-file .env -f deploy/docker-compose.yml stop redis`，再执行 `ps --all`，你应该看到 Redis 不再 healthy，而 MySQL/Kafka 仍运行。执行 `docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait redis` 恢复，再看状态。这里没有业务请求，所以只能说明依赖容器恢复，不能声称业务恢复。

## 6. 常见误区

- “internal 自动隔离所有领域”：它限制仓库外导入，仓库内的方向还要靠项目规则。
- “Go 的 interface 等于 Java 接口声明”：Go 是按方法集合隐式实现；接口通常由使用方定义所需能力。
- “context 取消会强杀 goroutine”：它只是信号，工作函数必须响应；资源所有者仍需等工作者退出。
- “defer 放在哪里都一样”：它延迟到当前函数返回，执行顺序是后进先出；循环内大量 defer 可能延长资源持有时间。
- “sql.OpenDB 已经证明数据库正常”：它创建池句柄，Ping 才会真正探测连接。
- “日志带 UTC 就够了”：数据库会话也要统一，不然 SQL 时间计算仍可能不一致。
- “JWT 只是能解码就合法”：必须校验签名、允许算法、issuer、audience、时间和主体。
- “bcrypt 是可解密的加密”：它是密码摘要，校验靠重新计算/比较，不能还原原密码。
- “测试 SKIP 等于 PASS”：没有运行就是没有证据，报告必须分清。
- “三个容器 healthy 等于项目完成”：这里只完成阶段 0 的基础依赖，后续业务和诊断尚未实现。

## 7. 必答问题

先自己写答案，再展开文末要点。

1. Go 的 internal 目录有什么特殊作用？
2. 为什么配置用环境变量，而不是把配置文件打包进程序？
3. 优雅停机的完整流程是什么？不做时正在处理的请求会怎样？
4. Compose 的 healthcheck 和 depends_on: condition: service_healthy 分别解决什么问题？
5. 为什么用程序检查包依赖，而不只靠人自觉？
6. sql.DB 和 sql.Conn 有什么区别？命名锁为什么必须用同一个 Conn？
7. 为什么 loc=UTC 与 MySQL time_zone 两项都要设置？
8. DDL 为什么不能承诺整批回滚？迁移文件应该如何设计？
9. 请求体大小限制为什么不能只看 Content-Length？
10. 迁移测试的结果 channel 为什么容量为 2？改成 0 后会发生什么？
11. 如何证明单元测试没有碰真实数据库？如何明确开启集成测试？
12. commentcheck、archcheck、race 分别能发现什么、不能证明什么？

## 8. 本阶段新术语

| 术语 | 一句话解释 |
|---|---|
| module / package | module 是依赖与版本管理单位；package 是 Go 代码组织、可见性和编译单位 |
| 导出标识符 | 以大写字母开头，包外代码才可以直接使用 |
| 组合与隐式接口 | 不通过类继承，而是用结构体组合与方法集合表达能力 |
| 闭包 | 能携带创建位置外部变量的函数值 |
| context | 在调用链中传递取消、截止时间和请求关联信息的对象 |
| defer | 在当前函数返回前执行清理，多个 defer 按后进先出执行 |
| goroutine | Go 运行时调度的并发执行单元，启动者要负责其退出路径 |
| channel | 类型明确的发送/接收通道，阻塞规则和缓冲容量影响并发协作 |
| WaitGroup | 等待一组并发工作全部完成的计数同步工具 |
| 错误包装 | 用 `%w` 附加上下文，同时保留 errors.Is/As 的分类判断能力 |
| AST | 源代码的抽象语法树，让检查器按声明结构而非文本猜测工作 |
| 连接池 | 重用数据库连接并限制并发数量的管理器 |
| 命名锁 | MySQL 会话持有的具名互斥机制，用来协调同库迁移 |
| DDL / 元数据锁 | 定义表结构的 SQL / 保护数据库对象定义的锁 |
| 可重入迁移 | 中断后重复执行不会产生错误状态的迁移语句 |
| KRaft | Kafka 自身维护元数据一致性的模式，本项目不再部署 ZooKeeper |
| healthcheck | 容器执行探测命令，判断依赖是否真的可用 |
| race detector | 在测试运行时跟踪并发访问、报告潜在数据竞争的工具 |

<details>
<summary>必答问题参考要点（先自己回答）</summary>

1. internal 只能由其父目录树内代码导入；同仓库领域隔离仍需 archcheck。
2. 同一二进制可部署到不同环境，秘密不进入源码/镜像；代价是必须明确校验环境且不能把值随意打日志。环境变量也不是万能保险，仍要限制谁能读取运行环境。
3. 收到信号 → 停止接新工作并取消工作 context → 等待/排空已有工作 → 按依赖顺序关闭资源 → 超时则按服务实现处理。清理用独立预算；否则请求和事务可能被突然中断。
4. healthcheck 提供“当前是否可用”的探测；service_healthy 让依赖者等第一次就绪，不能保证之后永远不断线。
5. 重复执行、位置明确、进入 CI；减少新增文件时无意越界，但规则本身也要用负例验证。
6. DB 是池，Conn 是租用会话；命名锁不随任意下一次池查询迁移到其他会话。
7. loc 影响驱动解码，time_zone 影响服务器会话；只设一边会留下时间理解不一致。
8. DDL 可隐式提交，不能用普通事务包装假装原子；小步、可重入、已执行只追加不改写。
9. 分块请求可能没有长度，伪造长度也不能当作唯一保障；限制实际 Reader 才能约束真实读取。
10. 两个工作者各发送一次，主线程先 Wait 后接收；无缓冲时两边互相等。容量 2 对应这段固定协议，不是随便选的性能参数。
11. 普通测试没有 TEST DSN，看到明确 Skip；注入独立测试库 DSN 才运行真实测试。HTTP 单元测试直接调用 Handler，认证测试无网络。
12. commentcheck 查注释存在性，archcheck 查静态结构与可识别路径，race 查执行到的并发访问；它们都不能替代语义审查或证明所有未来路径安全。

</details>

学完后由你自己勾选[进度看板](../../进度.md)的学习验收。开发停在这里，进入阶段 1 需要你明确同意。
