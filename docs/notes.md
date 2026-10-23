# 实测记录

<a id="phase-02"></a>

## 2026-10-23：阶段 2 开工记录

完整读完 1839 行计划书后，获取远端，确认工作区干净。按用户授权将 phase-01-inventory 快进合入 main 并推送，再从 main 创建 phase-02-order；基线 `3970868dae0e298c3cb0b1269a6800a473e2e7a7`。

环境实查：Go 1.27.1 windows/amd64，CGO_ENABLED=1；inventory-service、MySQL、Redis、Kafka 都 healthy。新库版本查询结果已写入 decisions。开工时发现“失败删除幂等占位”需要区分订单是否已经提交，已发出问题，先推进不依赖这个决定的缓存和服务治理。

只记录实际执行结果；没有执行的检查明确标为待验证。时间按用户时区 America/Los_Angeles。

阶段 2 的第一轮真实依赖测试：新增独立订单库 `opsdoctor_order_test`、销量库 `opsdoctor_sales_test`，Redis 商品测试用 DB 14，幂等测试用 DB 15。`go test -race` 的 9 个 Integration 测试全部通过：Redis 20 次并发占位只有一个执行者；同一订单消息并发处理 20 次后，销量为 3、去重记录为 1；销量溢出失败后去重记录回滚，随后能正确重放；订单详情和分页、空值缓存及 TTL、旧幂等所有者保护也通过。

开发中检查记录：首次运行 Go 测试因沙箱不能写机器上的编译缓存而失败，获得工具批准后正常运行。随后注释检查两次指出长函数缺少内部讲解，分别补充了 HTTP 错误映射顺序与取消后补偿的测试说明，重新检查通过。没有把这些失败记录成业务测试通过。

第一次完整 Compose 构建启动成功，七个容器均 healthy，loadgen 以 QPS=0 暂停。首次真实商品请求却降级，首次下单在约 804ms 后得到 `REQUEST_OUTCOME_UNKNOWN`。同键重复请求仍是待核对，没有重做。日志原因是 `deduct_outcome_unknown`，指标为 1。检查测试用户 880000002 没有订单，商品 499999 库存仍为 100。订单容器内 DNS、TCP 9001 和管理健康检查都正常，宿主机只读 RPC 也成功；稍后商品请求正常返回库存。

据此修正首次连接准备：`grpc.NewClient` 默认尚未建连，新增 `connectInventory` 在启动预算内等待 Ready，再开放 HTTP；增加内存握手与不可达超时测试。该次待核对 key 保留，没有删除或换键重做。修复后的冷启动与正式压测结果在后续记录。

修复后重建订单服务，第一次商品查询返回 stock.status=ok。新的独立测试用户 880000003 对商品 499998 购买两件，同键两次响应均为 201，订单编号同为 2000001，金额同为 3998 分。详情和列表返回该订单，数据库确认用户只有一笔订单、共两件。`shopctl check` 输出 `ordered_quantity=2000002 sold_quantity=2000002 gap=0`。

截至接线完成，`go test -race ./... -count=1 -timeout=180s` 全仓库通过；后续新增首次连接测试也已通过。`go vet ./...` 与 staticcheck 已通过。静态检查曾指出两处英文开头错误文本的 ST1005，改为中文开头后通过；这是实际遇到的格式问题，不是业务故障。

第一轮五分钟压测（目标 200 QPS，64 工作者）实际结果：300.0940009 秒，59119 次 HTTP 尝试，0 错误，880 次未发出，实际 197.00160557258243 QPS，P50=8ms、P99=453ms（一毫秒桶上界）。商品/下单/列表请求分别为 41405/11813/5901。12:44:56 UTC 的十秒窗口实际 116.466 QPS、P50=359ms、P99=2201ms，未发出 831 次；下个窗口又少发 49 次。不能把目标速率写成实际达到的速率。后续使用已有 `--workers 256` 配置重跑，保留两轮原始结果；第二轮已有部分缓存，不能把延迟差异全归因于工作者数量。

