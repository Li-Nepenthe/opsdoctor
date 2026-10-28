# OpsDoctor

> **每次重建前，先检查压测参数。** 阶段 4–5 的故障复验使用 `LOADGEN_PRODUCT_COUNT=10000`、`LOADGEN_QPS=200`、`LOADGEN_WORKERS=64`。把三项明确写在本机 `.env`，不能只在旧进程里临时设置。首次空库造数前仍先设 QPS 为 0，准备好后再恢复 200。代码默认商品范围是 50 万，不会自动采用这套实验基线。
>
> **已知限制（阶段 5 实测）**：整套重建后使用默认 50 万商品、200 QPS、64 个工作者，正常流量的回源比例约为 44%，超过 `CacheLoadsHigh` 的 35% 门槛，持续误报；当时请求错误为 0。查询涉及的商品范围变大后，正常未命中率本身就很高，单一比例门槛无法区分它与 S05。本阶段按用户决定保留规则和默认值，复验限定在 1 万商品基线；告警的适用范围问题留待单独修复。数据见 P5-8。


用 Go 构建可复现的微服务故障靶场，再逐步实现带证据报告的故障诊断 Agent。

当前已完成**阶段 6：Agent 循环与实时页面**的工程收尾，等待 Claude 验收和用户学习。医生收到告警后会自动查证据并保存报告；页面展示每一步。默认使用免费 Mock 脚本，真实模型必须显式启用。最新验收状态以[进度看板](进度.md)为准。

- 唯一需求来源：[项目计划书](项目计划书.md)
- 开发规则：[AGENTS.md](AGENTS.md)
- 设计记录：[decisions](docs/decisions.md)
- 目录与文件总入口：[代码地图](docs/代码地图.md)
- 实测记录：[notes](docs/notes.md)；对外引用数字：[数据台账](docs/数据台账.md)
- 阶段 0：[汇报书](docs/reports/phase-00-foundation.md)、[学习材料](docs/learn/phase-00-foundation.md)
- 阶段 1：[汇报书](docs/reports/phase-01-inventory.md)、[学习材料](docs/learn/phase-01-inventory.md)
- 阶段 2：[汇报书](docs/reports/phase-02-order.md)、[学习材料](docs/learn/phase-02-order.md)
- 阶段 3：[汇报书](docs/reports/phase-03-observability.md)、[学习材料](docs/learn/phase-03-observability.md)
- 阶段 4：[汇报书](docs/reports/phase-04-chaos.md)、[学习材料](docs/learn/phase-04-chaos.md)
- 阶段 5：[汇报书](docs/reports/phase-05-doctor-tools.md)、[学习材料](docs/learn/phase-05-doctor-tools.md)
- 阶段 6：[汇报书](docs/reports/phase-06-agent.md)、[学习材料](docs/learn/phase-06-agent.md)

## 启动靶场环境

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

告警只报告“延迟高、错误多、积压、goroutine 增长、采集目标不可达”等症状。Prometheus 的告警页应出现 `ConsumerLagHigh`，医生任务列表会留下对应的 pending 任务，不能把 `ServiceUnavailable` 当成消费积压验收通过。

