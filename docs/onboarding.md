# 接入你自己的系统

OpsDoctor 可以与被诊断系统分开部署。你提供数据源的固定地址、独立凭据和一份架构说明，医生通过现有只读适配器查看证据，不需要复制你的业务源码，也不要求你的服务使用 Go。

本文使用[独立医生 Compose](../deploy/doctor/compose.yaml)和[环境变量示例](../deploy/doctor/standalone.env.example)。它只启动医生及医生自己的 MySQL，不启动 shop、压测、故障注入或监控栈。这个独立基座已经用 otel-demo 完成 Mock（用固定脚本代替模型回答）启动、登录和实际只读查询验证，具体条件与原件见第 5 节和台账 P9-25–P9-27；本文的虚构 warehouse 仍是配置示例，尚未实际部署。Mock 接线验证不代表真实模型诊断完成；真实模型须另行配置并获得付费授权。

## 1. 先准备两个目录和两类凭据

在仓库外准备一个只放接入 YAML、架构说明的目录，例如 `D:/OpsDoctorConfig/targets`，再准备一个保存目标凭据的环境变量文件，例如 `D:/OpsDoctorConfig/targets.env`。Linux 可以使用你自己选择的绝对路径。下文用虚构的 `warehouse` 系统说明格式；请替换为真实架构。

| 内容 | 放在哪里 | 用途 |
|---|---|---|
| `warehouse.yaml`、`warehouse.profile.md` | `DOCTOR_TARGETS_DIR` 指定的宿主目录 | 固定系统名、可访问的数据源和组件清单；只读挂载给医生 |
| `WAREHOUSE_ALERT_TOKEN` 等目标凭据 | `DOCTOR_TARGET_ENV_FILE` 指定的文件 | Compose 将它们作为环境变量交给医生；YAML 只保存变量名 |
| 医生库密码、管理员登录、JWT 密钥 | 本机 `deploy/doctor/.env.standalone` | 医生自己的持久化与人类登录，不是目标系统的数据源账号 |

目标凭据文件优先放在仓库外。若选择仓库内的其他文件名，必须同时从 Git 跟踪和 Docker 构建上下文中排除；只加 `.gitignore` 不能阻止 Dockerfile 的 `COPY . .` 把文件送入构建。默认 `.env.*` 路径已有对应规则，自选 `targets.env` 等名字时仍须核对 `.dockerignore`，不要只按扩展名猜测。

不要直接把仓库现有的整个 `targets` 目录作为新部署的默认输入。它包含真实 shop 和 otel-demo 配置，加载时会要求那些系统的全部已声明凭据。只放你实际要接入的文件；实验场景、答案、恢复记录和 Docker socket 不属于接入材料。

从[standalone.env.example](../deploy/doctor/standalone.env.example)复制一份本机私有配置，已有文件不要覆盖：

```powershell
if (-not (Test-Path -LiteralPath deploy/doctor/.env.standalone)) {
    Copy-Item -LiteralPath deploy/doctor/standalone.env.example -Destination deploy/doctor/.env.standalone
}
```

或在 bash 中执行：

```bash
cp -n deploy/doctor/standalone.env.example deploy/doctor/.env.standalone
```

填写其中的 `DOCTOR_MYSQL_PASSWORD`、`DOCTOR_MYSQL_ROOT_PASSWORD`、`DOCTOR_ADMIN_EMAIL`、`DOCTOR_ADMIN_PASSWORD`、`JWT_SECRET`。把 `DOCTOR_TARGETS_DIR`、`DOCTOR_TARGET_ENV_FILE` 改为上述两个绝对路径；`DOCTOR_HTTP_PORT` 是宿主机访问医生的端口，默认 `8090`。同机已有医生占用这个端口时，为独立项目换一个空闲端口，后文的页面、`DOCTOR_URL` 与 MCP 地址也同步替换。保持示例的 Mock 设置，先不要填真实模型 Key。具体部署项以[医生部署说明](../deploy/doctor/README.md)为准。

医生自己的库固定为 `doctor`、账号为 `doctor`。独立项目使用自己的数据卷，启动时执行医生表迁移；它不替你的业务库建表，也不会用医生库的账号查询业务系统。

## 2. 写一份最小接入配置

