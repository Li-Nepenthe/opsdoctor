# OpsDoctor

用 Go 构建可复现的微服务故障靶场，再逐步实现带证据报告的故障诊断 Agent。

当前是**阶段 0：地基与仓库整理**。已经有共享基础包、独立认证包、注释/架构检查工具和 MySQL、Redis、Kafka 环境；尚无库存、订单、医生服务或大模型调用。最新验收状态以[进度看板](进度.md)为准。

- 唯一需求来源：[项目计划书](项目计划书.md)
- 开发规则：[AGENTS.md](AGENTS.md)
- 设计记录：[decisions](docs/decisions.md)
- 实测记录：[notes](docs/notes.md)
- 阶段 0：[汇报书](docs/reports/phase-00-foundation.md)、[学习材料](docs/learn/phase-00-foundation.md)

## 启动阶段 0 环境

需要 Go **1.27.1**、已启动的 Docker Desktop（Linux containers）。Windows 运行 `-race` 还需要 gcc 在 PATH 中、`CGO_ENABLED=1`。新装工具后重新打开终端。

所有命令都从仓库根目录运行。仅第一次复制示例文件，不覆盖你已经配置的 `.env`。

PowerShell：

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

bash：

```bash
test -f .env || cp .env.example .env
docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

应该看到 `shop-mysql`、`redis`、`kafka` 全部 `healthy`。显式指定 `--env-file .env`，避免 Compose 从 `deploy/` 找错配置。也可以先把 MySQL 两个密码变量注入当前 shell，再执行计划书中的 `docker compose -f deploy/docker-compose.yml up -d`。

三个端口仅映射到 `127.0.0.1`：3306、6379、9092。容器内连接 Kafka 用 `kafka:29092`，宿主机用 `127.0.0.1:9092`。此时不会创建业务表或 Kafka topic。

停止环境，保留数据卷（两种 shell 相同）：

```text
docker compose --env-file .env -f deploy/docker-compose.yml down
```

`.env.example` 只含演示假值；`.env` 和 `.obsidian/` 被 Git 忽略。不要把实际密码、API Key 或 Token 写进代码、日志或提交。

## 检查代码

以下命令在 PowerShell 和 bash 中相同；静态检查工具只需安装一次：

```text
go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
gofmt -l internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s
```

`gofmt -l` 应无输出；若有文件，执行 `gofmt -w internal tools` 后重新检查。若找不到 staticcheck，把 `go env GOPATH` 返回目录下的 `bin` 加到 PATH。没有配置测试 DSN 时，MySQL 集成测试明确跳过，其余测试不访问网络、不产生费用。

生成阅读清单：

```text
go run ./tools/commentcheck -list ./...
```

## 运行真实 MySQL 集成测试

Compose 首次初始化会创建 `opsdoctor_mysql_test` 和 `opsdoctor_migrate_test` 两个独立测试库。**迁移测试会清理专用库内的探针表和迁移记录**，代码拒绝其他库名。改 `.env` 中密码时也要同步更新 DSN；已有数据卷不会因为修改环境变量而自动改数据库密码。

PowerShell：

```powershell
Get-Content .env | Where-Object { $_ -match '^TEST_(MIGRATE_)?MYSQL_DSN=' } | ForEach-Object {
    $entry = $_ -split '=', 2
    [Environment]::SetEnvironmentVariable($entry[0], $entry[1], 'Process')
}
go test -race ./internal/platform/mysqlx ./internal/platform/migrate -run Integration -count=1 -timeout=90s -v
Remove-Item Env:TEST_MYSQL_DSN, Env:TEST_MIGRATE_MYSQL_DSN
```

bash：

```bash
while IFS= read -r line; do
  case "$line" in
    TEST_MYSQL_DSN=*|TEST_MIGRATE_MYSQL_DSN=*) export "$line" ;;
  esac
done < .env
go test -race ./internal/platform/mysqlx ./internal/platform/migrate -run Integration -count=1 -timeout=90s -v
unset TEST_MYSQL_DSN TEST_MIGRATE_MYSQL_DSN
```

这里解析赋值而不执行 `.env` 的内容；Go 程序本身不自动读取该文件。不要把未经核对的配置文件当脚本运行。

## 当前边界

- 没有业务进程，所以根目录没有 `.go` 文件，也没有为后续阶段预建空 `cmd/` 入口。
- 认证尚未接线；日志的 trace/span 字段只是关联信息载体，OpenTelemetry 在阶段 3 接入。
- 迁移是一文件一条 SQL，要求可重入；DDL 隐式提交，不保证整批迁移原子回滚。
- `archcheck` 检查可静态识别的导入、路径和 embed；它不是运行时文件访问沙箱。
- Compose 是单节点、本机实验环境，不提供生产级高可用；Redis 不持久化缓存。

本阶段结束后停止开发，等待你学习并决定何时进入阶段 1。