```text
Invoke-RestMethod http://127.0.0.1:9090/api/v1/alerts
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

独立的库名能防止测试清理业务数据，但这些库仍共用同一个 MySQL 进程。竞态测试会争用 CPU、连接和磁盘，还会有意制造锁等待。因此它可能触发 **InventoryLatencyHigh、OrderLatencyHigh、RowLockWaiting**。这不是正常业务的健康基线，不能混入故障实验的数据。

跑测试前，先结束并恢复正在做的故障实验，记下 loadgen 当前的 QPS，再把它设为 0，等消费追平。随后暂停 Alertmanager 和医生（医生要释放 diag_ro 的空闲连接，留给三连接上限测试）：`docker compose --env-file .env -f deploy/docker-compose.yml stop alertmanager doctor`。Prometheus 继续采集，便于检查测试结束后是否恢复正常；暂停的是通知。

跑完后，不论测试是否通过，都要先执行 `docker compose --env-file .env -f deploy/docker-compose.yml start doctor`，再恢复原来的流量。医生的采集端点恢复后，等 `chaos healthy` 通过，最后执行 `docker compose --env-file .env -f deploy/docker-compose.yml start alertmanager`。如果仍有异常，先排查，不要为了恢复通知而忽略告警。若没有暂停通知，测试告警会留下 pending 任务；阶段 6 调度器还会自动执行它们。因此测试开始前必须确认 `.env` 和正在运行的 `/api/v1/status` 都是 **Mock**，改完环境变量后要重新创建 doctor 容器，光修改文件不会影响旧进程。过时告警会被取消，未过时的会跑固定脚本。恢复告警不会删除记录，之后暂停通知也不会撤回已经送达的通知。

`.env` 必须包含以下 **11 个**变量（业务 8 个、医生 3 个）。不要只把名字复制过去；六个 DSN 要使用本机的测试账号和对应测试库，不能指向业务库。下面的命令会先检查缺项，并且把跳过的测试视为失败。

| 变量 | 用途 |
|---|---|
| `TEST_MYSQL_DSN` | 连接池测试，库名为 `opsdoctor_mysql_test` |
| `TEST_MIGRATE_MYSQL_DSN` | 迁移测试，库名为 `opsdoctor_migrate_test` |
| `TEST_INVENTORY_MYSQL_DSN` | 库存测试，库名为 `opsdoctor_inventory_test` |
| `TEST_SEED_MYSQL_DSN` | 造数测试，库名为 `opsdoctor_seed_test` |
| `TEST_ORDER_MYSQL_DSN` | 订单测试，库名为 `opsdoctor_order_test` |
| `TEST_SALES_MYSQL_DSN` | 消费去重与销量测试，库名为 `opsdoctor_sales_test` |
| `TEST_REDIS_ADDR` | 本机 Redis 地址，商品/幂等测试分别使用 DB 14/15 |
| `TEST_REDIS_PASSWORD` | Redis 业务账号密码，与本机 `REDIS_PASSWORD` 相同；缺少时测试会明确跳过 |
| `TEST_SHOP_MYSQL_RO_DSN` | 宿主机连接 shop 的 diag_ro；验证权限和三连接上限，绝不清理业务表 |
| `TEST_SHOP_REDIS_RO_URL` | 宿主机连接 Redis DB 0 的 diag_ro；验证查看成功、读取 key 和写命令被拒绝 |
| `TEST_DOCTOR_MYSQL_DSN` | 独立 doctor-mysql 上的 `opsdoctor_doctor_test` 库，使用 doctor 账号 |

从阶段 1 升级时，保留数据卷，手动补建阶段 2 测试库（密码只由容器环境传递）：

```powershell
Get-Content deploy/mysql/init/003_order_sales_test_databases.sql -Raw | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'
```

bash 用 `cat deploy/mysql/init/003_order_sales_test_databases.sql | docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql sh -c 'export MYSQL_PWD=$MYSQL_ROOT_PASSWORD; exec mysql -uroot'`。更早的阶段还需先补 `002_shop_test_databases.sql`。参照 `.env.example` 给已有 `.env` 增加订单、销量 TEST DSN 、`TEST_REDIS_ADDR` 和 `TEST_REDIS_PASSWORD`，保留你的本机密码。

PowerShell：

```powershell
$testNames = @('TEST_MYSQL_DSN', 'TEST_MIGRATE_MYSQL_DSN', 'TEST_INVENTORY_MYSQL_DSN', 'TEST_SEED_MYSQL_DSN', 'TEST_ORDER_MYSQL_DSN', 'TEST_SALES_MYSQL_DSN', 'TEST_REDIS_ADDR', 'TEST_REDIS_PASSWORD', 'TEST_SHOP_MYSQL_RO_DSN', 'TEST_SHOP_REDIS_RO_URL', 'TEST_DOCTOR_MYSQL_DSN')
# 只从本次 .env 读取，避免旧终端残留的值掩盖缺项。
$testNames | ForEach-Object { [Environment]::SetEnvironmentVariable($_, $null, 'Process') }
Get-Content .env | Where-Object { $_ -match '^TEST_(((MIGRATE|INVENTORY|SEED|ORDER|SALES|DOCTOR|SHOP)_)?MYSQL_DSN|REDIS_ADDR|REDIS_PASSWORD|SHOP_MYSQL_RO_DSN|SHOP_REDIS_RO_URL)=' } | ForEach-Object {
    $entry = $_ -split '=', 2
    [Environment]::SetEnvironmentVariable($entry[0], $entry[1], 'Process')
}
$missing = @($testNames | Where-Object { [string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($_, 'Process')) })
if ($missing.Count -gt 0) { throw "请补齐 .env 中的变量：$($missing -join ', ')" }
$integrationLog = Join-Path $env:TEMP ('opsdoctor-integration-' + [guid]::NewGuid().ToString('N') + '.jsonl')
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed ./internal/shop/order ./internal/shop/sales ./internal/shop/product ./internal/doctor/store ./internal/doctor/source/mysqlinspect ./internal/doctor/source/redisinspect -run Integration -count=1 -timeout=90s -json | Tee-Object -FilePath $integrationLog
$testExit = $LASTEXITCODE
$events = @(Get-Content $integrationLog | ForEach-Object { $_ | ConvertFrom-Json })
$skipped = @($events | Where-Object { $_.Action -eq 'skip' -and $_.Test })
$passed = @($events | Where-Object { $_.Action -eq 'pass' -and $_.Test })
Remove-Item Env:TEST_MYSQL_DSN, Env:TEST_MIGRATE_MYSQL_DSN, Env:TEST_INVENTORY_MYSQL_DSN, Env:TEST_SEED_MYSQL_DSN, Env:TEST_ORDER_MYSQL_DSN, Env:TEST_SALES_MYSQL_DSN, Env:TEST_REDIS_ADDR, Env:TEST_REDIS_PASSWORD, Env:TEST_SHOP_MYSQL_RO_DSN, Env:TEST_SHOP_REDIS_RO_URL, Env:TEST_DOCTOR_MYSQL_DSN -ErrorAction SilentlyContinue
Write-Output "pass=$($passed.Count), skip=$($skipped.Count), exit=$testExit；记录：$integrationLog"
if ($testExit -ne 0 -or $skipped.Count -ne 0 -or $passed.Count -eq 0) { throw '集成验收未通过：需要有测试通过、exit=0，并且 skip=0' }
```

bash：

```bash
test_names=(TEST_MYSQL_DSN TEST_MIGRATE_MYSQL_DSN TEST_INVENTORY_MYSQL_DSN TEST_SEED_MYSQL_DSN TEST_ORDER_MYSQL_DSN TEST_SALES_MYSQL_DSN TEST_REDIS_ADDR TEST_REDIS_PASSWORD TEST_SHOP_MYSQL_RO_DSN TEST_SHOP_REDIS_RO_URL TEST_DOCTOR_MYSQL_DSN)
unset "${test_names[@]}"
while IFS= read -r line; do
  line=${line%$'\r'} # 兼容 Windows 保存的换行，不执行文件中的内容。
  case "$line" in
    TEST_MYSQL_DSN=*|TEST_MIGRATE_MYSQL_DSN=*|TEST_INVENTORY_MYSQL_DSN=*|TEST_SEED_MYSQL_DSN=*|TEST_ORDER_MYSQL_DSN=*|TEST_SALES_MYSQL_DSN=*|TEST_REDIS_ADDR=*|TEST_REDIS_PASSWORD=*|TEST_SHOP_MYSQL_RO_DSN=*|TEST_SHOP_REDIS_RO_URL=*|TEST_DOCTOR_MYSQL_DSN=*) export "$line" ;;
  esac