先在目标目录中创建 `warehouse.yaml`。下面只开放架构查询和 MCP，暂时不连接任何监控后端，适合先检查文件、身份和网络入口是否接对：

```yaml
name: warehouse
profile: warehouse.profile.md
alert_token_env: WAREHOUSE_ALERT_TOKEN
alert_recovery_window: 3m
mcp_token_env: WAREHOUSE_MCP_TOKEN
```

在 `targets.env` 中分别填写独立随机值：

```dotenv
WAREHOUSE_ALERT_TOKEN=
WAREHOUSE_MCP_TOKEN=
```

以上留空项必须先填写；告警、MCP、运维令牌各自至少 32 字节，不能互相复用。即使暂时只手动取证，`alert_token_env` 也是必填项。不需要 MCP 时，删除 YAML 的 `mcp_token_env`，也不必提供对应变量。

### shop 模板里的每个字段是什么意思

[shop.yaml](../deploy/doctor/targets/shop.yaml)展示了全部现有接入类别。它包含审批后的写能力，新系统初次只读接入时应省略最后一组，而不是给医生复制 shop 的写账号。

| 字段 | 怎么填、什么时候需要 |
|---|---|
| `name` | 系统唯一名，例如 `warehouse`。小写字母开头，只能含小写字母、数字和短横线，最多 32 字符；会进入 API 与 MCP 路径 |
| `profile` | 同目录的 `.profile.md` 文件名，不是 URL。不能包含路径或借符号链接读取目录外文件 |
| `alert_token_env` | 本系统告警推送令牌的**环境变量名**，例如 `WAREHOUSE_ALERT_TOKEN`；不在 YAML 写令牌值 |
| `alert_recovery_window` | 全组告警恢复后，继续把迟到症状归入同一次故障的观察期。必填，正时长且不超过 `24h`；按自己的最长规则 `for`、规则评估间隔和通知等待决定，不直接照搬 shop 的 `3m` |
| `prometheus` | Prometheus HTTP 根地址，例如 `http://metrics.internal:9090`；不是 `/api/v1/query` 路径 |
| `jaeger` | Jaeger 查询服务根地址，例如 `http://traces.internal:16686`；不是 OTLP 接收端口 |
| `loki` | Loki 查询根地址；与 `opensearch` 只能选一个 |
| `mysql_readonly_dsn_env` | 目标 MySQL 只读 DSN 的环境变量名。DSN 必须固定库名、独立账号和密码，不能用 `root` 或业务 `app` 账号 |
| `redis_readonly_url_env` | 目标 Redis 只读 URL 的环境变量名。当前实现要求独立 ACL 用户、密码和 DB 0，不能用 `default` 用户 |
| `pprof` | `服务名: HTTP根地址` 的映射。只访问映射中服务的 `/debug/pprof/goroutine?debug=1`；没有 Go goroutine 接口的服务不填 |
| `mcp_token_env` | 外部只读客户端的独立令牌变量名。省略时该系统不开放 MCP；它不同于告警令牌和网页登录 JWT |
| `mysql_ddl_dsn_env` | **可选写能力**：独立 INDEX 账号的 DSN 变量。要求已有只读 MySQL 来源，且两账号固定到同一主机、同一库；允许表来自 profile 组件表 |
| `write_timeout` | **可选写能力**：审批后操作的时限，默认 `2m`，允许 `30s` 到 `3m`；它不是只读工具超时 |
| `ops_api.token_env` | **可选写能力**：目标运维接口的独立令牌变量名 |
| `ops_api.services` | **可选写能力**：服务名到运维 HTTP 根地址的固定映射。目标服务必须实现已有 `/ops/pool`、`/ops/cache/warm` 协议，不是填上普通业务 URL 就能修复 |
| `ops_api.pool_min`、`pool_max` | **可选写能力**：允许申请的连接池范围，必须位于 `5–200` 之内；shop 示例分别为 `5`、`200` |
| `ops_api.warm_cache_service` | **可选写能力**：允许预热缓存的单个服务，必须已经列在 `ops_api.services` 中 |

凭据变量必须以系统名的大写形式加下划线开头：`warehouse` 对应 `WAREHOUSE_`，`my-app` 对应 `MY_APP_`。所有系统的变量名和实际令牌、账号身份都要独立；换一个变量名指向同一个数据库账号，仍会被拒绝。未知 YAML 字段、重复键、重名系统或已声明却未提供的凭据会使启动失败，不会静默跳过这个系统。