第二轮（宿主机，256 工作者）：300.0869976 秒，59782 次 HTTP 尝试，168 错误，302 次未发出，199.21556241395777 QPS，错误率 0.281021%，P50=14ms、P99=1229ms。商品/下单/列表为 41926/11992/5864。错误与少发集中在 12:50:10 UTC 的十秒窗口。同期订单日志出现缓存写入失败和商品查询耗尽三秒预算；MySQL 的 cgroup memory.events 读数为 max=5136、oom=0、oom_kill=0。它说明触及过限额，不能单凭这个累计值断言停顿根因。随后在 Compose 内部网络重测，排除 Windows 端口转发对生成路径的影响。

### 五分钟内部网络压测与消费者恢复（06:06–06:13）

命令：`docker compose --env-file .env -f deploy/docker-compose.yml exec -T loadgen /app/server --qps 200 --duration 5m --workers 256 --control :6071`。Compose 主 loadgen 保持 QPS=0，所以没有叠加第二股流量。全量数据保留，没有重置或补库存。

结果：300.011038541 秒、59998 次 HTTP 尝试、0 错误、0 dropped、实际 199.98597482205832 QPS、P50=5ms、P99=51ms，毫秒桶无溢出。三类请求 42194 / 11935 / 5869。有限时长最后不足一个调度周期及排空时间会使实际均值略低于目标 200，原始完成数没有补齐。

这轮最后约半分钟与消费者强制终止实验重叠，因此它不是全程无故障基准。原本准备另开十秒流量时发现内部 :6071 仍被五分钟进程占用，短进程因端口冲突退出，没有发出额外流量。之后直接使用现有流量完成积压验证。该操作顺序失误如实保留。

`kill -s SIGKILL order-worker` 后，三个分区 LAG 分别为 274、282、282。稍后 SQL 得到订单件数 2035307、销量 2034435；两次读取时流量仍在继续，因此 LAG 总数与 SQL 差值不要求相同。启动消费者后，分区位点均追平到 11930、11882、11837，LAG 全为 0。暂停并结束流量后，真实 `shopctl check` 输出 `ordered_quantity=2035650 sold_quantity=2035650 gap=0`。总量检查与真实 MySQL 重复事件测试共同证明本次没有多计数，不把一个总量相等推广成所有订单永远正确。

### 库存停服、恢复与优雅退出（06:14–06:16）

停止 inventory-service 后连续 15 次商品请求均返回 200、stock.status=unavailable。15 个独立测试下单中，前 10 次得到 409 REQUEST_OUTCOME_UNKNOWN，后 5 次在 Deduct 熔断后得到 503 UPSTREAM_UNAVAILABLE；后五次耗时分别为 5.432、5.276、6.723、5.636、5.337ms。同一个待核对键重发仍是 409，2.742ms，没有重新扣减。

日志含 order_id=0、sku_id=499996、idempotency_key 和 deduct_outcome_unknown 原因。`idempotency_requests_total{result="needs_review",service="order-service"}` 为 10。GetStock 与 Deduct 各自记录 closed→open；恢复库存后，各自记录 open→half-open→closed。商品重新返回 available=100。新的独立恢复验证订单返回 201，编号 2035650、金额 1999 分。

三个业务进程收到正常停止信号后都记录退出过程：订单“开始排空订单请求”与“订单入口已关闭，准备关闭下游连接”，drain_failed=false；消费者“等待当前订单消息完成”与“消费循环与管理入口已结束”；库存“开始排空库存请求”与“库存入口已关闭”。随后恢复靶场并暂停压测，不删除任何业务卷。

### 最终源码与说明检查

人工审查发现商品 handler 把数据库查询超时归成 500，已改为契约的 504 UPSTREAM_TIMEOUT，新增七个表格场景检查。库存展示失败仍是降级 200。这是验收后对错误分类的修正，没有改压测调度或成功请求路径。把客户端与调度的固定预算改成具名常量，并补充取值理由。

用去掉注释与 token.Pos 的 Go AST 加编译指令比较：main 中 71 个 Go 文件保持相同逻辑。仅排除本阶段新增 check 分派的 shopctl/app.go；新的阶段 2 文件不在这个旧代码比较里。三十五个正式读取的环境变量均能在 .env.example 和计划书找到。