done < .env
for name in "${test_names[@]}"; do
  if [ -z "${!name}" ]; then echo "请补齐 .env 中的变量：$name"; exit 1; fi
done
integration_log=$(mktemp)
go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed ./internal/shop/order ./internal/shop/sales ./internal/shop/product ./internal/doctor/store ./internal/doctor/source/mysqlinspect ./internal/doctor/source/redisinspect -run Integration -count=1 -timeout=90s -v | tee "$integration_log"
test_exit=${PIPESTATUS[0]}
skipped=$(grep -c -- '--- SKIP:' "$integration_log" || true)
passed=$(grep -c -- '--- PASS:' "$integration_log" || true)
unset TEST_MYSQL_DSN TEST_MIGRATE_MYSQL_DSN TEST_INVENTORY_MYSQL_DSN TEST_SEED_MYSQL_DSN TEST_ORDER_MYSQL_DSN TEST_SALES_MYSQL_DSN TEST_REDIS_ADDR TEST_REDIS_PASSWORD TEST_SHOP_MYSQL_RO_DSN TEST_SHOP_REDIS_RO_URL TEST_DOCTOR_MYSQL_DSN
echo "pass=$passed, skip=$skipped, exit=$test_exit；记录：$integration_log"
test "$test_exit" -eq 0 && test "$skipped" -eq 0 && test "$passed" -gt 0
```

这里解析赋值而不执行 `.env` 的内容；Go 程序本身不自动读取该文件。不要把未经核对的配置文件当脚本运行。

只看最后一行 `ok` 不够：Go 会把主动跳过的测试也视为成功退出。以上汇总必须同时满足 `pass>0`、`skip=0`、`exit=0`。若有跳过，去记录里搜索 `SKIP`（PowerShell 的 JSON 记录搜索 `"Action":"skip"`），找到缺少的配置后重新跑整轮。

医生库由独立容器初始化，端口为 `127.0.0.1:3307`，不要把 `TEST_DOCTOR_MYSQL_DSN` 指向 3306 的业务实例。只读账号测试先用 `ACL DRYRUN` 确认危险命令会被拒绝，再实际发送；如果预检查发现权限过大，立即失败，不执行这些命令。MySQL 写权限测试使用永假条件，不修改业务行。

## 阶段 5：启动医生与手动取证

医生监听 `127.0.0.1:8090`。它自己的数据库保存登录账号、诊断任务、步骤和费用。Alertmanager 通知医生；旧 alertsink 不再出现在默认 Compose 环境，保留的源码只供回看阶段 3。

升级已有环境时，参照 `.env.example` 补齐 `DOCTOR_*`、`SHOP_ALERT_TOKEN`、`SHOP_MYSQL_RO_PASSWORD`、`SHOP_MYSQL_RO_DSN`、`SHOP_REDIS_RO_PASSWORD` 和 `SHOP_REDIS_RO_URL`。密码只填在本机 `.env`。接入 YAML 里只放变量名。`SHOP_*` DSN/URL 使用容器主机名；`TEST_SHOP_*` 使用 `127.0.0.1`。Redis URL 中的特殊密码字符需要做 URL 编码。

**每次重建前，重新确认 `.env` 或本终端里的 `LOADGEN_PRODUCT_COUNT=10000`、`LOADGEN_QPS=200`、`LOADGEN_WORKERS=64`。只改运行时控制接口不会保留到重建之后。首次空库则先用 QPS=0，造数完成后再开流量。** 这是一万商品工作集的告警验收基线，商品表仍有五十万行。默认五十万商品的 CacheLoadsHigh 误报尚未修复，见上方基线说明和数据台账 P5-8。

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
# 首次空库的业务表要等迁移完成后才存在，因此再补授精确的表级权限。
docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql bash /docker-entrypoint-initdb.d/006_doctor_readonly.sh
Invoke-RestMethod http://127.0.0.1:8090/health/ready
```

