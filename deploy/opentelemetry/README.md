# OpenTelemetry Demo：第二套被诊断系统

阶段 9 要检验医生能否诊断一个陌生系统。这里保存官方 Demo 的固定版本和本机隔离配置；业务源码使用官方仓库，放在仓库外的临时目录，避免把另一个项目复制进来。

| 文件 | 作用 |
|---|---|
| [versions.env](versions.env) | 把上游默认的镜像版本固定为 3.1.0；只含公开版本号 |
| [compose.phase09.yaml](compose.phase09.yaml) | 为所有容器和网络加专用名称，替换全部宿主机端口 |
| [otelcol-config-extras.phase09.yaml](otelcol-config-extras.phase09.yaml) | 固定版 Collector 的日志属性改名，避免 `http` 文本与 `http.` 对象路径争用同一映射 |
| [doctor.phase09.yaml](doctor.phase09.yaml) | 专用医生容器，复用原医生数据库网络，固定阶段 9 两个预算账户 |
| [loki-verify.phase09.yaml](loki-verify.phase09.yaml) | 原 Alloy 缺少项目环境项时使用的独立对照采集器，验证后 stop |

上游固定为 [3.1.0](https://github.com/open-telemetry/opentelemetry-demo/releases/tag/3.1.0)，提交 `dedc0178918e260823323b8d95005a8cb924b007`。上游 `.env` 仍使用 `DEMO_VERSION=latest` 和 `IMAGE_VERSION=3.0.0`，因此命令需要在它之后加载本目录的版本文件。

本机首次启动发现 payment 的 Node 健康探测也会加载自动遥测模块，原命令耗时超过上游时限。覆盖文件只延长同一 TCP 探测的等待，业务和故障开关保持上游版本；原失败和复验见台账 P9-4。`versions.env` 同时显式固定已派生的 `OTEL_RESOURCE_ATTRIBUTES`，避免镜像已固定而运行元数据仍显示旧版本。

## 准备资源和源码

官方[部署说明](https://opentelemetry.io/docs/demo/docker-deployment/)给出的完整版需求约为 6 GB 内存、14 GB 磁盘。先查看主机可用内存、Docker/WSL 实际上限、Docker 数据盘所在物理盘的剩余空间，再查看 `docker system df`。稀疏虚拟磁盘内部的空闲空间可能大于物理盘空闲，应以两者较小值判断。

两套靶场串行运行。先用 `stop` 停止 shop 的业务、流量和监控容器，保留医生和医生数据库可用。历史测试项目、数据卷和 QQ 容器由用户管理，代理不清理。阶段 9 的实际资源记录见[台账 P9-1](../../docs/数据台账.md#阶段-9开源靶场泛化测试进行中)。

从仓库根目录，在 PowerShell 准备路径：

```powershell
$demo = Join-Path $env:TEMP 'opsdoctor-phase09\opentelemetry-demo'
git clone --branch 3.1.0 --depth 1 https://github.com/open-telemetry/opentelemetry-demo.git $demo
git -C $demo rev-parse HEAD
$local = Join-Path (Get-Location) 'deploy\opentelemetry'
Copy-Item -LiteralPath "$local\otelcol-config-extras.phase09.yaml" -Destination "$demo\src\otel-collector\otelcol-config-extras.yml"
$compose = @('--project-directory', $demo, '--env-file', "$demo\.env", '--env-file', "$local\versions.env", '-p', 'opsdoctor-otel-demo', '-f', "$demo\compose.yaml", '-f', "$demo\compose.full.yaml", '-f', "$demo\compose.observability.yaml", '-f', "$demo\compose.extras.yaml", '-f', "$local\compose.phase09.yaml")
docker compose @compose config --images
```

已有该临时目录时跳过 `git clone`，先核对固定提交，再按本节复制 extras 文件，不替换其他源码。应看到固定提交号和全部具体版本的镜像名。bash 对应写法：

```bash
demo="${TMPDIR:-/tmp}/opsdoctor-phase09/opentelemetry-demo"
git clone --branch 3.1.0 --depth 1 https://github.com/open-telemetry/opentelemetry-demo.git "$demo"
git -C "$demo" rev-parse HEAD
local_config="$PWD/deploy/opentelemetry"
cp "$local_config/otelcol-config-extras.phase09.yaml" "$demo/src/otel-collector/otelcol-config-extras.yml"
compose=(--project-directory "$demo" --env-file "$demo/.env" --env-file "$local_config/versions.env" -p opsdoctor-otel-demo -f "$demo/compose.yaml" -f "$demo/compose.full.yaml" -f "$demo/compose.observability.yaml" -f "$demo/compose.extras.yaml" -f "$local_config/compose.phase09.yaml")
docker compose "${compose[@]}" config --images
```

复制的是本仓库审阅过的 extras 配置，上游副本仍固定在上述提交，Collector 镜像仍为 `0.159.0`。完整版 Compose 已把这个文件挂载到容器并最后加载，因此不用另改上游基础配置。已有临时副本若带自行定制的 extras，先核对差异，不直接覆盖自己的定制。

这条规则只把日志属性键的 `http.` 前缀换成 `http_`，例如 `http.request.method` 变为 `http_request.method`。cart 启动日志原有的 `http="8080"` 和旧索引中的 `http` 文本映射保留。配置还原样保留上游的 scope 改名语句，因为 Collector 合并配置时会替换数组；漏掉它们会丢掉上游已经提供的处理。

本次启用前已确认目标 `http_` 命名空间为空。以后升级或新增日志源时，应重新检查：同一条日志若已经同时有 `http.request.method` 和 `http_request.method`，这个通用改名可能覆盖同名值，不能直接照搬为空的前提。修复原因、上游先例、原始错误和相对上游的 diff 见[采集修复专项报告](../../docs/reports/phase-09-logging-fix.md)。

## 启动和停止

PowerShell 使用 `docker compose @compose pull`，再执行 `docker compose @compose up -d --no-build --wait --wait-timeout 300`。bash 使用 `docker compose "${compose[@]}" pull` 和 `docker compose "${compose[@]}" up -d --no-build --wait --wait-timeout 300`。应看到本项目的容器运行，带健康检查的组件为 healthy。

停止使用 PowerShell 的 `docker compose @compose stop`，或 bash 的 `docker compose "${compose[@]}" stop`。停止后保留容器、文件和数据，供复核同一批实验。

| 入口 | 本机地址 |
|---|---|
| 商店 | `http://127.0.0.1:18000` |
| 官方开关管理界面 | `http://127.0.0.1:18080` |
| 官方流量生成器 | `http://127.0.0.1:18089` |
| Prometheus | `http://127.0.0.1:19090` |
| Jaeger | `http://127.0.0.1:19686` |
| OpenSearch | `http://127.0.0.1:19200` |
| Grafana | `http://127.0.0.1:13000` |

首次启动后先查询 OpenSearch 的索引、mapping 和一条真实日志。确认服务、级别、正文、时间和 trace ID 的实际字段，再填写医生接入配置。上游文档中的字段只是预期；日志没有进入 OpenSearch 时必须停止接入并交由用户决定。

## 给已有 Demo 加载采集修复

只在上述固定副本上应用本目录的 extras 文件。先按准备步骤复制配置，再用运行中的固定版 Collector 检查完整配置；以下命令在 PowerShell 和 bash 都适用：

```text
docker exec opsdoctor-otel-demo-otel-collector /otelcol-contrib validate --config=/etc/otelcol-config.yml --config=/etc/otelcol-config-full.yml --config=/etc/otelcol-config-observability.yml --config=/etc/otelcol-config-extras.yml --feature-gates=service.profilesSupport
```

确认退出码为零后，仅对本项目的 Collector 执行 PowerShell 的 `docker compose @compose stop otel-collector`、`docker compose @compose start otel-collector`；bash 对应 `docker compose "${compose[@]}" stop otel-collector`、`docker compose "${compose[@]}" start otel-collector`。这不会重建 OpenSearch 索引或数据卷。用同一索引 UUID 和重启后的时间窗核对结果，既看 Collector 是否仍报告映射冲突丢弃，也逐个检查实际启用 OTLP 日志的服务。

医生的字段配置继续为 `resource.service.name.keyword`、`severity.text.keyword`、`body`、`observedTimestamp`、`traceId.keyword`，索引限定为 `otel-logs-*`。这次改名没有改变这些字段；仍需用真实 mapping 与查询样本复验。`image-provider` 在固定版官方遥测矩阵中没有启用日志导出，不应要求它在 OpenSearch 中出现日志。

采集修复和验证材料须先交用户转给 Claude 审核；审核通过前不开始付费诊断。这一步只验证数据能到达医生，不能证明阶段 9 的泛化诊断已经通过。

## 医生和评测之间的分工

阶段 9 使用独立的 `opsdoctor-phase09-doctor` 项目运行一个医生，容器名也为 `opsdoctor-phase09-doctor`。原 `opsdoctor-doctor-1` 保持停止，避免两个调度器同时领取任务；原 `opsdoctor-doctor-mysql-1` 只执行启动，数据卷继续为 `opsdoctor_doctor-mysql-data`。医生的两个接入配置在同一次启动中全部加载，切换靶场时只启动或停止对应靶场，医生持续运行。

先准备 `.env` 中的独立 `OTEL_DEMO_ALERT_TOKEN`。首次验证保持 Mock：PowerShell 先设置 `$env:LLM_PROVIDER='mock'`、`$env:EVAL_COMPARISON_ENABLED='false'`；bash 在命令前加 `LLM_PROVIDER=mock EVAL_COMPARISON_ENABLED=false`。再执行：

```text
docker start opsdoctor-doctor-mysql-1
docker compose --env-file .env -p opsdoctor-phase09-doctor -f deploy/opentelemetry/doctor.phase09.yaml up -d --build --wait --wait-timeout 240
```

医生的页面继续使用 `http://127.0.0.1:8090`。专用 Compose 固定 `phase-09-openai`（USD）和 `phase-09-deepseek`（CNY），总上限各为 10，不沿用阶段 7、8 的账户；真实模型只在用户已授权的运行中显式启用。

医生根文件系统只读，单独的内存 `/tmp` 用于免费网络预检程序。预检仅查询两家模型列表，使用医生容器中的 Go HTTP 网络路径，不调用生成接口；程序和记录放在本机临时目录，Key 只从环境读取。

医生只读取接入配置和[系统说明书](../doctor/targets/otel-demo.profile.md)，按固定地址查 Prometheus、Jaeger 和 OpenSearch。它不持有 Demo 的开关接口、场景文件或 Docker socket。PostgreSQL、Valkey 直查与 pprof 按数据源缺失降级。

评测器在宿主机读取[独立场景目录](../../eval/scenarios/otel-demo/README.md)，通过官方开关接口注入和恢复。它保存原始开关配置，恢复后读回确认；新的批次编号与两个预算账户都和阶段 8 分开。提示、工具、通用手册及评分保持阶段 8 冻结版本。正式运行命令和实测结果在阶段 9 汇报中归档。