### 可选的 OpenSearch 日志配置

先由部署者查看真实索引、mapping 和一条日志，再填以下分组。下例字段布局沿用已经实测的 OpenTelemetry Demo 格式（P9-3、P9-15），`warehouse-logs-*` 只是新系统的索引示例，必须替换为自己的实际索引：

```yaml
opensearch:
  url: https://logs.internal:9200
  index: warehouse-logs-*
  username_env: WAREHOUSE_OPENSEARCH_RO_USER
  password_env: WAREHOUSE_OPENSEARCH_RO_PASSWORD
  fields:
    service: resource.service.name.keyword
    level: severity.text.keyword
    message: body
    timestamp: observedTimestamp
    trace_id: traceId.keyword
```

| 字段 | 约束与用途 |
|---|---|
| `url` | HTTP(S) 根地址；不允许内嵌用户名、密码、路径、查询串或片段 |
| `index` | 单个索引名，或带固定前缀、仅末尾有 `*` 的表达式；不支持 `*`、`_all`、逗号分隔的多个索引或工具调用时改索引 |
| `username_env`、`password_env` | 只读 Basic 认证账号的两个环境变量名，必须一起提供；后端未启用认证时可一起省略。不能把密码写进 URL |
| `fields.service` | 服务名精确匹配字段；若源字段是 text，通常选实际 mapping 中的 keyword 子字段 |
| `fields.level` | 级别字段；工具接受大写 `DEBUG/INFO/WARN/ERROR`，OpenSearch 查询忽略大小写，兼容来源的 `info` 等写法（P9-21） |
| `fields.message` | 正文文本字段，关键词翻译为 `match_phrase`，关键词始终只作为 JSON 值 |
| `fields.timestamp` | 真实 `date` 或 `date_nanos` 字段，用于范围、倒序和时间读取；先分清事件时间与采集观察时间 |
| `fields.trace_id` | Trace ID 精确匹配字段；源记录没有此值时可以缺值，但配置的字段路径仍必填 |

五个字段路径必须全部配置。它们只描述已有数据，不会帮你改变索引 mapping 或建立采集链路。`loki` 与 `opensearch` 同时出现会直接报错。OpenSearch 账号只授指定日志索引的查询权限；适配器只向固定索引发 `_search`，不提供写文档、改 mapping、删除或管理接口。

Loki 当前使用固定的 `service` 流标签，并在 JSON 正文中按 `level`、`trace_id` 筛选；这些名字没有 OpenSearch 那样的接入字段映射。Loki 的级别条件按工具的大写取值精确匹配，不自动沿用 OpenSearch 的大小写兼容。若自己的日志布局不同，不能只改一个 URL 就假定能查到。

## 3. 写系统 profile

profile 是医生能读到的架构资料。写正常业务、服务调用、表和缓存的职责、消息关系，以及指标和日志的实际命名；不要写实验场景、故障开关、容易出问题的组件、标准答案或要求医生执行的指令。可参照 [shop 说明书](../deploy/doctor/targets/shop.profile.md)和[外部系统说明书](../deploy/doctor/targets/otel-demo.profile.md)。

与最小 YAML 配套的 `warehouse.profile.md` 可以先写成：

```markdown
# Warehouse 的系统说明书

warehouse-api 接收货位查询，通过 inventory-api 查询可用数量。
两个服务的指标由本系统的 Prometheus 保存，服务标签与日志中的服务名一致。
本说明不提供故障答案；数据源是否可查以接入配置和实际查询为准。

<!-- COMPONENTS_START -->
| 标识 | 架构职责 |
|---|---|
| `unknown` | 当前证据不足以定位 |
| `none` | 在报告说明的时间和范围内未发现故障 |
| `warehouse-api` | 货位查询入口 |
| `inventory-api` | 查询可用数量 |
| `warehouse-api->inventory-api` | 两个服务之间的调用链路 |
<!-- COMPONENTS_END -->
```