你应该看到医生健康接口成功。已有账号的密码不会被这个脚本自动重置；修改 `.env` 不等于修改数据库中的账号密码。Redis 的 ACL 在每次启动时从环境生成，密码不会出现在启动参数里。

登录、环境变量注入、十六项工具菜单和 S01 的逐步操作见[阶段 5 学习材料](docs/learn/phase-05-doctor-tools.md)。登录令牌只放当前终端的 `DOCTOR_TOKEN`，不要贴到命令参数或笔记里。从阶段 6 起，新任务会自动执行；旧 pending 告警按过时规则取消。医生仍只有查看权限，报告中的建议不会自动执行。

## 阶段 6：打开实时诊断页面

日常学习在本机 `.env` 保持 `LLM_PROVIDER=mock`。浏览器打开 `http://127.0.0.1:8090`，登录后先确认顶部的“Mock 模式，不是真实诊断”。选系统、填症状、发起诊断，即可看到固定脚本产生的步骤和报告。Mock 不会联网调用模型，也不能用来衡量根因判断是否准确。

真实验收使用官方 DeepSeek 4.1 Flash：`LLM_PROVIDER=openai`、`LLM_BASE_URL=https://api.deepseek.com`、`LLM_MODEL=deepseek-flash`。Key 只放本机 `.env` 的 `LLM_API_KEY`。预算、单价和超时变量见 `.env.example`；本次授权全局上限为 ¥20。更改环境文件后需要重新创建 doctor 容器，已运行进程不会自行读取新值。验收结束后切回 mock 并重新创建 doctor，再确认页面横幅。不要在真实模式跑集成测试或 chaos selftest。