最终检查顺序为 gofmt、go mod tidy、go vet、staticcheck、commentcheck、archcheck，再运行全仓库 race。全部退出 0，commentcheck=132 个非生成 Go 文件，archcheck 通过。无依赖轮为 67 个顶层测试通过、16 个真实集成测试跳过；注入六个专用 MySQL DSN 与 Redis 测试地址后为 83 个顶层测试通过、0 跳过、23 个有测试的包通过。新增 40 个顶层测试，地图含 134 个 Go 文件（含 2 个生成文件）。

学习材料九段 Go 最小示例补上包、导入及缺省上下文后编译通过。七段 PowerShell 代码块语法通过，CI YAML 和两个内嵌 Python 脚本语法通过。没有执行文档中的破坏实验来代替语法检查；实际故障测试另有前面的记录。

Redis 停服与恢复补验：原键重放订单 2035650，停止时响应 503，保留卷重启后仍为同一订单、同一金额 1999 分。最后 `shopctl check` 输出 `ordered_quantity=2035651 sold_quantity=2035651 gap=0`。压测保持暂停。

最终非 Markdown 文件 156 个，按路径排序、CRLF 归一化为 LF 后逐文件 SHA-256，再对“路径 + TAB + 文件摘要 + LF”的拼接结果计算 SHA-256：`c689a00cc74ca1a88494255b3c6fc2bd673715fbfb7c52a9c3118dec0ce3a89b`。文档追加 CI 结果不改变这个源码指纹。

提交前再检查：七个容器全部 healthy，loadgen 生效配置 qps=0、hot_sku_id=0。真实凭据扫描通过，.env 被 Git 忽略；相对文档链接有效，全部学习验收仍未勾选。链接脚本初次误把代码里的泛型签名与 Markdown 语法示例当作路径，排除代码块/行内代码后通过；没有因此修改合法示例。

阶段 1 记录见本文末尾；阶段 0 的结果保留当时版本，不用新版本的计数覆盖历史。

## 2026-10-23：开工检查

- 代码基线：main `3ebd194`，工作区干净，远端 `https://github.com/Li-Nepenthe/opsdoctor.git`。
- `go version`：`go version go1.27.1 windows/amd64`。
- `go env CGO_ENABLED CC`：`0`、`gcc`；未找到 gcc/clang 可执行文件。
- `docker version`、`docker compose version`：命令不存在；常见 Docker Desktop 安装位置也不存在。
- 原仓库 `git status --short --branch`：`## main`，无文件修改。
- 用户同意继续开发，本机 race 暂由固定版本 Linux CI 验证；Docker 由用户准备。

## 2026-10-23：环境补齐后的实测

记录汇总时间：01:50，America/Los_Angeles。检查时仍是 `phase-00-foundation` 的未提交工作树；下方源码指纹对应这一轮最终本地验证。对应源码提交为 de4459208791899131d5f67cbc8d42310b3948d7，远端记录如下。

- 刷新进程 PATH 后，`docker version --format '{{.Client.Version}} / {{.Server.Version}}'` 返回 `29.9.2 / 29.9.2`。
- `docker compose version` 返回 `v5.6.1`。
- `gcc --version` 返回 WinLibs MinGW-W64 GCC `16.2.0`；`go env CGO_ENABLED` 返回 `1`。
- `go list -m -json ...@latest`：Gin v1.12.0、MySQL driver v1.10.1、JWT v5.3.1、x/crypto v0.58.0、staticcheck v0.8.2。
- `gh api repos/actions/checkout/releases/latest --jq .tag_name`：v7.0.1；setup-go 同命令：v7.0.0。

### 失败记录与修复

1. `go test ./...` 初次运行：archcheck 的 embed 路径样例失败，因为未去掉 `//go:embed` 前缀。修复为逐个解析 embed pattern，随后用例通过。
2. commentcheck 初次自查发现 11 个长函数缺内部说明；补注释并拆分扫描职责。一次拆分后新函数又缺少内部说明，检查再次拒绝；补充后全部通过。
3. `docker compose -f deploy/docker-compose.yml up -d --wait ...` 未找到根目录 `.env`，报缺少 MySQL 密码变量；未创建容器。改为显式 `--env-file .env`。
4. 首次 Kafka 启动失败：`AccessDeniedException: /tmp/kraft-combined-logs/__cluster_metadata-0`。只读检查官方镜像发现 `/var/lib/kafka/data` 已归 appuser 所有；改用该目录，并修正本次新建空数据卷的属主。没有删除任何数据卷。

