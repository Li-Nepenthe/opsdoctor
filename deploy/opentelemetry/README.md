# OpenTelemetry Demo：第二套被诊断系统

阶段 9 要检验医生能否诊断一个陌生系统。这里保存官方 Demo 的固定版本和本机隔离配置；业务源码使用官方仓库，放在仓库外的临时目录，避免把另一个项目复制进来。

| 文件 | 作用 |
|---|---|
| [versions.env](versions.env) | 把上游默认的镜像版本固定为 3.1.0；只含公开版本号 |
| [compose.phase09.yaml](compose.phase09.yaml) | 为所有容器和网络加专用名称，替换全部宿主机端口 |
| [otelcol-config-extras.phase09.yaml](otelcol-config-extras.phase09.yaml) | 固定版 Collector 的日志属性改名，避免 `http` 文本与 `http.` 对象路径争用同一映射 |
| [doctor.phase09.yaml](doctor.phase09.yaml) | 在通用双服务 Compose 上覆盖阶段 9 的容器名、预算和宿主评测端口 |
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

阶段 9 现在使用[通用独立医生 Compose](../doctor/compose.yaml)作基础，再加载本目录的薄覆盖。薄覆盖只补本阶段的身份、预算和评测端口，不能单独作为完整 Compose 启动。项目名为 `opsdoctor-phase09-doctor`，医生与新库的容器分别为 `opsdoctor-phase09-doctor`、`opsdoctor-phase09-doctor-mysql`；网络和新卷 `opsdoctor-phase09-doctor_doctor-state` 都属于这个项目。它不再复用原项目网络或 `opsdoctor_doctor-mysql-data`，旧诊断和阶段 8 数据不迁移。

先按[医生部署说明](../doctor/README.md)准备医生自身配置和目标凭据两份私有文件。本阶段命名为仓库根目录的 `.env.phase09-doctor` 与 `.env.phase09-targets`，原 `.env` 不覆盖。前者以[通用假值示例](../doctor/standalone.env.example)为格式，填写医生自己的新库密码、登录、JWT 和路径；后者只填接入 YAML 引用的系统凭据。本次只加载 otel-demo，`DOCTOR_TARGETS_DIR` 指向仓库外单独目录，精确复制 [otel-demo.yaml](../doctor/targets/otel-demo.yaml) 和 [otel-demo.profile.md](../doctor/targets/otel-demo.profile.md) 两个文件，`DOCTOR_TARGET_ENV_FILE` 指向私有凭据文件的绝对路径，其中提供独立的 `OTEL_DEMO_ALERT_TOKEN`，不要求任何 `SHOP_` 变量。接入自己的系统时见[完整接入指南](../../docs/onboarding.md)。

命令行 `--env-file` 供 Compose 插值，容器 `env_file` 则把目标凭据传给医生；自身和模型参数由 Compose 的 `environment` 设置，优先于容器 `env_file`。因此两类变量分开存放。仓库的 Git 与 Docker 忽略规则均排除了本阶段的 `.env.*`，不把秘密复制到构建上下文。自选其他文件名或目录时，也必须分别核对这两种排除规则。

首次验证保持 Mock：PowerShell 先设置 `$env:LLM_PROVIDER='mock'`、`$env:EVAL_COMPARISON_ENABLED='false'`；bash 先执行 `export LLM_PROVIDER=mock EVAL_COMPARISON_ENABLED=false`。以下命令从仓库根目录执行，在两个 shell 中相同：

```text
docker compose --env-file .env.phase09-doctor -p opsdoctor-phase09-doctor -f deploy/doctor/compose.yaml -f deploy/opentelemetry/doctor.phase09.yaml config --quiet
docker compose --env-file .env.phase09-doctor -p opsdoctor-phase09-doctor -f deploy/doctor/compose.yaml -f deploy/opentelemetry/doctor.phase09.yaml up -d --build --wait --wait-timeout 240
docker compose --env-file .env.phase09-doctor -p opsdoctor-phase09-doctor -f deploy/doctor/compose.yaml -f deploy/opentelemetry/doctor.phase09.yaml ps
```

医生页面默认仍为 `http://127.0.0.1:8090`。通用基础文件不发布数据库端口；阶段 9 薄覆盖额外发布 `127.0.0.1:3309`，只供宿主评测器访问新医生库。医生容器自身仍连本项目的 `doctor-mysql:3306`。**宿主评测器的 `DOCTOR_MYSQL_DSN` 必须使用新库的 doctor 账号，指向 `127.0.0.1:3309` 的 doctor 库**，不能沿用阶段 8 的旧库 DSN。MySQL 官方入口只创建医生库和账号，医生自己执行表迁移，不挂原演示的测试库初始化脚本。

薄覆盖固定 `phase-09-openai`（USD）和 `phase-09-deepseek`（CNY），总上限各为 10。切换前已核对旧库阶段 9 生成请求为 0、美元支出为 0，再启用新状态卷；这次没有借换库重置已经发生的费用（P9-27）。P9-27 的当时检查中，Mock且comparison关闭，人民币账户尚未实例化；后续已实际创建两个账户并核对各自额度与零支出（P9-32、P9-41），不能混淆历史检查点。美元继续按项目用量账本计，Key 与其他用途共用，平台账单无法单独核对。

当前独立医生镜像为 `phase09-20261102.4`，包含 P9-21 的级别大小写补丁及 P9-38 的 Jaeger 安全路径前缀接线。工具仍传大写 INFO，OpenSearch 可以匹配实际小写 info；服务和 trace 仍精确匹配。早先同一医生注册两个系统并查询两个日志后端的事实保留在 P9-8、P9-9；本次独立实例按追加要求只选择 otel-demo，不能把旧双系统菜单当作新实例的菜单。