同一组故障持续期间，多次 firing 通知会并入原诊断，即使它已经结束。整组 resolved 后还会观察一段时间，shop 在接入配置中设为 3 分钟。期间重新出现的告警仍并入原诊断，页面单独列为“恢复观察期内的后续告警”，报告不重跑。观察期内若真的发生新故障，可以手动发起诊断。恢复通知丢失时仍有十五分钟通知静默兜底。

下一场实验除了通过 `chaos healthy`，还要读取 `GET /api/v1/targets`，等 shop 的 `fault.open` 变成 false。`fault.recovery_until` 是预计观察结束时间；又有 firing 时它会变回 null，不能按旧时间或固定 sleep 猜测故障已经结束。也要确认 Prometheus 没有仍在 pending 的症状，避免刚恢复流量时的真实慢请求稍后才满足告警的持续时间。

页面展示每次模型调用的实际模型名；点击步骤可以查看脱敏后的请求与结果。费用是按未命中缓存单价算的保守值，取消时无法确认的费用预留也包含在内。

页面用 HttpOnly Cookie 登录，浏览器自动携带它建立 SSE 连接。令牌不出现在 URL 中。命令行仍使用 Bearer。模型输出只当文字显示，建议由人核对后执行。详细讲解见[阶段 6 学习材料](docs/learn/phase-06-agent.md)。

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

阶段 4 增加一次性的商品查询突发。下面让 16 个已有工作者同时查询商品 1，不会下单或扣库存。每批请求数不能超过启动时配置的工作者数，上一批未结束时会拒绝叠加。202 只表示接收，实际完成数查 `GET /control` 中的 `burst`。

```powershell
Invoke-RestMethod http://127.0.0.1:6070/burst -Method Post -ContentType 'application/json' -Body '{"sku_id":1,"requests":16}'
Invoke-RestMethod http://127.0.0.1:6070/control | ConvertTo-Json -Depth 5
```

S04 会先暂停普通流量，再对一组商品进行有限批次的突发；不接受通用 `--qps`、`--sku` 覆盖。它不会把下单集中到热点，单商品下单竞争仍由 S08 单独实验。

## 阶段 4：准备与运行故障实验

本阶段命令在宿主机运行，场景范围是 S01–S10、S13。连续两轮自测 22/22 通过，完整结果见[汇报书](docs/reports/phase-04-chaos.md)、[学习材料](docs/learn/phase-04-chaos.md)和[数据台账](docs/数据台账.md) P4-18、P4-19。先停止其他手工实验，准备完整业务数据和足够库存。

阶段 4 的健康基线使用 **目标 200 QPS、10000 个活跃商品、64 个固定工作者**。各场景会按定义临时调整流量，实际完成速率单独观察。数据库仍有五十万商品；活跃范围只决定压测器会选择哪些商品，不会缩小表。当前缓存回源告警按这组负载校准，不能直接套到默认的五十万活跃商品上；扩大范围时，正常未命中也会增加，需要先重测基线。具体失败与对照数据见数据台账 P4-12、P4-13、P4-16。

缓存告警返工后，同时判断回源比例和此前流量，并要求条件持续三十秒。停流超过十一分钟后恢复、随后持续健康运行、S04 和 S05 各两次定向流程均已实测，额外完整 selftest 也通过 11/11；数字见 P4-28 至 P4-39。具体判断与适用范围见[告警规则说明](deploy/prometheus/rules/README.md)。