### 最终本地检查

按以下顺序执行；所有命令退出码均为 0，负例实验除外：

```text
gofmt -l internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s
go run ./tools/commentcheck -list ./...
```

- gofmt、vet、staticcheck：无诊断。
- commentcheck：`通过（32 个非生成 Go 文件）`。
- archcheck：`通过`。
- race：9 个包全部通过。源码共有 23 个顶层 Test 函数，另含表格子用例；没有把子用例数当成顶层测试数。
- 这一轮显式从 `.env` 注入 `TEST_MYSQL_DSN` 和 `TEST_MIGRATE_MYSQL_DSN`，两个集成测试均实际执行：UTC 会话、并发迁移只记一次、历史改写拒绝、已取消 context 后的锁释放与重获。
- 未注入测试 DSN 的独立运行也通过；集成测试会明确输出开启方法并跳过。HTTP 用 httptest 直接调用 Handler，不监听端口。
- 无大模型调用、无压测，无 QPS、P99、准确率或费用数据。

### 删除函数注释的独立验收

临时删除 `internal/platform/config/config.go` 中 `New` 的文档注释，执行 `go run ./tools/commentcheck ./...`：

```text
internal\platform\config\config.go:18:1: 函数或方法 New 缺少文档注释
commentcheck：发现 1 项问题
exit status 1
```

使用 `finally` 恢复原始字节，再运行检查：`commentcheck：通过（32 个非生成 Go 文件）`。这次实验没有留下工作区破坏。

### Compose 健康验收

```text
docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

| 服务 | 实际拉取并运行的镜像 | 状态 | 映射端口 |
|---|---|---|---|
| shop-mysql | mysql:8.4.13 | healthy | 127.0.0.1:3306 |
| redis | redis:8.10.2 | healthy | 127.0.0.1:6379 |
| kafka | apache/kafka:4.4.0 | healthy | 127.0.0.1:9092 |

进一步只把 `.env` 中的 `MYSQL_ROOT_PASSWORD`、`MYSQL_PASSWORD` 注入当前 PowerShell 进程，再执行计划书的原样命令 `docker compose -f deploy/docker-compose.yml up -d`：三个容器均返回 Running。没有把密码打印到终端或记录文件。

### CI

[GitHub Actions 运行 39929219914](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39929219914) 的 quality 任务结论为 success，源码提交 de4459208791899131d5f67cbc8d42310b3948d7。ubuntu-24.04 上格式、模块一致性、vet、staticcheck、两个检查器、无外部依赖的 race、全新 Compose 健康检查、MySQL 集成 race 和清理步骤均成功。首次本机 Kafka 卷权限修正没有成为 CI 的人工前置条件，全新卷也实际启动成功。

### 最终本地验证的源码指纹

非 Markdown、非 .env 的已交付源码/配置文件，按相对路径排序，逐文件计算 SHA-256，再对路径与摘要的 LF 连接文本计算 SHA-256。值为：
a0e9a2b4db7821d8ac24099bbe210148b3c16e79fa33a068539b45da71cfde85

这包含 Go、SQL、Compose、CI、模块与示例配置，不包含后续补记结果的文档。后续提交只要这些文件不变，就对应同一份本地测试源码。

## 2026-10-23：注释可读性返工复验

验证对象：`phase-00-foundation` 在合并提交 `62126cff67130b70e70e4681466af54ea670f9dc` 之上的待提交返工。该合并引入 main 的 `6f92d0e1663acce2596e2194649188bc9a8f1b8a`，没有冲突。阶段 0 原有工程验收保留，学习验收仍未勾选。

变更范围：Go 中文注释、测试表格的注释与排版、学习材料、报告和本记录；`.env.example` 只删除 `CHAOS_ADDR` 与 `CHAOS_TOKEN`。没有修改 Go 的逻辑、测试输入或断言、SQL、依赖、Compose 和 CI 配置。

### 无行为改动的核对

临时核对程序使用 `git ls-files -z -- *.go` 枚举文件，分别读取 `git show d9b24f6:<路径>` 和工作树源码。用 `parser.ParseFile`（不加载注释，启用 `SkipObjectResolution`）解析，用 `ast.Fprint` 排除 `token.Pos` 字段后比较；另比较 `//go:`、`//line`、`// +build` 指令。32 个文件全部一致。临时程序在系统临时目录运行后删除，没有加入仓库。