实际独立部署从准备私有配置、配置验证、构建到健康为 33.696456 秒，到登录和菜单为 33.8045 秒，医生内 `SHOP_` 环境变量为 0（P9-25）。这是已有本机源码与镜像条件下的计时，不含开发、Demo 恢复或诊断。恢复 Demo 后，仅新双服务与 Demo 运行，目标仅 otel-demo、菜单 8 个只读工具；`query_metrics` 的 `up` 查询为 `status=ok`，但返回 0 条序列，不能写成非空指标验证。`search_logs` 按 frontend、INFO 和关键词返回 5 条（P9-26），原件见[独立查询记录](../../docs/reports/phase-09-standalone-queries.json)。这一早期接线检查尚未运行整轮诊断；后续完整双 Mock 和付费正式结果另见下文，不能混用测量边界。

原医生容器保持停止；原 doctor-mysql 在新双服务健康并完成旧账核对后仅执行 `stop`，卷和原数据保留。早期复验资源见 P9-27；本阶段最终同样仅保留新 Mock 医生与新库，Demo 全部 stop，资源复核见 P9-79。需要停止新医生时，继续使用同一组配置并保留数据：

```text
docker compose --env-file .env.phase09-doctor -p opsdoctor-phase09-doctor -f deploy/doctor/compose.yaml -f deploy/opentelemetry/doctor.phase09.yaml stop
```

医生根文件系统只读，单独的内存 `/tmp` 用于免费网络预检程序。预检仅查询两家模型列表，使用医生容器中的 Go HTTP 网络路径，不调用生成接口；程序和记录放在本机临时目录，Key 只从环境读取。

医生只读取接入配置和[系统说明书](../doctor/targets/otel-demo.profile.md)，按固定地址查 Prometheus、Jaeger 和 OpenSearch。它不持有 Demo 的开关接口、场景文件或 Docker socket。PostgreSQL、Valkey 直查与 pprof 按数据源缺失降级。

评测器在宿主机读取[独立场景目录](../../eval/scenarios/otel-demo/README.md)，通过官方开关接口注入和恢复。它保存原始开关配置，恢复后读回确认；新的批次编号与两个预算账户都和阶段 8 分开。提示、工具、通用手册及评分保持阶段 8 冻结版本。独立部署及隔离适配的检查另见台账 P9-24–P9-28。采集修复已获 Claude 审核通过；首次试跑因采集起点审计异常在模型创建前中断（P9-41），第二次试跑因评委格式失败停止（P9-53）。两次原失败及费用保留；后续新正式批次结果单列如下。

## 正式前置规则与最终结果

用户撤回“评分必须成功才继续”：已返回且计量可确认、但冻结校验拒收的评分失败保留为缺失，不重评；新 run 累计第三份仍停，API、预算、采集、时限和基础设施等其他停点保留。OT03 判定仅限定免费实查的 PaymentService/Charge SERVER，五个 within 和阶段 9 恢复等待为 5m，[2m] 和其他判据保持原样（P9-58、P9-59）。

[历史回放](../../docs/reports/phase-09-historical-baseline-replay.md)未全通过，原失败保留。用户随后仅批准健康基线改成每 5 秒一轮、5 分钟内同一轮五场景全部通过，开关/开放期/谓词不改，查询报错立即停。本轮实测的业务 spanmetrics 原生间隔中位 59.997 秒，已完成调用计数曾出现平台；因此等待方式处理的是 Demo 下单稀疏、业务指标约一分钟一个样本的问题，不是放宽错误条件（P9-67、P9-73）。原生证据与推断边界见[专项报告](../../docs/reports/phase-09-native-metrics.md)。

运行代码 ac71f30 的 [CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41679143036)通过后，真实 Demo 上的新双 Mock 五场景 E1/E2 完整结束，11 次基线通过、各 5 次症状和恢复都在 5 分钟内、10 个诊断采集窗均 normal，模型生成费用为零（P9-69、P9-72）。[逐次计时表](../../docs/reports/phase-09-dualmock-timing.md)区分健康等待和整次基线调用。

重新核对 pilot2 实际费用乘 10、两账户剩余额度与官方 DeepSeek 余额 ¥5.66 后，88 项门槛通过，直接启动新正式 run `phase09-formal-20261103T181204Z-terra`（P9-75）。Terra 医生、DeepSeek 评委、v8.0/v1.1/rubric-v1.1 均保持冻结，没有再试跑或重评。

正式五场景 E1/E2 全部结束，10 份 normal 报告和 10 份有效评分；两组各组件 3/5、机制正确/部分/错误 2/2/1、证据覆盖均值 0.75（P9-77）。[正式结果与阶段 8 并列](../../docs/reports/phase-09-results-summary.md)，[失败分析](../../docs/reports/phase-09-failure-analysis.md)保留未查明机制及工具返回截断的边界，不外推生产准确率。

正式账本 USD $0.325033、CNY ¥0.196047，项目累计 USD $0.352212、CNY ¥0.210967，两个账户各在原 10 上限内（P9-78）。美元按项目用量账本计；Key 与其他用途共用、平台账单无法单独核对。两币种不相加。

正式收尾已将医生切回 Mock、comparison 关闭，仅 stop 本次 Demo 全部 28 个容器。当前仅独立医生和专用库运行，原容器/卷、20 个测试项目和 138 个测试卷保留（P9-79）。[最终资源](../../docs/reports/phase-09-final-runtime.json)与[正式计时](../../docs/reports/phase-09-formal-timing.md)可复查。阶段 9 等待 Claude 验收，未合并 main，学习验收不代勾。