把虚构文字换成自己的真实架构。组件表的标记必须原样保留，每个标识用反引号包住，不得重复，必须包含 `unknown`。服务用服务名，模块可写 `服务:模块`，MySQL 表用 `mysql:库.表`，链路用 `调用方->被调用方`。这些标识也是报告允许选择的范围，不能在看过某次故障答案以后临时补名。

所有接入 YAML 和 profile 在启动时一起读取，运行中不会自动重载。修改或增加文件后，先停止医生，再启动，使它重新读取配置；只对运行中的服务重复 `up`，不会因为 bind 目录里的内容变化就重新加载 targets：

```text
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml stop doctor
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml up -d --wait --wait-timeout 240 doctor
```

已经加载的多个系统之间，选择不同 target 不需要换配置或重启医生。P9-8、P9-9 已验证同一医生同时注册 shop 和 otel-demo，并按各自后端查询。

## 4. 数据源地址、账号与降级

地址要从**医生容器内部**能访问。`localhost` 指医生容器自己，不指宿主机，也不指你的业务服务。Docker Desktop 可以使用 `host.docker.internal` 访问宿主机；同一 Docker 网络可以使用服务名；远端系统使用医生实际可达的内网地址。独立 Compose 已声明 Linux 的 `host-gateway` 别名，但别名只解决地址解析，不会让仅绑定宿主回环的服务自动对容器可达。宿主发布端口与容器端口不要混用，复制 shop 地址前先检查网络是否相同。

目前 HTTP 数据源只接受根地址，不接受 URL 内的账号或任意 API 路径。Prometheus、Jaeger、Loki、pprof 没有独立的 Basic/Bearer 或租户头配置；OpenSearch 另外支持上面的 Basic 环境变量。已有后端要求这些鉴权方式时，当前配置不能直接完成认证，应由部署者提供适合医生只读访问的网络入口，不能把网页登录地址当成查询 API。

| 已实现来源 | 提供的只读工具 | 没有配置时 |
|---|---|---|
| 架构 profile | `service_topology` | profile 必填，缺失或组件表不合法则不启动 |
| Prometheus HTTP API | `list_firing_alerts`、`query_metrics`、`kafka_consumer_lag`、`recent_changes` | 不注册这些工具，报告说明缺少指标 |
| Jaeger 查询 API | `search_traces`、`get_trace` | 不注册调用链工具；其他链路后端不能直接假定兼容 |
| Loki 或 OpenSearch | `search_logs` | 不注册日志工具，不借其他 target 的日志 |
| MySQL | `mysql_explain`、`mysql_show_indexes`、`mysql_table_stats`、`mysql_top_queries`、`mysql_lock_waits`、`mysql_status` | 不注册 MySQL 工具 |
| Redis | `redis_info`，包括删去 key/value 的慢命令摘要 | 不注册 Redis 工具 |
| Go pprof goroutine 文本 | `goroutine_summary` | 不注册堆栈工具；已配映射里也不能查询未列出的服务 |

MySQL 账号应只授必要业务表的 SELECT 和固定监控列权限，不能复用业务写账号。[shop 的授权脚本](../deploy/mysql/init/006_doctor_readonly.sh)展示了当前工具使用的 `performance_schema` 列，应按自己的库表由管理员审核调整，不能把脚本不加修改地执行到生产库。查询计划只执行工具添加的 EXPLAIN，不执行原 SELECT；不支持任意 SQL、UPDATE 或 EXPLAIN ANALYZE。数据库账号权限与工具代码共同限制访问范围，不能仅靠“只读”这个名字。

Redis 使用独立 ACL 账号，只需要 `HELLO`、`PING`、`INFO`、`SLOWLOG GET/LEN` 等现有查看命令，不允许读取业务键或清库。[当前 ACL 示例](../deploy/redis/start-with-acl.sh)可用于核对权限范围。pprof 只提供固定服务的 goroutine 文本，不是任意进程分析或 shell 接口。

当前没有 PostgreSQL、通用 SQL、Elasticsearch、Tempo 的直接适配器；Kafka 只通过已有 Prometheus 指标查看积压，不直连管理接口。Valkey 在本阶段没有接入并验证独立只读账号，不能写成已验证的 Redis 替代品。也不负责替业务安装 exporter、埋点、日志采集器或修复动态 mapping 冲突。