最初尝试直接比较词法 token，因表格换成多行后增加合法的尾逗号而报告差异；随后改用上述 AST 对比，确认是排版差异，没有修改实现。这个区别保留在记录里，避免把工具的误判说成代码故障。

### 本机全部检查

```text
gofmt -w internal tools
gofmt -l internal tools
go vet ./...
C:\Users\Nepenthe\go\bin\staticcheck.exe ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
```

- gofmt 列表为空，vet/staticcheck 无诊断，退出码均为 0。staticcheck 不在当前 PATH，使用既有安装的绝对路径，没有更换版本。
- commentcheck：通过（32 个非生成 Go 文件）；archcheck：通过。
- race：9 个包全部通过。从被忽略的 `.env` 只读取两个 TEST DSN 并注入当前进程，没有输出密码；JSON 测试事件中 `TestOpenIntegration`、`TestApplyIntegration` 均为 pass，无 skip。
- `docker compose --env-file .env -f deploy/docker-compose.yml ps --format json`：shop-mysql、redis、kafka 的 Health 均为 healthy，State 均为 running。本轮未重建或清空数据卷。
- 学习材料第 5 节：把六段 Go 示例抽取到系统临时目录；前五个用 `go run` 执行，输出分别为 `11`、三个 `true`、`true true` 与 `true`、退出通知中文文本、`false` 与 `true 0`。第六段用 `go test <临时文件>` 编译通过，输出 `[no test files]`；未连接数据库执行丢弃连接实验。
- 示例验证脚本最初使用 Python 默认 cp1252 输出，打印第 4 个例子的中文时失败；改用 `python -X utf8` 后完成剩余验证。Go 示例本身没有运行错误。

### 本轮源码与配置指纹

取 `git ls-files` 中所有非 Markdown 文件（不包含被忽略的 `.env`），按相对路径排序；逐文件将 CRLF 统一为 LF 后计算 SHA-256，再把每行 `相对路径 + 制表符 + 文件摘要 + LF` 连接起来求 SHA-256。用于标识上述已验证工作树，值为：

`d10bb5b5c85a89729749d78a5850a68317fb29dc5511216035f23ab656af6bc8`

