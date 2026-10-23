# OpsDoctor

用 Go 构建可复现的微服务故障靶场，再逐步实现带证据报告的故障诊断 Agent。

当前是**阶段 1：库存服务（gRPC + MySQL）**。已有库存查询、原子扣减、归还、shop 表迁移和批量造数命令；订单服务、消息消费、指标、调用链和医生系统尚未实现。最新验收状态以[进度看板](进度.md)为准。

- 唯一需求来源：[项目计划书](项目计划书.md)
- 开发规则：[AGENTS.md](AGENTS.md)
- 设计记录：[decisions](docs/decisions.md)
- 目录与文件总入口：[代码地图](docs/代码地图.md)
- 实测记录：[notes](docs/notes.md)
- 阶段 0：[汇报书](docs/reports/phase-00-foundation.md)、[学习材料](docs/learn/phase-00-foundation.md)
- 阶段 1：[汇报书](docs/reports/phase-01-inventory.md)、[学习材料](docs/learn/phase-01-inventory.md)

## 启动阶段 1 环境

需要 Go **1.27.1**、已启动的 Docker Desktop（Linux containers）。Windows 运行 `-race` 还需要 gcc 在 PATH 中、`CGO_ENABLED=1`。新装工具后重新打开终端。

所有命令都从仓库根目录运行。仅第一次复制示例文件，不覆盖你已经配置的 `.env`。

PowerShell：

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

bash：

```bash
test -f .env || cp .env.example .env
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

应该看到 `inventory-service`、`shop-mysql`、`redis`、`kafka` 全部 `healthy`。显式指定 `--env-file .env`，避免 Compose 从 `deploy/` 找错配置。首次构建需要下载固定版本 Go/Alpine 镜像和模块。

端口仅映射到 `127.0.0.1`：3306、6379、9092、库存 gRPC 9001、库存管理 6062。访问 `http://127.0.0.1:6062/health` 应返回 `ok`。服务启动自动迁移五张 shop 表，但不生成数据、不创建 Kafka topic。容器内 Kafka 地址是 `kafka:29092`，宿主机是 `127.0.0.1:9092`。

停止环境，保留数据卷（两种 shell 相同）：

```text
docker compose --env-file .env -f deploy/docker-compose.yml down
```

`.env.example` 只含演示假值；`.env` 和 `.obsidian/` 被 Git 忽略。不要把实际密码、API Key 或 Token 写进代码、日志或提交。

## 检查代码

日常运行服务和造数的命令见后面的“造数与调用库存”；本节用于修改代码后的检查。

以下命令在 PowerShell 和 bash 中相同；静态检查工具只需安装一次：

```text
go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
gofmt -l cmd gen internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s
```

`gofmt -l` 应无输出；若有手写文件，执行 `gofmt -w cmd internal tools` 后重新检查。若找不到 staticcheck，把 `go env GOPATH` 返回目录下的 `bin` 加到 PATH。没有配置测试 DSN 时，MySQL 集成测试明确跳过，其余测试使用内存连接，不访问外部服务、不产生费用。

生成阅读清单：

```text
go run ./tools/commentcheck -list ./...
go run ./tools/commentcheck -map ./...
```

`-list` 给出类型与函数；`-map` 给出文件说明和相对于 docs 的链接，用于更新代码地图。普通 Go 文件顶部必须有与 package 空一行的 `// 本文件：` 说明；doc.go 列出包内文件，cmd 的薄入口用包说明。

## 运行真实 MySQL 集成测试

Compose 首次初始化创建四个独立测试库：`opsdoctor_mysql_test`、`opsdoctor_migrate_test`、`opsdoctor_inventory_test`、`opsdoctor_seed_test`。迁移/库存/造数测试会清理各自库内的数据，代码校验专用库名。改 `.env` 中密码时同步更新 DSN；已有数据卷不会自动改密码或重新执行初始化脚本。

从阶段 0 升级时，保留数据卷，手动补建新增测试库（密码只由容器环境传递）：

```powershell
Get-Content deploy/mysql/init/002_shop_test_databases.sql -Raw | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'
```

bash 用 `cat deploy/mysql/init/002_shop_test_databases.sql | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'`。再参照 `.env.example` 给已有 `.env` 补上两个 TEST DSN，保留你的本机密码。

PowerShell：