数据源**未配置**时移除工具并明确缺什么；已配置但地址不可达、没有权限或返回异常时，工具报告失败，不能视为健康。已有工具也未必有数据：`kafka_consumer_lag` 使用固定指标名，`recent_changes` 依赖已采集的版本与启动指标，接入 Prometheus 不等于这些指标自动存在。P9-8 的外部系统因此只有 8 个只读工具，shop 有 16 个；这是按来源降级的实测，不是所有新系统的固定工具数量。

## 5. 启动独立医生并手动验收

确认配置和私有文件就绪后，先在当前终端显式选择 Mock，避免旧终端的同名变量覆盖 env 文件。PowerShell：

```powershell
$env:LLM_PROVIDER = 'mock'
$env:EVAL_COMPARISON_ENABLED = 'false'
```

bash：

```bash
export LLM_PROVIDER=mock EVAL_COMPARISON_ENABLED=false
```

然后从仓库根目录运行：

```text
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml up -d --build --wait --wait-timeout 240
```

它使用独立的 Compose 项目和医生数据卷，不依赖原 shop 项目的 doctor-mysql。医生页面默认为 `http://127.0.0.1:8090`；修改了 `DOCTOR_HTTP_PORT` 时使用相应端口。`/health/ready` 只检查医生自己的数据库，返回就绪不代表每个目标数据源都已接通。

手动 CLI 使用网页登录同一套 JWT。将与本机配置一致的 `DOCTOR_ADMIN_EMAIL`、`DOCTOR_ADMIN_PASSWORD` 安全载入当前终端环境；Compose 的 env 文件不会自动变成任意终端的环境变量。PowerShell 登录示例：

```powershell
$env:DOCTOR_URL = 'http://127.0.0.1:8090'
$doctorBody = @{ email = $env:DOCTOR_ADMIN_EMAIL; password = $env:DOCTOR_ADMIN_PASSWORD } | ConvertTo-Json -Compress
$doctorLogin = Invoke-RestMethod -Method Post -Uri "$env:DOCTOR_URL/api/v1/auth/login" -ContentType application/json -Body $doctorBody
$env:DOCTOR_TOKEN = $doctorLogin.data.access_token
```

bash 可以用本机 Python 通过同一个登录接口读取环境变量，令牌捕获到环境而不打印：

```bash
export DOCTOR_URL='http://127.0.0.1:8090'
export DOCTOR_TOKEN="$(python3 - <<'PY'
import json, os, urllib.request
payload = json.dumps({"email": os.environ["DOCTOR_ADMIN_EMAIL"], "password": os.environ["DOCTOR_ADMIN_PASSWORD"]}).encode()
request = urllib.request.Request(os.environ["DOCTOR_URL"] + "/api/v1/auth/login", data=payload, headers={"Content-Type": "application/json"})
with urllib.request.urlopen(request, timeout=15) as response:
    print(json.load(response)["data"]["access_token"])
PY
)"
```

不要输出登录响应或令牌。需要运行下面的 Go CLI 时，本机 Go 版本以 [go.mod](../go.mod)为准；CLI 通过 HTTP 调用医生，不另建数据源连接池：

```text
go run ./cmd/doctorctl target list
go run ./cmd/doctorctl tool list --target warehouse
go run ./cmd/doctorctl tool run --target warehouse service_topology --args '{}'
```

检查目标确实为 `warehouse`、架构正文正确、工具菜单与已配置来源相符。先用最小配置时，只有架构查询，不能据此要求日志工具出现。补上真实来源并应用配置后，再按自己的实际服务名取证，例如：

```text
go run ./cmd/doctorctl tool run --target warehouse query_metrics --args '{"query":"up","lookback_seconds":0}'
go run ./cmd/doctorctl tool run --target warehouse search_logs --args '{"service":"warehouse-api","level":"INFO","lookback_seconds":300,"limit":5}'
```

确认查询窗口内确实有已知记录，核对返回的服务、时间与正文。日志为空时先检查标签、字段、级别、采集与保留窗口；不要直接解释为没有请求或没有故障。工具 `status=ok` 说明本次查询完成，不是诊断准确率。用户名密码用于登录，`DOCTOR_TOKEN` 是登录 JWT；告警令牌和 MCP 令牌都不能替代它。

停止时保留医生数据以便复核：

```text
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml stop
```

