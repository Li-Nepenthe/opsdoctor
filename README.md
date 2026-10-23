# OpsDoctor

用 Go 构建可复现的微服务故障靶场，再逐步实现带证据报告的故障诊断 Agent。

当前是**阶段 2：订单服务、缓存与消息**。已有商品缓存、订单 HTTP 接口、库存 gRPC、Kafka 销量消费、压测控制和服务治理。按用户要求提供待核对幂等请求的计数，其余指标、调用链和医生能力留到后续阶段。最新验收状态以[进度看板](进度.md)为准。

- 唯一需求来源：[项目计划书](项目计划书.md)
- 开发规则：[AGENTS.md](AGENTS.md)
- 设计记录：[decisions](docs/decisions.md)
- 目录与文件总入口：[代码地图](docs/代码地图.md)
- 实测记录：[notes](docs/notes.md)
- 阶段 0：[汇报书](docs/reports/phase-00-foundation.md)、[学习材料](docs/learn/phase-00-foundation.md)
- 阶段 1：[汇报书](docs/reports/phase-01-inventory.md)、[学习材料](docs/learn/phase-01-inventory.md)

## 启动阶段 2 环境

需要 Go **1.27.1**、已启动的 Docker Desktop（Linux containers）。Windows 运行 `-race` 还需要 gcc 在 PATH 中、`CGO_ENABLED=1`。新装工具后重新打开终端。

所有命令都从仓库根目录运行。仅第一次复制示例文件，不覆盖你已经配置的 `.env`。

PowerShell：

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
$env:LOADGEN_QPS = '0' # 先暂停流量，给空库造数或逐步学习留出时间。
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

bash：

```bash
test -f .env || cp .env.example .env
export LOADGEN_QPS=0 # 先暂停流量，后面通过控制接口开始。
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

应该看到 `inventory-service`、`order-service`、`order-worker`、`loadgen`、`shop-mysql`、`redis`、`kafka` 全部 `healthy`。显式指定 `--env-file .env`，避免 Compose 从 `deploy/` 找错配置。首次构建需要下载固定版本 Go/Alpine 镜像和模块。

端口仅映射到 `127.0.0.1`：3306、6379、9092、订单 HTTP 8081、库存 gRPC 9001、三个服务管理端口 6061/6062/6063、压测控制 6070。管理端口的 `/health` 应返回 `ok`。服务自动迁移五张 shop 表，创建或检查三分区 `order.created` 主题，但不自动造数。容器内 Kafka 地址是 `kafka:29092`，宿主机是 `127.0.0.1:9092`。

Redis 使用 `noeviction`、AOF 每次刷盘及持久卷，避免幂等记录被缓存淘汰或普通重启丢失。内存不足时新写入会失败，带幂等键的订单返回 503。不要删除数据卷来“解决”内存不足；这会丢掉仍有效的去重记录。

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

## 运行真实 MySQL 与 Redis 集成测试

Compose 首次初始化创建六个独立测试库：`opsdoctor_mysql_test`、`opsdoctor_migrate_test`、`opsdoctor_inventory_test`、`opsdoctor_seed_test`、`opsdoctor_order_test`、`opsdoctor_sales_test`。测试只清理各自白名单库。Redis 商品测试固定用 DB 14，幂等测试固定用 DB 15，只删除本次创建的键。改 `.env` 中密码时同步更新 DSN；已有数据卷不会自动改密码或重新执行初始化脚本。

从阶段 1 升级时，保留数据卷，手动补建阶段 2 测试库（密码只由容器环境传递）：

```powershell
Get-Content deploy/mysql/init/003_order_sales_test_databases.sql -Raw | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'
```

bash 用 `cat deploy/mysql/init/003_order_sales_test_databases.sql | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'`。更早的阶段还需先补 `002_shop_test_databases.sql`。参照 `.env.example` 给已有 `.env` 增加订单、销量 TEST DSN 和 `TEST_REDIS_ADDR`，保留你的本机密码。

PowerShell：