```powershell
Get-Content .env | Where-Object { $_ -match '^TEST_((MIGRATE|INVENTORY|SEED)_)?MYSQL_DSN=' } | ForEach-Object {
    $entry = $_ -split '=', 2
    [Environment]::SetEnvironmentVariable($entry[0], $entry[1], 'Process')
}
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed -run Integration -count=1 -timeout=90s -v
Remove-Item Env:TEST_MYSQL_DSN, Env:TEST_MIGRATE_MYSQL_DSN, Env:TEST_INVENTORY_MYSQL_DSN, Env:TEST_SEED_MYSQL_DSN
```

bash：

```bash
while IFS= read -r line; do
  case "$line" in
    TEST_MYSQL_DSN=*|TEST_MIGRATE_MYSQL_DSN=*|TEST_INVENTORY_MYSQL_DSN=*|TEST_SEED_MYSQL_DSN=*) export "$line" ;;
  esac
done < .env
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed -run Integration -count=1 -timeout=90s -v
unset TEST_MYSQL_DSN TEST_MIGRATE_MYSQL_DSN TEST_INVENTORY_MYSQL_DSN TEST_SEED_MYSQL_DSN
```

这里解析赋值而不执行 `.env` 的内容；Go 程序本身不自动读取该文件。不要把未经核对的配置文件当脚本运行。

## 造数与调用库存

Go 程序不会自动加载 `.env`；只把本命令需要的变量读进进程，不把文件当脚本执行。

PowerShell：

```powershell
Get-Content .env | ForEach-Object {
    if ($_ -match '^(MYSQL_DSN|INVENTORY_GRPC_TARGET)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
go run ./cmd/shopctl seed
go run ./cmd/shopctl stock get --sku 1
go run ./cmd/shopctl stock deduct --sku 1 --quantity 1
go run ./cmd/shopctl stock get --sku 1
```

bash 的环境加载（其后四条 Go 命令与上面相同）：

```bash
while IFS= read -r line; do
  case "$line" in
    MYSQL_DSN=*|INVENTORY_GRPC_TARGET=*) export "$line" ;;
  esac
done < .env
```

首次 `seed` 生成 50 万商品、50 万库存、200 万历史订单，并写好对应销量/已处理记录，输出真实计数和耗时。新数据的库存从 100 变为 99。重复造数显示 `skipped=true`，不会重置你扣过的库存。`seed --small` 目标为 100 商品、400 订单；完整数据已存在时也跳过，不会缩小数据库。每批事务原子提交，中断后可从完整前缀续跑；五表计数不匹配则拒绝继续。造数期间不要接收业务写入或并发运行多个 seed。

本机全量实测总耗时 75.651 秒，重跑 0.538 秒；这是开发机的结果，不是性能承诺。原始记录见 [notes](docs/notes.md#phase-01)。CLI 的 `--timeout 800ms` 控制整个 RPC 等待，`Deduct` 没有应用层重试。

宿主机调试服务：先停止 Compose 库存容器，加载上述 MYSQL_DSN，设置 `$env:ADMIN_ADDR=':6062'`，执行 `go run ./cmd/inventory-service`（bash 用 `ADMIN_ADDR=:6062 go run ./cmd/inventory-service`）。旧阶段 0 `.env` 的 `ADMIN_ADDR=:6061` 要改成库存对应的 `:6062`。结束时 Ctrl+C，再启动 Compose 库存容器，避免端口冲突。

## 重新生成 protobuf

生成物已提交，普通构建和 CI 不需要装 buf。修改契约时才运行：

```text
go install github.com/bufbuild/buf/cmd/buf@v1.75.0
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
buf lint
buf generate
```

把 `go env GOPATH` 下的 `bin` 加入 PATH。源文件在 `api/proto`，生成物在 `gen`，禁止手改 `.pb.go`。字段编号是协议身份，不能随意重排或重用。

## 当前边界

- 只有库存业务进程和 shopctl；没有阶段 2 的订单服务、缓存或消费者。
- 认证尚未接线；日志的 trace/span 字段只是关联信息载体，OpenTelemetry 在阶段 3 接入。
- 迁移是一文件一条 SQL，要求可重入；DDL 隐式提交，不保证整批迁移原子回滚。
- `archcheck` 检查可静态识别的导入、路径和 embed；它不是运行时文件访问沙箱。
- Compose 是单节点、本机实验环境，不提供生产级高可用；Redis 不持久化缓存。

本阶段结束后停止开发，等待你学习并决定何时进入阶段 2。