长实验会积累尚未过期的幂等记录。Redis 写入预算现为 512 MiB，容器上限 768 MiB，保持二十四小时期限和 noeviction。noeviction 表示内存不足时拒绝新写入，保留原有凭据。原 128 MiB 预算曾被约三十万个幂等键填满，导致下单被拒绝，记录见 P4-17。继续增加实验时长时需要重新估算容量，不能靠清除凭据维持流量。

PowerShell 在启动 Compose 前设置本次实验参数；这些值只影响当前终端，不覆盖 `.env`：

```powershell
$env:LOADGEN_PRODUCT_COUNT = '10000'
$env:LOADGEN_WORKERS = '64'
$env:LOADGEN_QPS = '0'
```

bash 对应为 `export LOADGEN_PRODUCT_COUNT=10000 LOADGEN_WORKERS=64 LOADGEN_QPS=0`。准备完成后再通过控制接口开启 200 QPS，先等健康检查通过，再开始场景。

先给本机 `.env` 增加独立的 `CHAOS_MYSQL_PASSWORD`，不要覆盖已有配置。空库初始化时只有库、没有业务表，脚本先创建实验账号；服务健康表示业务迁移已完成，再显式运行脚本授予两张表的权限。已有数据卷也需要这个步骤，因为 MySQL 不会自动重跑初始化脚本：

```text
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml exec -T shop-mysql bash /docker-entrypoint-initdb.d/005_chaos_user.sh
```

空库首次启动仍先设 `LOADGEN_QPS=0`，造数后才开启流量。Toxiproxy 管理端口为 127.0.0.1:8474；订单→库存、库存→MySQL、订单→Redis 默认经过代理。宿主机 shopctl 仍用原库存端口。

准备 S09 的另一个镜像。该命令只构建，不会立即切换正在运行的服务：

```text
docker build -f deploy/docker/go.Dockerfile --build-arg CMD_NAME=order-service --build-arg APP_VERSION=20261026.2 --build-arg GO_TAGS=leakbug -t opsdoctor/order-service:20261026.2 .
```

实验账号不使用业务账号。PowerShell 从本机配置读取密码并组装 DSN，整个过程不打印它：

```powershell
Get-Content .env | ForEach-Object {
    if ($_ -match '^(CHAOS_MYSQL_PASSWORD|REDIS_PASSWORD)=(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
    }
}
$env:CHAOS_MYSQL_DSN = "chaos:$($env:CHAOS_MYSQL_PASSWORD)@tcp(127.0.0.1:3306)/shop?parseTime=true"
```

bash 对应写法：

```bash
while IFS= read -r line; do
  case "$line" in CHAOS_MYSQL_PASSWORD=*|REDIS_PASSWORD=*) export "$line" ;; esac
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

阶段 6 起，`chaos healthy` 和 `selftest` 会先读取 loadgen 控制接口的活跃商品数和工作者数。默认要求 10000 和 64。若重建后回到五十万商品，命令会立即失败，并打印应设置的变量和重建命令。自定义实验需同步设置 `CHAOS_BASELINE_PRODUCT_COUNT`、`CHAOS_BASELINE_WORKERS`；这只是明确实验条件，不会修复五十万工作集的告警误报。

完整自测用 `go run ./cmd/chaos selftest`。先暂停流量并按前面的 restock 命令显式准备库存，再恢复流量、等 healthy 通过。每一轮的完整结果表都保留，包括失败轮次。工具不会在自测内部自动补库存。

实验状态保存在忽略文件 `.opsdoctor-chaos.json`。同一时刻只允许一个活动实验。中断后运行 `go run ./cmd/chaos restore-all`，它按预先保留并核对过内容 ID 的原镜像、公开配置和流量恢复；恢复失败时保留文件，修复外部环境后可重试。这个文件含参考答案，不要挂载进业务或医生容器。恢复环境不会自动处理过去已经产生的待核对订单。

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
- 医生已接登录鉴权；监控和靶场入口仍用于本机实验，不能直接暴露到公网。
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