前文的首次验收指纹和 CI 记录属于原始交付，作为历史保留；本轮以这里的验证对象和指纹为准。分支推送后的运行可在 [phase-00-foundation 的 Actions 记录](https://github.com/Li-Nepenthe/opsdoctor/actions?query=branch%3Aphase-00-foundation) 查看。

<a id="phase-01"></a>

## 2026-10-23：阶段 1 的开发与验收

代码版本：`phase-01-inventory` 工作树，基线 main `3aa6c99`（包含阶段 0 的 `4404d78`）。最终源码指纹与远端 CI 记录补在本节末尾。下列耗时来自本机 Windows 10 / Go 1.27.1 / MySQL 8.4.13；不是通用性能承诺。

### 实际开发顺序与发现的问题

1. 用户批准阶段 0 合入 main 后，执行 fast-forward 合并和推送，再创建阶段 1 分支。先读完整计划书和当前进度，没有勾选学习验收。
2. 先写 proto、迁移、领域类型与条件更新仓储，再接 gRPC、健康检查、进程退出流程，随后实现 seed 和 CLI，补单元与真实 MySQL 测试。
3. buf lint/generate 通过；生成物保存在 gen。安装的是固定版本 Go 工具，没有安装系统软件。构建镜像 manifest 与实际拉取均成功。
4. 原 commentcheck 要求每个包有 doc.go，与 cmd 薄入口只有 main.go 的规范冲突；先补 cmd/main.go 包说明例外及测试。后续 main 的新规范也明确了这个例外。
5. 检查退出流程时把 Serve 错误逐个过滤后再 errors.Join，避免一组错误里包含正常关闭哨兵时把另一个真实故障一起忽略。这是代码审查发现的问题，没有编造线上故障。
6. 写 Docker 配置并启动四个容器；运行 full seed、重复执行、RPC 手工扣减，再停止库存服务完成删索引/恢复索引实验。
7. 用户发来三层规范后，暂存本阶段未提交文件，合入 main `3aa6c99`，恢复工作树，无冲突；先升级 commentcheck，再改已有说明和目录导航。
8. 新增 cmd/README.md 后，archcheck 报“cmd/<程序>/ 只能包含 main.go”。因阶段 0 的逻辑范围受到限制，暂停此项并询问；用户批准精确放行根 cmd/README.md，要求同步计划书和决定记录。修复后正反例及全量检查通过。

### 数据初始化原始结果

先按 README 从 .env 注入 MYSQL_DSN；日志不输出 DSN。

```text
go run ./cmd/shopctl seed
products=500000 stock=500000 orders=2000000 inserted_products=500000 skipped=false seed_elapsed=1m15.631s total_elapsed=1m15.651s

go run ./cmd/shopctl seed
products=500000 stock=500000 orders=2000000 inserted_products=0 skipped=true seed_elapsed=499ms total_elapsed=538ms
```

SQL COUNT 实查：products=500000，stock=500000，orders=2000000，sales 行数=500000，SUM(sold)=2000000，processed_events=2000000。耗时区分 seed 内部计时和 CLI 连接、迁移在内的总计时。

RPC 实查：`stock get --sku 1` 得到 100；`stock deduct --sku 1 --quantity 3` 成功；再次 get 得到 97。实验后在停服期间只把这次修改的 SKU 1 恢复为 100；没有重置其他库存或删除数据卷。

### EXPLAIN 原始观察

库存服务先停止，避免索引实验影响 RPC。MySQL CLI 会话隔离级别为 REPEATABLE-READ、时区 SYSTEM；这是 CLI 的会话，应用连接仍由 mysqlx 设置为 +00:00，集成测试另外验证。

```sql
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
ALTER TABLE stock DROP INDEX uk_stock_sku;
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
ALTER TABLE stock ADD UNIQUE INDEX uk_stock_sku(sku_id);
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
```

| 状态 | id | select_type | table | partitions | type | possible_keys | key | key_len | ref | rows | filtered | Extra |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 删除前 | 1 | SIMPLE | stock | NULL | const | uk_stock_sku | uk_stock_sku | 8 | const | 1 | 100.00 | NULL |
| 删除后 | 1 | SIMPLE | stock | NULL | ALL | NULL | NULL | NULL | NULL | 498780 | 3.33 | Using where |
| 恢复后 | 1 | SIMPLE | stock | NULL | const | uk_stock_sku | uk_stock_sku | 8 | const | 1 | 100.00 | NULL |

`rows` 是优化器估算，不是实际 COUNT。finally 路径恢复索引，SHOW INDEX 确认 PRIMARY(id) 和唯一的 uk_stock_sku(sku_id) 均存在，再启动库存容器并确认 healthy。

停服日志实际出现“收到退出信号，开始排空库存请求”和“库存入口已关闭，准备关闭数据库连接池”。普通请求日志包含 request_id、RPC 方法、状态码与耗时。

### 测试与静态检查

从 .env 白名单注入四个 TEST DSN，各自指向专用测试库。执行：

```text
gofmt -w internal tools
gofmt -l cmd gen internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
go run ./tools/commentcheck -list ./...
go run ./tools/commentcheck -map ./...
```

最终本地结果：全部退出 0；格式、vet、staticcheck 无诊断；commentcheck 通过（70 个非生成 Go 文件）；archcheck 通过。JSON 输出统计 43 个顶层测试、15 个通过的包，没有 skipped 测试。72 个 Go 文件进入地图，其中 2 个是生成物。

```text
100 次并发扣减：成功=50，不足=50，最终库存=0
100ms deadline：客户端=DeadlineExceeded，服务端=context deadline exceeded，总等待=100.5768ms（原查询 5s）
small：products=100 orders=400；重跑 skipped=true，已修改库存保持 7
```

seed 回滚测试制造订单主键冲突，确认本批先写的商品也回滚；已取消 context 不写入。deadline 测试验证 Go handler 及时返回、驱动停止等待，不据此宣称 MySQL 远端查询瞬间终止或写入必定回滚。

三层规范升级前的同一业务实现也跑过完整 race：40 个顶层测试、15 个包，100ms deadline 实测 100.9908ms，服务端感知 context canceled。新增 3 个文件说明/清单/地图测试后才是上面的 43 个；两轮计数不混用。

注释范围核验：用 go/parser 去掉注释和 token.Pos 后比较 main 与工作树 AST，并单独比较 //go:、//line、旧式 build 指令。26 个阶段 0 文件完全一致。仅排除用户要求扩展的 commentcheck 原有 4 个文件、以及用户额外批准的 archcheck/check.go 和 check_test.go；其余阶段 0 逻辑未变。

### 最终收尾核验（04:05–04:06，America/Los_Angeles）

人工复读最后几条注释后，再按相同顺序跑完整检查：gofmt/vet/staticcheck/commentcheck/archcheck/AST 比较及全量 race 都通过。43 个顶层测试、15 个有测试的包通过，无测试函数跳过；另 5 个包为 no test files（正常构建，不是集成测试被跳过）。本轮 deadline 输出为 100.5017ms、客户端 DeadlineExceeded、服务端 context canceled，两种服务端取消原因都符合协议取消传播。

重新构建当前业务源码的 Compose 成功，`ps --format '{{.Service}} {{.Health}}'` 得到 inventory-service、kafka、redis、shop-mysql 全部 healthy；`Invoke-RestMethod http://127.0.0.1:6062/health` 得到 ok。文档 PowerShell 代码块只做语法解析，没有执行其中的破坏实验；7 段均无语法错误。文件地图和相对文档链接检查通过，忽略的 .env 未纳入 Git，未发现本机真实密码或私钥/令牌模式进入待提交文件。

最终非 Markdown 源文件 93 个，归一化 CRLF 为 LF 后，按路径排序计算每文件 SHA-256，再对 `路径 + TAB + 文件哈希 + LF` 拼接结果计算 SHA-256：`dcda6bee0d7937567b82434b74028c844cca48fc3ff7068c8f7297b3938bff7d`。这个指纹包含代码、测试、协议、部署和 CI，文档后补 CI 链接不会改变它。

### Linux CI 39943928805

[运行链接](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39943928805)，源码提交 `54bd24f024001b57a5b4faa9d974c619957c24d8`。2026-10-23 04:07:53 创建，04:11:29 完成（America/Los_Angeles），结论 success。ubuntu-24.04 / Go 1.27.1 的格式、tidy、vet、staticcheck、两种规则、无外部依赖 race、全新 Compose、真实 MySQL race 与 CLI 冒烟全部通过。

日志摘录：

```text
100 次并发扣减：成功=50，不足=50，最终库存=0
100ms deadline：客户端=DeadlineExceeded，服务端=context deadline exceeded，总等待=102.604738ms（原查询 5s）
small：products=100 orders=400；重跑 skipped=true，已修改库存保持 7
products=100 stock=100 orders=400 inserted_products=100 skipped=false seed_elapsed=16ms total_elapsed=20ms
products=100 stock=100 orders=400 inserted_products=0 skipped=true seed_elapsed=3ms total_elapsed=6ms
sku_id=1 available=100
sku_id=1 deducted=1
sku_id=1 available=99
```

CI 只跑 small，不把它的 20ms 当作全量造数时间。完整规模的 75.651 秒来自前面的本机记录。收尾另对学习材料 10 段 Go 示例在临时模块中补上必要包/导入进行编译：全部通过；t.Context/Cleanup 示例测试也通过。随后只修改文档记录与文件行数清单，不改业务源码或检查工具。