### 本次独立入口的真实验证

资源先行复核见 P9-24 和[部署前快照](reports/phase-09-standalone-preflight.json)。实际使用上述独立基座，叠加[阶段 9 身份与预算覆盖](../deploy/opentelemetry/doctor.phase09.yaml)，只复制 otel-demo 的 YAML 与 profile，使用新建的医生数据库卷；没有加载 shop，也没有向新医生提供 `SHOP_` 环境变量。

从准备私有 env、复制目标文件、校验配置，到新医生和数据库健康，按同一进程的单调时钟计时为 33.696456 秒；它累计经过时间，不用两段 UTC 时间戳相减。同一起点累计到登录并读取菜单为 33.8045 秒（P9-25，[启动与登录原件](reports/phase-09-standalone-startup.json)）。本机源码和镜像已经缓存，计时不含随后恢复 Demo，也不含付费诊断，不能当作首次下载或任意机器的启动保证。

实际取证时只运行新医生、新数据库和 Demo，共 30 个容器；新医生只注册 `otel-demo`，菜单为 8 个只读工具。`query_metrics` 执行 `up` 返回 `status=ok`、0 条序列，只证明这次请求完成，不能称作非空指标证据；`search_logs` 按 frontend、`INFO` 和 API 关键词实际返回 5 条（P9-26，[真实查询原件](reports/phase-09-standalone-queries.json)）。这不是仓库示例 warehouse 的验收，也不是一次实际根因诊断。

该次验证收尾时 Demo 已 stop，仅保留新医生和新数据库可用，原数据库卷未复用，原容器身份与挂载名的核对结果保留；未执行真实模型生成或付费诊断（P9-27，[收尾运行快照](reports/phase-09-standalone-runtime-final.json)、[新库核对](reports/phase-09-standalone-database.tsv)）。独立入口补充后的[全仓检查](reports/phase-09-standalone-checks.json)另列 P9-28；没有启用的外部集成测试仍按跳过记录，不冒充通过。

针对 `up` 空结果，随后只临时启动 Demo 的 Prometheus 读取保留数据：`sum(traces_span_metrics_calls_total)` 回看一小时，实际返回 1 条序列、124 个采样点，`limited=false`，查询完成后再次 stop（P9-29，[独立后续响应](reports/phase-09-standalone-metric-query.json)）。这证明医生能够取得已有的非空指标，与之前 30 个容器运行时的 `up` 查询是不同观测，不能混成一次结果，也不能把历史采样当作当前系统健康或新鲜故障基线。

## 6. 让 Alertmanager 把告警发给医生

完成只读工具验证后，再把自己系统的 Alertmanager webhook 指向 `/api/v1/targets/warehouse/alerts`。`warehouse` 必须与 YAML 的 `name` 一致，使用 `WAREHOUSE_ALERT_TOKEN` 对应的独立 Bearer 令牌；网页登录 JWT、MCP 令牌和另一个系统的告警令牌都不能替代它。

下面沿用仓库[现有 Alertmanager 配置](../deploy/alertmanager/alertmanager.yml)的字段格式。地址 `doctor.example.internal:8090` 是待替换的网络示例；它必须从 Alertmanager 所在进程能访问，不能照抄医生宿主机的 `localhost`：

```yaml
route:
  receiver: doctor-warehouse
  group_by: []
  group_wait: 5s
  group_interval: 15s
  repeat_interval: 1m
receivers:
  - name: doctor-warehouse
    webhook_configs:
      - url: http://doctor.example.internal:8090/api/v1/targets/warehouse/alerts
        send_resolved: true
        max_alerts: 64
        http_config:
          authorization:
            type: Bearer
            credentials_file: /run/secrets/warehouse-alert-token
```

`credentials_file` 是 Alertmanager 进程可读的私有文件，内容为该目标令牌本身，不带 `Bearer ` 前缀。由部署者安全提供，示例不包含真实值。现有环境若有多个 receiver，应合入自己的路由，而不是直接覆盖整份 Alertmanager 配置；不同系统使用不同目标路径和不同令牌。上例按一个系统把症状聚合，自己的分组与观察期需结合规则设计。