```powershell
Get-Content .env | Where-Object { $_ -match '^TEST_(((MIGRATE|INVENTORY|SEED|ORDER|SALES)_)?MYSQL_DSN|REDIS_ADDR)=' } | ForEach-Object {
    $entry = $_ -split '=', 2
    [Environment]::SetEnvironmentVariable($entry[0], $entry[1], 'Process')
}
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed ./internal/shop/order ./internal/shop/sales ./internal/shop/product -run Integration -count=1 -timeout=90s -v
Remove-Item Env:TEST_MYSQL_DSN, Env:TEST_MIGRATE_MYSQL_DSN, Env:TEST_INVENTORY_MYSQL_DSN, Env:TEST_SEED_MYSQL_DSN, Env:TEST_ORDER_MYSQL_DSN, Env:TEST_SALES_MYSQL_DSN, Env:TEST_REDIS_ADDR -ErrorAction SilentlyContinue
```

bash：

```bash
while IFS= read -r line; do
  case "$line" in
    TEST_MYSQL_DSN=*|TEST_MIGRATE_MYSQL_DSN=*|TEST_INVENTORY_MYSQL_DSN=*|TEST_SEED_MYSQL_DSN=*|TEST_ORDER_MYSQL_DSN=*|TEST_SALES_MYSQL_DSN=*|TEST_REDIS_ADDR=*) export "$line" ;;
  esac
done < .env
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/inventoryclient ./internal/shop/seed ./internal/shop/order ./internal/shop/sales ./internal/shop/product -run Integration -count=1 -timeout=90s -v
unset TEST_MYSQL_DSN TEST_MIGRATE_MYSQL_DSN TEST_INVENTORY_MYSQL_DSN TEST_SEED_MYSQL_DSN TEST_ORDER_MYSQL_DSN TEST_SALES_MYSQL_DSN TEST_REDIS_ADDR
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

阶段 2 已经产生新订单后，五表不再是最初的造数基线，seed 会拒绝继续。不要用它来重置业务或补库存。后续核对使用 `shopctl check`，保留已有订单和去重记录。

本机全量实测总耗时 75.651 秒，重跑 0.538 秒；这是开发机的结果，不是性能承诺。原始记录见 [notes](docs/notes.md#phase-01)。CLI 的 `--timeout 800ms` 控制整个 RPC 等待，`Deduct` 没有应用层重试。

宿主机调试服务：先停止 Compose 库存容器，加载上述 MYSQL_DSN，设置 `$env:ADMIN_ADDR=':6062'`，执行 `go run ./cmd/inventory-service`（bash 用 `ADMIN_ADDR=:6062 go run ./cmd/inventory-service`）。旧阶段 0 `.env` 的 `ADMIN_ADDR=:6061` 要改成库存对应的 `:6062`。结束时 Ctrl+C，再启动 Compose 库存容器，避免端口冲突。

## 调用订单接口与观察幂等

PowerShell（bash 可用 curl 发送同样的路径、JSON 和请求头）：

```powershell
Invoke-RestMethod http://127.0.0.1:8081/products/1 | ConvertTo-Json -Depth 5
$key = 'learn-' + [guid]::NewGuid().ToString('N')
$headers = @{ 'Idempotency-Key' = $key }
$body = '{"user_id":1001,"sku_id":1,"quantity":1}'
$first = Invoke-RestMethod http://127.0.0.1:8081/orders -Method Post -Headers $headers -ContentType 'application/json' -Body $body
$second = Invoke-RestMethod http://127.0.0.1:8081/orders -Method Post -Headers $headers -ContentType 'application/json' -Body $body
$first.data
$second.data
Invoke-RestMethod ("http://127.0.0.1:8081/orders/" + $first.data.order_id)
Invoke-RestMethod 'http://127.0.0.1:8081/orders?user_id=1001&page=1&page_size=20'
```

两次下单都返回 201，同一个订单编号与金额。处理中返回 409 `REQUEST_IN_PROGRESS`；结果不确定返回 409 `REQUEST_OUTCOME_UNKNOWN`，这时应核对日志和订单，不能换一个键重做。幂等键可选，但不带键就没有请求去重保证。同一键改变用户、商品或数量返回 400。

待核对日志包含 `order_id`、`sku_id`、`idempotency_key`。`order_id=0` 表示尚未得到订单号。`http://127.0.0.1:6061/metrics` 提供本阶段明确要求的 `idempotency_requests_total{result="needs_review",service="order-service"}`；计数随进程重启归零，不替代日志记录。

