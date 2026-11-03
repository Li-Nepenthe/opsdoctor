# 阶段 9：OpenTelemetry Demo 日志采集修复专项审核

本文供用户转交 Claude 审核采集配置。阶段 9 的付费诊断尚未开始，泛化评测尚未完成。用户选择先修复日志链路、审核后再评测；不能沿用“缺少部分日志也直接开跑”的假设。

修复已应用到固定版 Demo 的 Collector extras 配置，配置校验通过。修复后已保存固定的首十分钟观察窗口：未报告最终映射丢弃，预期 OTLP 日志源均有新日志；具体口径见第 6 节和台账 P9-14、P9-15。历史缺口保留在原索引和原始日志中，配置修复不会补回此前被拒绝的记录。

## 1. 用户要求与本次范围

| 用户要求 | 对应材料和边界 |
|---|---|
| 先量化哪些服务、哪类日志受影响，报告 `dropped_items`，说明场景取证影响 | 第 2、3 节；台账 P9-11、P9-13；保留初始和最终 before，两者不相加 |
| 先查上游 main，再参照 #3297、#3321，只改 Demo 采集配置 | 第 4、5 节；已核对固定 main 快照，未发现本次 `http` 冲突的修复 |
| 单列相对上游 diff，固定镜像和配置；重建索引或数据卷前询问 | [独立 diff](phase-09-upstream-config.diff)；本次没有重建、删除索引或数据卷 |
| 修复后无 mapping 丢弃，逐服务查询日志，核对适配器字段 | 第 6 节；固定窗口原始证据已归档，官方未启用日志的服务单列例外 |
| 配置 diff 与验证结果经用户转交 Claude，审核通过后才付费 | 第 7 节；不把本文当作付费批准或阶段 9 验收 |

这次修改位于 Demo 的采集链路：业务产生日志以后、写入 OpenSearch 以前。医生提示 v8.0、工具、通用手册 v1.1、rubric-v1.1 和场景故障开关均不因这次修复改变。

## 2. 修复前影响量化

### 2.1 两份 before 必须分开读

全部时间为 UTC。初始快照是首次发现问题时保存的证据；最终快照延长了同一修复前历史，包含初始窗口，不能把两列累加。

| 指标 | 初始 before（P9-11） | 最终 before（P9-13） |
|---|---:|---:|
| 当前容器日志的可见窗口 | 04:40:10.080405485–05:00:09.003373529 | 04:40:10.080405485–05:04:44.921223478 |
| OpenSearch 最终映射丢弃事件 | 193 | 316 |
| 对应 `dropped_items` 报告和 | 4006 | 6209 |
| 错误内嵌的单条 `mapper_parsing_exception` 次数 | 1438 | 2293 |
| 同窗 frontend 控制台 API 完成日志 | 1438 | 2296 |