`send_resolved: true` 让医生知道全组何时恢复。医生接收 Alertmanager 的原生通知对象，要求有效 `status`、`groupKey` 和告警列表；它不是只接收一段描述文字的聊天接口。身份正确且保存成功时返回 HTTP `202`，表示通知已接收，不代表诊断已完成。登录页面核对目标、告警时间和重复通知是否归到预期故障，再开始依赖自动诊断。

独立 Compose 默认只向本机发布医生端口。Alertmanager 位于另一台机器时，应由部署者配置其可达的受控入口；当前独立 Compose 不会自动把自己加入你的业务网络。开始真实模型调用以前，仍须先确认付费权限、模型配置和预算。Mock 只验证工具、存储与流程，不提供真实的根因判断。

## 7. 用 MCP 验证外部只读接入

在支持 HTTP MCP 的客户端中，把服务器地址设为 `http://127.0.0.1:8090/mcp/warehouse`，每次请求带 `Authorization: Bearer <WAREHOUSE_MCP_TOKEN的实际值>`。只在客户端的私有环境或凭据设置中提供真实值，不放进 URL、仓库或截图。完整客户端配置格式见 [MCP 使用说明](mcp.md)。

连接后先初始化、列工具，再调用 `service_topology`，参数为 `{}`。返回的应是本系统 profile，菜单中不应出现 `create_index`、`set_db_pool_size`、`warm_cache` 等写操作。一个 MCP 地址固定一个 target，不能靠工具参数提供另一个主机或系统名。更换 target 使用那个系统自己的 URL 和令牌。

MCP 工具调用不需要医生调用大模型，但客户端自己发起的模型对话可能另行计费。医生先把调用起点保存到自己的 `mcp_audit`，再执行只读工具，并保存结束状态；审计无法写入时不执行。由医生库管理员核对目标、工具名、状态和时间，未知工具或参数错误也应有审计，不能把只有客户端截图当作完整验证。

已有证据见台账 P9-10：[Codex 实际协议调用](reports/phase-09-codex-mcp-call.json)与[新增数据库审计](reports/phase-09-codex-mcp-audit.tsv)。当时实际列出 shop 的 16 个只读工具并调用 `service_topology`；传输由 Codex 通过 PowerShell HTTP 发起，验证一次工具调用，既不是桌面原生 MCP 配置，也不是一次完整自主诊断。不能把这个结果写成新 `warehouse` 已经完成 MCP 验收。

## 已验证的范围与仍待完成的事

| 证据 | 已验证什么 | 不代表什么 |
|---|---|---|
| P9-8、P9-9 | 同一医生加载 shop 与 otel-demo；按不同日志后端调用统一工具，缺来源则降级 | 任意新系统无需核对字段就能工作 |
| P9-3、P9-14、P9-15 | 固定 Demo 的真实日志、字段映射及采集修复窗口已有留档 | 新部署的日志一定完整、原始缺口已回填 |
| P9-21、P9-23 | OpenSearch 级别大小写兼容已通过真实集成和医生工具查询 | 已完成泛化诊断或付费评测 |
| P9-10 | 真实 Agent 通过 HTTP MCP 调用并留下审计 | 任意 MCP 客户端、任意系统均已实测 |
| P9-24、P9-25 | 复核资源，独立基座与阶段 9 覆盖配置在新库上完成 Mock 启动、登录和菜单读取 | 首次下载耗时、任意机器的时限保证，或 warehouse 模板已实测 |
| P9-26、P9-27 | 不提供 shop 环境，实际查询 otel-demo；日志非空，`up` 查询完成但无序列，新库与原卷隔离且未付费 | 所有数据源都有证据、端到端采集完整，或实际诊断已完成 |
| P9-28 | 独立入口补充后的全仓检查通过，跳过项单列 | Claude 已审核通过或泛化付费评测已完成 |
| P9-29 | 后续仅临时恢复 Prometheus，读取保留的一小时 span 指标，取得非空响应 | P9-26 的 `up` 当时非空、当前业务健康，或付费前的新鲜基线已确认 |

实测数字和原件统一从[数据台账](数据台账.md#阶段-9开源靶场泛化测试进行中)追溯。接入完成后，应为自己的独立启动、实际来源查询、告警通知和 MCP 调用分别留证；结果不足时明确缺什么，不把日志空结果、Mock 脚本输出或健康检查当成真实诊断能力。