## 控制压测与核对销量

全量造数完成后，开始 200 QPS：

```powershell
Invoke-RestMethod http://127.0.0.1:6070/control -Method Post -ContentType 'application/json' -Body '{"qps":200,"hot_sku_id":0}'
docker compose --env-file .env -f deploy/docker-compose.yml logs -f loadgen
```

每十秒能看到真实 `qps`、`error_rate`、`p50_ms`、`p99_ms` 和 `dropped`。百分位使用一毫秒桶的上界估计。`error_rate` 是 0..1 的比例，重试的失败尝试也计入；`dropped` 是工作者都忙时没有发出的请求。不要只看目标 QPS。

暂停与核对：

```powershell
Invoke-RestMethod http://127.0.0.1:6070/control -Method Post -ContentType 'application/json' -Body '{"qps":0}'
Invoke-RestMethod http://127.0.0.1:6070/control | ConvertTo-Json -Depth 5
# MYSQL_DSN 按上一节加载；等消费者追平后再核对。
go run ./cmd/shopctl check
```

应看到 `gap=0`。非零时命令退出失败，并提示消费延迟、消息发送失败等可能原因。它不会改数据；总量一致也不能单独证明每个商品都正确。

单独做五分钟基准时，先暂停 Compose 中的 loadgen，再启动宿主机进程，使用 6071 避免与容器的控制端口冲突：

```text
go run ./cmd/loadgen --qps 200 --duration 5m --control 127.0.0.1:6071
```

本阶段在 Compose 内部网络完成了无少发的基准，显式使用 256 个工作者。下面这条命令使用已有容器中的二进制，6071 是容器内部的临时控制端口。先确认主 loadgen 的 QPS 为 0，并等命令输出 `scope=total` 后再开始其他故障实验：

```text
docker compose --env-file .env -f deploy/docker-compose.yml exec -T loadgen /app/server --qps 200 --duration 5m --workers 256 --control :6071
```

结束时输出 `scope=total` 的真实汇总。small 数据使用 `--products 100`，并降低速率或时长，避免把有限库存耗尽。`POST /control` 可单独修改 `qps` 或 `hot_sku_id`；`hot_sku_id=0` 恢复 Zipf 分布。默认 Zipf 参数为 s=1.1、v=100，不是均匀随机。

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

- 下单仍有跨服务一致性缺口：补偿可能失败；订单已写入但发消息失败会少统计销量。本阶段不实现 Outbox。
- 扣库存超时不一定代表没有扣成功。根治需要 Deduct 自身携带请求 ID 去重，本阶段不改这个 RPC 契约。
- 结果待核对须人工检查。Redis 暂时不能写状态时，原占位超过完整处理预算后会转成待核对；幂等窗口过去或数据卷丢失后不再保证去重。
- 认证尚未接线；日志的 trace/span 字段只是关联信息载体，OpenTelemetry 在阶段 3 接入。
- 迁移是一文件一条 SQL，要求可重入；DDL 隐式提交，不保证整批迁移原子回滚。
- `archcheck` 检查可静态识别的导入、路径和 embed；它不是运行时文件访问沙箱。
- Compose 是单节点、本机实验环境，不提供生产级高可用；Redis 幂等保存依赖本机持久卷，内存满时拒绝新写入。

本阶段的[汇报书](docs/reports/phase-02-order.md)和[学习材料](docs/learn/phase-02-order.md)记录交付、实际失败与恢复过程。结束后停止开发，等待你学习并决定何时进入阶段 3。
