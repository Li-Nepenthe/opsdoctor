# 实测记录

只记录实际执行结果；没有执行的检查明确标为待验证。时间按用户时区 America/Los_Angeles。

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
