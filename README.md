# OpsDoctor

用 Go 构建可复现的微服务故障靶场，再逐步实现带证据报告的故障诊断 Agent。

当前正在进行**阶段 4：故障注入器与场景库**。订单、库存、销量消费已接入指标、调用链和关联日志。Grafana 看板、消费积压告警与临时接收器用于观察真实靶场，医生系统尚未开发。最新验收状态以[进度看板](进度.md)为准。

- 唯一需求来源：[项目计划书](项目计划书.md)
- 开发规则：[AGENTS.md](AGENTS.md)
- 设计记录：[decisions](docs/decisions.md)
- 目录与文件总入口：[代码地图](docs/代码地图.md)
- 实测记录：[notes](docs/notes.md)；对外引用数字：[数据台账](docs/数据台账.md)
- 阶段 0：[汇报书](docs/reports/phase-00-foundation.md)、[学习材料](docs/learn/phase-00-foundation.md)
- 阶段 1：[汇报书](docs/reports/phase-01-inventory.md)、[学习材料](docs/learn/phase-01-inventory.md)
- 阶段 2：[汇报书](docs/reports/phase-02-order.md)、[学习材料](docs/learn/phase-02-order.md)
- 阶段 3：[汇报书](docs/reports/phase-03-observability.md)、[学习材料](docs/learn/phase-03-observability.md)

## 启动阶段 3 环境

需要 Go **1.27.1**、已启动的 Docker Desktop（Linux containers）。Windows 运行 `-race` 还需要 gcc 在 PATH 中、`CGO_ENABLED=1`。新装工具后重新打开终端。

所有命令都从仓库根目录运行。仅第一次复制示例文件，不覆盖你已经配置的 `.env`。已有配置需参照 `.env.example` 补上 `GRAFANA_ADMIN_PASSWORD`、`MYSQL_EXPORTER_PASSWORD`、至少 32 字符的 `ALERT_WEBHOOK_TOKEN`；三者都使用你自己的本机值。Grafana 的环境密码只用于首次创建管理员，已有数据卷不会随环境变量自动改密码。

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

应该看到 17 个容器运行，有健康探测的容器显示 `healthy`。部分官方精简镜像没有探测命令，`--wait` 对这类容器只能确认进程运行；还要按下方命令确认 Loki 就绪、采集目标和数据源可用。显式指定 `--env-file .env`，避免 Compose 从 `deploy/` 找错配置。首次构建需要下载固定版本 Go/Alpine 镜像和模块。

端口仅映射到 `127.0.0.1`：3306、6379、9092、订单 HTTP 8081、库存 gRPC 9001、三个服务管理端口 6061/6062/6063、压测控制 6070。管理端口的 `/health` 应返回 `ok`。服务自动迁移五张 shop 表，创建或检查三分区 `order.created` 主题，但不自动造数。容器内 Kafka 地址是 `kafka:29092`，宿主机是 `127.0.0.1:9092`。

Redis 使用 `noeviction`、AOF 每次刷盘及持久卷，避免幂等记录被缓存淘汰或普通重启丢失。内存不足时新写入会失败，带幂等键的订单返回 503。不要删除数据卷来“解决”内存不足；这会丢掉仍有效的去重记录。

停止环境，保留数据卷（两种 shell 相同）：

```text
docker compose --env-file .env -f deploy/docker-compose.yml down
```

`.env.example` 只含演示假值；`.env` 和 `.obsidian/` 被 Git 忽略。不要把实际密码、API Key 或 Token 写进代码、日志或提交。

## 查看指标、调用链、日志与告警

已有阶段 2 数据卷不会重跑 MySQL 初始化脚本。第一次升级保留卷并补建监控账号：

```text
docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql bash /docker-entrypoint-initdb.d/004_metrics_user.sh
```

脚本从容器环境取密码，账号只能读监控需要的数据。它不会修改已经存在的账号密码。Prometheus 的 `mysql_up` 应为 1，只有 exporter 的 `up=1` 还不能证明它能登录 MySQL。

| 入口 | 用途 |
|---|---|
| http://127.0.0.1:3000 | Grafana，用户名 admin，密码来自本机环境；打开 shop 总览 |
| http://127.0.0.1:9090/targets | Prometheus，应有八个采集目标，全部 UP |
| http://127.0.0.1:16686 | Jaeger，查看一笔请求经过的服务与操作 |
| http://127.0.0.1:9093 | Alertmanager，查看触发与恢复的告警 |
| http://127.0.0.1:3100/ready | Loki，返回 ready 才能开始日志验收 |
| http://127.0.0.1:6061/metrics | 订单服务指标；库存、消费者分别使用 6062、6063 |
| http://127.0.0.1:6061/debug/pprof/goroutine?debug=1 | 订单进程当前 goroutine；只在管理端口开放 |