来源：[数据台账 P9-11、P9-13](../数据台账.md#阶段-9开源靶场泛化测试进行中)、[初始量化 JSON](phase-09-log-impact-initial.json)、[最终量化 JSON](phase-09-log-impact-final.json)。初始窗口另有 OpenSearch 对照查询：匹配的 frontend API 完成日志为 0，但 frontend 其他日志仍有留存；这份查询没有被改标成最终窗口查询。

最终原始 Collector 输出见[完整 before 日志](phase-09-collector-final-before-fix.log)。文件为 2099650 字节，SHA-256 为 `d4842ea39085af9d43286dbf8b51bf87d1efd1c5592a1193f8f8cca8427b86cb`，与[修复前索引身份和文件指纹](phase-09-log-fix-before.json)一致。原始文件的错误包含以下理由：

```text
Could not dynamically add mapping for field [http.request.method].
Existing mapping for [attributes.http] must be of type object but found [text].
```

`6209` 是失败请求所涉及条目数的准确报告和，不能称为“已证明唯一丢失 6209 条日志”。固定版 [queue_sender.go](https://github.com/open-telemetry/opentelemetry-collector/blob/v0.159.0/exporter/exporterhelper/internal/queue_sender.go) 在调用下游之前记录整个请求的条目数；OpenSearch 的 [Bulk 索引实现](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/exporter/opensearchexporter/log_bulk_indexer.go)允许部分条目成功，再由[日志 exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/exporter/opensearchexporter/sso_log_exporter.go)汇总失败。因此同一个报告数可能包含已经成功写入的条目。单条 mapper 错误也缺少可做全局去重的业务日志 ID。

最终窗口 frontend 控制台 2296 条与单条 mapper 错误 2293 次相差 3，原因未知。没有逐条关联证据，不能擅自解释成在途日志、延迟、额外损失，也不能用它们推算丢失率。Prometheus 另有 4 次最终事件、80 个 metrics 条目；23 条重试消息也单列，不加入日志丢失数字。以上边界均来自台账 P9-13。

### 2.2 已定位的生产者和日志类型

cart 的 ASP.NET 启动警告把 `HTTP_PORTS` 放进名为 `http` 的结构化日志属性，实查值为文本 `8080`，先将 `attributes.http` 动态映射成 `text`。修复前索引保留了这类启动记录，不能把它解释为“cart 日志全部丢失”。cart 的 [Program.cs](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/cart/src/Program.cs)将 ILogger 接到 OTLP 日志导出；ASP.NET 的[官方日志模板](https://github.com/dotnet/aspnetcore/blob/v10.0.12/src/Hosting/Hosting/src/Internal/HostingLoggerExtensions.cs)说明 `http` 占位字段的来源。该框架 tag 仅用于核对相同模板，未据此声称运行中的精确 .NET 补丁版本。

frontend 的 [InstrumentationMiddleware.ts](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/frontend/utils/telemetry/InstrumentationMiddleware.ts)为 API 日志附上 `http.request.method`、`http.response.status_code`。OpenSearch 解释点号路径时需要 `attributes.http` 成为对象，撞上已存在的文本映射。这与控制台和错误记录相符，已证实这一路 API 日志存在缺口。

混合请求的 Collector 错误没有逐条业务服务 ID，因而不能把 `6209` 分给 frontend、cart 或其他服务，也不能列出每个后端各丢了多少。初始窗口可查到 ad、cart、payment、checkout、product-catalog 等后端日志，只说明部分日志确实留存，不能作为采集总量或损失率分母。

## 3. 对选定场景取证的影响

以下是采集缺陷对取证路径的影响分析，不是故障诊断成绩。场景尚未注入，不能把上述混合批次报告数按场景分摊。来源为[最终量化 JSON 的场景影响](phase-09-log-impact-final.json)和[场景定义](../../eval/scenarios/otel-demo/README.md)。

| 场景 | 受影响的交叉验证 | 仍需实际故障验证的证据 |
|---|---|---|
| OT01：ad | frontend API 日志缺口削弱上游影响范围的日志对照；后端日志、指标和调用链仍可使用 | ad 错误日志与失败调用链能否共同指向广告服务 |
| OT02：cart | cart 启动日志导致映射冲突，不代表 cart 业务日志全部丢失 | `EmptyCart` 的存储连接错误在日志与 trace 中是否留存 |
| OT03：payment | frontend 结账 API 日志缺口削弱 HTTP 症状的日志对照 | payment 的 `Payment request failed`、`Invalid token` 或相应 span 错误事件 |
| OT04：checkout → payment | 已知采集缺口下，查不到 payment 日志不能单独证明请求没有到达 | checkout 的错误地址、`UNAVAILABLE` 与出站调用证据 |
| OT05：product-catalog | 查不到 frontend 商品 API 日志不能解释成没有商品请求 | 失败 `GetProduct` 与其他商品成功请求的对照证据 |

## 4. 上游先例和 main 核验

上游 [#3297](https://github.com/open-telemetry/opentelemetry-demo/issues/3297)报告的是 `source.address` 点字段与 `attributes.source` 文本冲突。它与本次 `http` 同属叶子文本和对象路径争用同一映射，但不是相同属性。

上游 [#3321](https://github.com/open-telemetry/opentelemetry-demo/pull/3321/files)实际新增 `transform/sanitize_logs`，在 **scope** 上把标量 `otelcol.signal` 改名为 `otelcol_signal`，保留 `otelcol.signal.output` 使用的对象路径。合并提交为 `05fab63b804f9d243c34ac624f0a9083bec92127`。虽然 PR 关联关闭了 #3297，它的配置差异处理的是 scope 属性，不能写成“上游已经修好了 source 和 http”。

查到的 main 固定快照为 `e6754e07e275834b16515c55d0a909b86778ecc6`。其 [Collector 基础配置](https://github.com/open-telemetry/opentelemetry-demo/blob/e6754e07e275834b16515c55d0a909b86778ecc6/src/otel-collector/otelcol-config.yml)仍只有上述 scope 改名；核对该目录的分层配置，没有本次 `http` 冲突的处理。因此本次沿用“在 Collector transform 中通用改名”的先例，新增本次所需规则，没有升级到 main 或更换镜像。

## 5. 最小采集配置修复

部署仍固定 OpenTelemetry Demo `3.1.0`，源码提交 `dedc0178918e260823323b8d95005a8cb924b007`，Collector `0.159.0`。改动仅写在[本仓库 extras 配置](../../deploy/opentelemetry/otelcol-config-extras.phase09.yaml)，相对固定上游副本的差异单列为 [phase-09-upstream-config.diff](phase-09-upstream-config.diff)。这份 diff 是采集修复的审阅范围，不含此前已单独说明的 payment 健康探测时限修正。

在原来的 `transform/sanitize_logs` 中，原样保留 scope 的两条语句，仅增加日志上下文中的规则：

```text
replace_all_patterns(attributes, "key", "^http[.]", "http_")
```

它把所有以 `http.` 开头的日志属性键改到 `http_` 前缀，保持值和类型。例如 `http.request.method` 变成 `http_request.method`；不匹配标量键 `http`，也不匹配正文或资源属性。这里用 `[.]` 表示字面点号，避免 YAML 与 OTTL 的双层反斜杠转义。

不能只改名 cart 的标量 `http`：现有索引已经将该路径映射为 `text`，后来的 `http.request.method` 仍会要求同一路径是对象。这次迁移点前缀到空命名空间，保留旧字段和原索引。

固定版 [OTTL 函数文档](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/pkg/ottl/ottlfuncs/README.md#replace_all_patterns)支持按 map 键做正则替换，[实现源码](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/pkg/ottl/ottlfuncs/func_replace_all_patterns.go)会移动值到新键；这也意味着同一条记录若同时存在改名前后的键，可能覆盖值。本次启用前的[目标命名空间核验](phase-09-http-rename-precondition.json)未发现既有 `http_` 字段。这是固定系统下已核验的前提，不是对未来任意日志源的无冲突承诺。

Collector 的多文件合并会替换数组。extras 因而必须重复保留原有 scope 语句，再添加日志语句，不能只写新增的一项覆盖掉上游已有处理。原日志 pipeline 已引用这个 processor，不新增按服务、场景或故障开关判断的分支。

实际部署把仓库内 extras 复制到仓库外的固定上游副本 `src/otel-collector/otelcol-config-extras.yml`，先以固定版二进制验证完整配置，再仅 stop/start 此 Demo 的 Collector。具体 PowerShell、bash 复制及校验命令见[部署说明](../../deploy/opentelemetry/README.md#给已有-demo-加载采集修复)。没有重建或删除 OpenSearch 索引、数据卷，也没有操作原 opsdoctor 数据库或 QQ 容器。

## 6. 修复后验证

### 6.1 配置、索引和字段

完整配置已由运行中的 `/otelcol-contrib validate` 校验通过，退出码为 0。修复前[索引身份快照](phase-09-log-fix-before.json)与[修复后索引身份](phase-09-opensearch-indices-after.json)指向同一个 UUID `4Vc81FNnQEOVppU8gd17sw`；旧 `http` 映射仍为文本，新 frontend 样本的 method 已进入 `http_request.method`。校验命令、版本及配置指纹见[配置验证证据](phase-09-log-config-validation.json)，数据台账为 P9-15。

医生沿用如下字段配置；[完整 mapping 对照](phase-09-log-field-comparison.json)逐节点核对结果为 5/5 相同（P9-15）：

| 医生读取项 | 配置字段 | 修复前后实际 mapping |
|---|---|---|
| 服务名 | `resource.service.name.keyword` | keyword |
| 级别 | `severity.text.keyword` | keyword |
| 正文 | `body` | text |
| 时间 | `observedTimestamp` | date |
| Trace ID | `traceId.keyword` | keyword |

查询继续限定 `otel-logs-*`。规则只处理日志属性键，以上字段不变；这里的结论来自[原始真实 schema](phase-09-opensearch-schema.json)与 after 实查对照，没有只凭配置推断成功。

### 6.2 固定首十分钟窗口

观察从 Collector 实际重启时刻开始，固定为 `[2026-11-03T05:04:46.894786114Z, 2026-11-03T05:14:47.000000000Z)`，时长 600.105213886 秒。按这个时间条件分别保存 Collector 日志和 OpenSearch 查询，没有把修复前历史计入 after。来源为台账 P9-14、[固定窗口汇总](phase-09-log-fix-after.json)、[原始 after 日志](phase-09-collector-after-fix.log)、[逐服务原始响应](phase-09-log-fix-services.json)。

| 验收项 | 固定窗口实测 |
|---|---|
| Collector 最终 `Dropping data` 事件 | 0 |
| OpenSearch 日志 `dropped_items` 报告和 | 0 |
| mapper 错误行 | 0 |
| 有新日志的预期 OTLP 日志源 | 17/17 |
| `http_request.method` 记录 | 1884 |
| Jaeger traces 重试消息 | 10，单列，不计入最终日志丢弃 |

原始 after 日志 SHA-256 为 `aefd7cae491edff383abdac553c29499477cf6d03fdf6ac26a7d2264f442c806`。窗口内的零最终丢弃是 Collector 记录的精确结果，不证明上游没有静默漏发，也不证明业务结账、shipping 或未来任意输入都正常。Jaeger 重试仍存在，所以不能写成“整个 Collector 零错误”。

以下逐服务检查均要求 `observedTimestamp` 落在本窗口内，避免用修复前留存日志充当验证（P9-14，完整计数见原始响应）：

| 日志源 | 本窗口新日志 |
|---|---|
| accounting | 可查 |
| ad | 可查 |
| cart | 可查 |
| checkout | 可查 |
| currency | 可查 |
| email | 可查，包含受控合成记录 |
| fraud-detection | 可查 |
| frontend | 可查，含改名后的 HTTP 属性 |
| frontend-proxy | 可查 |
| load-generator | 可查 |
| payment | 可查 |
| product-catalog | 可查 |
| quote | 可查 |
| recommendation | 可查 |
| shipping | 可查 |
| kafka | 可查 |
| otelcol-contrib | 可查，Collector 内部遥测日志 |
| image-provider | 0；官方未启用 OTLP 日志，单列例外 |

逐服务验收以固定版[官方遥测矩阵](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/test/telemetry/services.py)和 Collector 配置为准。`image-provider` 的 `logs=False`，官方 nginx 配置的本地 access log 并没有被本次 OTLP 接收管道自动采入，必须单列为未启用日志的例外，不能要求“全部容器都有 OpenSearch 日志”。frontend-proxy 和 quote 则有官方 OTLP 日志接线，不应因为低频或初查为空而直接豁免。

email 的普通访问不一定触发业务日志。本次通过受控本地合成确认邮件请求触发业务路径，Pony 使用 `via: :test`，随后核对 `Order confirmation email sent` 及对应测试订单标记进入 OpenSearch；这不是向外部发送邮件。窗口内 email 共 9 条，其中测试订单标记精确匹配 1 条（P9-14）。不能把合成触发解释成一次成功的真实结账或自然流量的普遍覆盖率。

### 6.3 持续采样的扩展窗口

首窗口之后继续只读观察，扩展到 05:17:07.909028 UTC，共 741.014241886 秒；实际连续采样 20 次，查询无超时或失败分片，Collector 身份、镜像和启动点不变。扩展窗口仍是最终 drop、OpenSearch 日志条目报告和及 mapper 错误均为 0，预期源 17/17；窗口索引 24737 条、改名 method 2359 条，email 13 条中合成标记仍精确匹配 1 条。全部数字单列为台账 P9-18，不覆盖或叠加 P9-14。

见[扩展观察汇总](phase-09-log-fix-observation.json)与[逐次原始采样](phase-09-log-fix-samples.jsonl)。为避免重复保存首窗口，扩展日志仅归档[原始续段](phase-09-collector-after-fix-continuation.log)；将首窗日志的原始字节与续段按序拼接，可复原扩展日志并核对汇总中的 SHA-256。Git 对这两种原始证据禁用换行转换，[暂存对象字节检查](phase-09-evidence-byte-check.json)已确认与原始指纹一致（P9-19）。扩展验证沿用上一节全部局限。

## 7. 审核交付与下一步门槛

请 Claude 结合本报告、[配置文件](../../deploy/opentelemetry/otelcol-config-extras.phase09.yaml)、[上游差异](phase-09-upstream-config.diff)、两份 before、最终 after 原始日志和 mapping 对照，检查改名范围、目标命名空间前提、计数口径与服务例外。连续验证不合格时继续停在采集修复，不开启付费评测。

审核材料齐全并由用户确认 Claude 通过后，才重新核对免费网络与时钟预检、DeepSeek 余额和两个独立预算账户，然后开始已约定的 Terra 小规模试跑。美元仍按项目用量账本计；Key 与其他用途共用，平台账单无法单独核对。本文没有诊断准确率、正式 E1/E2 成绩或阶段 9 完成结论。