Grafana Explore 选择 Loki，用 `{service="order-service"} | json` 查看日志。拿到某条日志的 trace_id 后，用 `{service=~"order-service|inventory-service|order-worker"} | json | trace_id="填入实际值"` 查整条请求；点击 TraceID 可跳转调用链。Jaeger 按用户批准固定 2.20.0，兼容证据及恢复最新版的条件见 [decisions](docs/decisions.md#jaeger-compatibility)。默认根请求采样 10%，没有被采样的请求仍有日志，但 Jaeger 里没有保存它的 span。启动日志没有所属请求，因此不会有 trace ID。

采集和导出都需要一点时间。开始流量后，给看板的两分钟速率窗口留出时间。`http_server_request_duration_seconds_count` 是 HTTP 请求数，P99 由桶计算，只是近似值。熔断状态用 0/1/2 表示关闭/半开/打开；待核对计数已经并入统一注册表。

告警只报告“延迟高、错误多、积压、goroutine 增长、采集目标不可达”等症状。`alertsink` 的日志应出现 `ConsumerLagHigh`，不能把 `ServiceUnavailable` 当成消费积压验收通过。

```text
docker compose --env-file .env -f deploy/docker-compose.yml logs --since 5m alertsink
docker stats --no-stream
```

Prometheus 保留七天且最多 1 GB 数据，Loki 保留 48 小时；Jaeger 从阶段 4 起使用内存保存最近 30000 条 trace，重启会清空。所有发布端口仅绑定 `127.0.0.1`。Alloy 独占日志采集所需的 Docker socket；挂载 `:ro` 不会把 Docker API 变成只读权限，不能把这个挂载交给后续医生程序。

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

阶段 4 加入显式的补库存命令。长时间压测会卖空热门商品，之后的行锁实验就可能看不到足够的更新。**只在实验前暂停流量后使用**，例如为 1–10000 号商品各补 10 万件：

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:6070/control -ContentType application/json -Body '{"qps":0}'
go run ./cmd/shopctl stock restock --sku 1 --quantity 100000 --count 10000 --timeout 3s
```

bash：

```bash
curl -fsS -H 'Content-Type: application/json' -d '{"qps":0}' http://127.0.0.1:6070/control
go run ./cmd/shopctl stock restock --sku 1 --quantity 100000 --count 10000 --timeout 3s
```

命令通过正式 gRPC `Release` 增加库存，成功后打印已确认的范围。省略 `--count` 就只改一个商品，最多一次处理 10000 个。若中途失败，命令指出已确认数量和待核对的商品编号。先用 `stock get` 查这个商品，再决定如何继续；不要直接重跑整批，因为增加库存不是幂等操作。


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

待核对日志包含 `order_id`、`sku_id`、`idempotency_key`。`order_id=0` 表示尚未得到订单号。`http://127.0.0.1:6061/metrics` 的统一注册表继续提供 `idempotency_requests_total{result="needs_review",service="order-service"}`；计数随进程重启归零，不替代日志记录。

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

阶段 2 在 Compose 内部网络完成了无少发的基准，显式使用 256 个工作者。下面这条命令使用已有容器中的二进制，6071 是容器内部的临时控制端口。先确认主 loadgen 的 QPS 为 0，并等命令输出 `scope=total` 后再开始其他故障实验：

```text
docker compose --env-file .env -f deploy/docker-compose.yml exec -T loadgen /app/server --qps 200 --duration 5m --workers 256 --control :6071
```

结束时输出 `scope=total` 的真实汇总。small 数据使用 `--products 100`，并降低速率或时长，避免把有限库存耗尽。`POST /control` 可单独修改 `qps` 或 `hot_sku_id`；`hot_sku_id=0` 恢复 Zipf 分布。默认 Zipf 参数为 s=1.1、v=100，不是均匀随机。

## 阶段 4：准备与运行故障实验

本阶段命令在宿主机运行，场景范围是 S01–S10、S13。完整自测仍在验收中，实际结论以进度看板和 notes 为准。先停止其他手工实验，准备完整业务数据和足够库存。

已有数据卷不会自动重跑 MySQL 初始化脚本。先给本机 `.env` 增加独立的 `CHAOS_MYSQL_PASSWORD`，不要覆盖已有配置；再启动更新后的 Compose，显式补建账号：

```text
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql bash /docker-entrypoint-initdb.d/005_chaos_user.sh
```

空库首次启动仍先设 `LOADGEN_QPS=0`，造数后才开启流量。Toxiproxy 管理端口为 127.0.0.1:8474；订单→库存、库存→MySQL、订单→Redis 默认经过代理。宿主机 shopctl 仍用原库存端口。

准备 S09 的另一个镜像。该命令只构建，不会立即切换正在运行的服务：

```text
docker build -f deploy/docker/go.Dockerfile --build-arg CMD_NAME=order-service --build-arg APP_VERSION=20261024.4 --build-arg GO_TAGS=leakbug -t opsdoctor/order-service:20261024.4 .
```

实验账号不使用业务账号。PowerShell 从本机配置读取密码并组装 DSN，整个过程不打印它：

```powershell
Get-Content .env | ForEach-Object {
    if ($_ -match '^CHAOS_MYSQL_PASSWORD=(.*)$') {
        $env:CHAOS_MYSQL_PASSWORD = $Matches[1]
    }
}
$env:CHAOS_MYSQL_DSN = "chaos:$($env:CHAOS_MYSQL_PASSWORD)@tcp(127.0.0.1:3306)/shop?parseTime=true"
```

bash 对应写法：

```bash
while IFS= read -r line; do
  case "$line" in CHAOS_MYSQL_PASSWORD=*) export "$line" ;; esac
done < .env
export CHAOS_MYSQL_DSN="chaos:${CHAOS_MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/shop?parseTime=true"
```

先用 loadgen 控制接口开启真实流量，再依次执行：

```text
go run ./cmd/chaos list
go run ./cmd/chaos healthy
go run ./cmd/chaos inject S07
go run ./cmd/chaos verify S07
go run ./cmd/chaos restore S07
go run ./cmd/chaos healthy
```

`inject` 可用 `--qps`、`--sku`、`--latency-ms` 覆盖该场景已有的对应动作。`verify` 从注入开始计三分钟，需要同时看到症状和相关告警；它不会因为配置已改就宣布成功。`healthy` 需要连续一分钟有流量、延迟与错误率正常、消费追平、依赖可用且没有 firing 告警。

完整自测用 `go run ./cmd/chaos selftest`。先暂停流量并按前面的 restock 命令显式准备库存，再恢复流量、等 healthy 通过。每一轮的完整结果表都保留，包括失败轮次。工具不会在自测内部自动补库存。

实验状态保存在忽略文件 `.opsdoctor-chaos.json`。同一时刻只允许一个活动实验。中断后运行 `go run ./cmd/chaos restore-all`，它按原镜像内容 ID、公开配置和流量恢复；恢复失败时保留文件，修复外部环境后可重试。这个文件含参考答案，不要挂载进业务或医生容器。恢复环境不会自动处理过去已经产生的待核对订单。

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
- 认证尚未接线；监控入口仅用于本机实验，不能直接暴露到公网。
- span 批量导出有五秒预算，监控后端不可用时可能丢记录；业务成功不保证每次都能查到完整 trace。
- 迁移是一文件一条 SQL，要求可重入；DDL 隐式提交，不保证整批迁移原子回滚。
- `archcheck` 检查可静态识别的导入、路径和 embed；它不是运行时文件访问沙箱。
- Compose 是单节点、本机实验环境，不提供生产级高可用；Redis 幂等保存依赖本机持久卷，内存满时拒绝新写入。

阶段 2 的[汇报书](docs/reports/phase-02-order.md)和[学习材料](docs/learn/phase-02-order.md)保留原有验收记录；阶段 3 的完成状态见顶部进度链接。


## 阶段 3：验证 gRPC 空闲恢复

库存和 OTLP 客户端在代码中跳过 DNS TXT 服务配置读取，普通主机名解析仍保留。`INVENTORY_GRPC_IDLE_TIMEOUT` 默认 30m，实验时可临时缩短为 10s；它与 800ms 的 RPC 预算不同。先暂停 loadgen，再运行：

```powershell
$env:INVENTORY_GRPC_IDLE_TIMEOUT = '10s'
docker compose --env-file .env -f deploy/docker-compose.yml up -d --no-deps --wait order-service
Invoke-RestMethod http://127.0.0.1:8081/products/1
Start-Sleep -Seconds 25
Invoke-RestMethod http://127.0.0.1:8081/products/1
Remove-Item Env:INVENTORY_GRPC_IDLE_TIMEOUT
docker compose --env-file .env -f deploy/docker-compose.yml up -d --no-deps --wait order-service
```

两次商品查询都应返回 `stock.status=ok`。最后重建订单容器，恢复 `.env` 中的配置，示例为 30m。使用已有商品，不重新造数；更完整的前后实验与限制见阶段 3 学习材料。MySQL 上限调整为 1536 MiB，保留 256 MiB buffer pool 与最多 100 个连接，给其他数据库内存分配留出余量。
