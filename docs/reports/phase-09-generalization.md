# 阶段 9 汇报书：陌生系统接入与第二个日志适配器（进行中）

> 本报告是进行中记录，**阶段 9 尚未完成，工程验收未通过**。从 `main` 的 `231ee48` 切出 `phase-09-generalization`。用户于 2026-11-02 明确授权开始本阶段；阶段 10 已由 Claude 验收并合并，学习验收继续由用户本人决定。
>
> 当前停点：用户已撤回“评委评分必须成功才继续”，本轮按限定范围恢复阶段9评分失败处置、修订OT03业务SERVER范围与五分钟等待，并完成免费历史回放。按对应开关关闭口径，OT03为354/390、OT04为350/390，未达到100%；其余场景全部通过可评估点。因此未启动真实双Mock全矩阵，也未开启正式批次。本轮新增项目模型请求及费用均为0，旧pilot2的报告、失败评分与费用保持原样。Demo已stop，仅独立Mock医生与新库运行。工程和学习验收均未勾。数字与原始序列见台账P9-58–P9-64及[历史回放专项](phase-09-historical-baseline-replay.md)。

## 一、总览

### 1. 一句话总结

医生已能独立部署并真实调查外部 Demo；pilot2原报告及失败评分保留，本轮历史健康门槛未全通过，正式评测继续暂停。

### 2. 做了什么

接入固定版本 OpenTelemetry Demo，新增 OpenSearch 日志适配器，与 Loki 运行同一张契约测试表。先验证同一医生轮流查询两后端、真实 MCP 调用，再按用户追加要求拆出通用双服务部署：目标目录和凭据文件外置，只选 Demo 时完全不需要 shop 变量。评测器增加独立的外部系统入口，复用原评分设施。当前已有 1 份真实 Terra E1 报告与本次正常 Collector 窗口；医生自己仍保留“具体触发原因尚未确定”，评分协议失败，机制与证据分保持缺失。采集历史缺口、首次零诊断中断、此次失败评分及全部费用均保留。

### 3. 计划完成情况

| 计划书阶段 9 的“做”项 | 状态 | 当前证据与剩余工作 |
|---|---|---|
| 独立 Compose 部署固定版本 Demo，并核对日志进入 OpenSearch | 已完成 | 资源门槛、首次启动失败、修正后启动和实际 schema 均已留档；见 P9-1、P9-3–P9-5。采集修复的固定窗口验证见 P9-14、P9-15，Claude 已审核通过；后续每份报告单独审计采集窗口。 |
| 第二个日志适配器；接口和上层工具不变 | 已完成 | 新增 `source/opensearch`，支持服务、级别、关键词、trace ID、时间和条数；共享契约及真实只读查询通过。 |
| 接入配置、说明书、数据缺失降级；同一医生同时注册两个系统 | 已完成 | P9-8、P9-9：otel-demo 与 shop 并存，靶场轮流运行；没有切换医生配置或重启医生完成后端对照。 |
| 从上游选择 5–8 个故障开关并标注答案 | 已完成 | 选取 OT01–OT05，官方文档与固定 3.1.0 源码链接写入[场景目录](../../eval/scenarios/otel-demo/README.md)。已按用户授权免费验证 OT01 注入和恢复；没有完整 Mock 彩排。 |
| 不调参运行 E1、E2 | 试跑有报告；正式批次未做 | P9-51–P9-54保留当时的1份Terra试跑与评分失败；本轮按新规则做历史预检，OT03/04未全通过（P9-60/61），真实双Mock矩阵和正式E1/E2均未运行。 |
| 外部结果单列；如作通用改进则两靶场重跑 | 已单列试跑；完整比较未完成 | 原报告与失败评分分别保留，机制和证据有效评分为 0 份。提示 v8.0、手册 v1.1、rubric-v1.1 和工具保持原版本；未作通用诊断改进。 |
| 用户追加：医生单独部署、接入自己的系统说明 | 部署、只读接线和真实试跑报告完成；完整评测未完成 | P9-24–P9-29、P9-51：[通用双服务](../../deploy/doctor/compose.yaml)、[接入指南](../onboarding.md)。本次报告来自独立医生与新状态库，旧数据继续隔离保留。 |

## 二、新引入的技术与依赖

没有新增 Go 第三方依赖。新增适配器使用标准库 `net/http`、`encoding/json`；场景读取沿用仓库已有的 YAML 库。下列版本来自固定部署配置，不是性能测量结果。

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| OpenTelemetry Demo | 3.1.0，提交 `dedc0178918e260823323b8d95005a8cb924b007` | 第二套电商靶场；业务及配套 Demo 镜像固定 `3.1.0-*` | 上游提供公开架构和故障开关，可复用 Prometheus、Jaeger；版本选择见[决策记录](../decisions.md)末尾阶段 9 条目 | 不使用可变化的 `latest`，不改上游业务实现造症状 |
| OpenTelemetry Collector Contrib | 0.159.0 | 接收遥测、转发到三个观测后端，执行日志属性重命名 | 沿用固定 Demo 的采集链路；修复控制在部署配置中 | 不换采集器，不升级整套 Demo |
| OpenSearch | Demo `3.1.0-opensearch`；上游 Dockerfile 基于 3.7.0 及固定 digest | 第二日志后端 | 计划书指定的第二种日志实现，实际 mapping 已读取 | 不引入 OpenSearch Go 客户端，不把 AI 输出作为查询 DSL |
| Prometheus / Jaeger / Grafana | v3.13.1 / 2.19.0 / 13.1.0 | Demo 的指标、链路与界面 | 保持官方固定组合；shop 的原版本不变 | 不共用 shop 的监控实例 |
| flagd / PostgreSQL / Valkey | v0.16.0 / 18.4 / 9.0.4-alpine3.23 | 官方故障开关及业务依赖 | 作为外部靶场的既有组件运行 | 医生本阶段不增加 PostgreSQL、Valkey 专用适配器 |
| Grafana Alloy | v1.21.2，见独立对照 Compose | 临时采集 shop 日志，验证 Loki 路径 | 原 Alloy 缺少创建时环境项，新增独立采集器保护原容器配置 | 不重建或替换原 Alloy |
| Go / staticcheck | 1.27.1 / v0.8.2，沿用原工具链 | 编译、竞态和静态检查 | 保持现有工程环境 | 无新工具安装 |

全部端口和启动方式见[部署说明](../../deploy/opentelemetry/README.md)。上游 `.env` 中的 `latest`、旧镜像版本和已展开的版本属性，由[版本文件](../../deploy/opentelemetry/versions.env)显式固定。

## 三、代码明细

### 1. 新增和修改的包

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| `internal/doctor/source/opensearch` | 新增 | 固定索引和字段，生成查询 DSL，统一日志响应 | `Fields`、`Config`、`Client`、`New`、`Config.Validate`、`Client.Search`、`Client.Close` |
| `internal/doctor/source` | 修改 | 补充统一时间说明和两个日志实现的共享契约测试 | `LogSource` 的方法及请求、响应结构不变 |
| `internal/doctor/target` | 修改 | 校验日志后端互斥、字段和独立凭据，装配 OpenSearch，计算缺失数据源 | 原有 `Load`、`System` 接口不变 |
| `internal/eval/runner` | 修改 | 独立外部批次、开关注入与恢复、版本/账户/环境守卫 | 从原 `RunCLI` 分流 `generalization` 子命令；不向医生增加故障控制能力 |
| `internal/doctor/store` | 修改 | 允许阶段 9 独立批次使用 OT 编号，沿用既有轮次和执行审计 | 既有仓储方法签名、表结构不变 |
| `internal/eval/report` | 修改 | 外部系统来源标记、采集质量分表与正常样本统计资格 | 原 `Markdown` 接口不变；仍按模型分层，affected/unknown保留全部费用 |
| `internal/doctor/source/jaeger` | 修改 | 接入固定Demo的安全base path | 原TraceSource/Search/Get方法签名不变，共享HTTP层不变 |

### 2. 新增和修改的文件

本审核停点的完整新增/修改文件与行数见[git diff --stat](phase-09-diff-stat.txt)，逐文件职责见下表及[代码地图](../代码地图.md)。stat 包含新增证据文件，排除其自身和用户保留的两处修改。

| 文件或文件组 | 作用 |
|---|---|
| `source/opensearch/{config,client,response}.go`、`doc.go` | 配置边界、HTTP 搜索、字段/时间转换和包说明 |
| `source/opensearch/{client,integration}_test.go`、`source/log_contract_test.go` | 产品边界、显式真实查询和共享行为测试 |
| `target/{config,catalog,profile}.go`、`opensearch_test.go`、`doc.go` | 新后端配置、接线、缺失源识别及隔离测试 |
| `runner/generalization*.go` | 外部场景、批次、运行、资源与恢复；测试与说明跟随同目录 |
| `runner/{cli,clock,grading,manifest,paired}.go`、`doc.go` | 分流、专用时钟目标、独立评委账本、说明书哈希和阶段 9 异常停点 |
| `store/{eval_trials,eval_execution}.go`、`eval_generalization_test.go`、`doc.go` | OT 编号校验及旧编号兼容 |
| `report/markdown.go`、`markdown_test.go`、`doc.go` | 外部结果来源标记及回归 |
| `deploy/doctor/targets/otel-demo.{yaml,profile.md}` | 数据源、架构组件表与降级说明；不包含故障答案 |
| `deploy/opentelemetry/` | 固定版本、隔离部署、专用医生、临时 Loki 对照和 Collector 候选补丁 |
| `eval/scenarios/otel-demo/` | OT01–OT05、标准答案出处、症状及健康判断 |
| `.env.example`、`deploy/docker-compose.yml`、项目计划书及相关 README/文档 | 新变量、接入范围、限制、学习材料和本阶段证据入口 |
| `.github/workflows/ci.yml`、`.gitattributes` | Mock 冒烟核对双系统菜单与降级；按字节保留带指纹的原始采样证据 |
| `deploy/doctor/{compose.yaml,standalone.env.example,README.md}`、`docs/onboarding.md`、`.dockerignore` | 只运行医生与状态库；目标凭据外置、接入指南，以及任意层级私有.env文件的构建排除 |

本轮执行 `git diff --cached --stat main -- internal/doctor/tools internal/doctor/agent` 输出为空，命令与结果附在上述 stat 中。上层工具定义、提示和 Agent 循环均未修改。

### 3. 类型与函数清单

下表初稿来自 `go run ./tools/commentcheck -list ./...`，再对照工作区差异补充调用方。列出全部新增生产导出符号，以及接入、注入、持久化与并发守卫的关键内部符号；新增测试函数完整列在第四节。行号以当前工作区为准，提交前应随最终清单核对。

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 |
|---|---|---|---|
| `Fields` | [config.go:25](../../internal/doctor/source/opensearch/config.go) | 描述服务、级别、正文、时间和 trace ID 的真实字段 | 接入配置、客户端 |
| `Config` | [config.go:34](../../internal/doctor/source/opensearch/config.go) | 固定根地址、索引、字段及可选只读账号 | `target.bindHTTP` |
| `Config.Validate` | [config.go:43](../../internal/doctor/source/opensearch/config.go) | 拒绝路径、索引范围和映射错误 | `New`、`target.validateLogs` |
| `Client`、`New` | [client.go:35](../../internal/doctor/source/opensearch/client.go)、同文件:41 | 创建绑定单索引的有界客户端，不探测后端 | `target.bindHTTP` |
| `Client.Search` | [client.go:58](../../internal/doctor/source/opensearch/client.go) | 实现原 `LogSource` | 原 `search_logs` 工具 |
| `Client.Close` | [client.go:55](../../internal/doctor/source/opensearch/client.go) | 释放空闲连接 | target 关闭流程、测试 |
| `expression`、`Client.search` | [client.go:74](../../internal/doctor/source/opensearch/client.go)、同文件:91 | 生成固定 DSL；只向指定索引发送 `_search` | `Client.Search` |
| `entries`、`fieldValue` | [response.go:24](../../internal/doctor/source/opensearch/response.go)、同文件:79 | 检查来源、排序截断；兼容嵌套对象、点键和 `.keyword` | `Client.Search`、响应解析 |
| `logLine`、`timestamp` | [response.go:66](../../internal/doctor/source/opensearch/response.go)、同文件:101 | 保留结构化正文，转成统一 Unix 纳秒 | `entries` |
| `openSearchConfig`、`Config.validateLogs` | [config.go:47](../../internal/doctor/target/config.go)、同文件:161 | 配置公开地址/凭据变量，检查后端二选一 | 配置加载 |
| `credentials`、`credentialIdentity`、`System.bindHTTP`（修改） | [catalog.go](../../internal/doctor/target/catalog.go) | 凭据隔离、账号绑定和新后端接线 | `target.Load`、`build` |
| `missingSources`（修改） | [profile.go](../../internal/doctor/target/profile.go) | OpenSearch 配置存在时不再把日志列为缺失 | `build` |
| `generalizationCommand`、`generalizationManifest` | [generalization.go:37](../../internal/eval/runner/generalization.go)、同文件:46 | 保存 CLI 安排与不可覆盖的版本、顺序、端点和原始开关 | 新入口及批次校验 |
| `parseGeneralization`、`runGeneralizationCLI` | [generalization.go:60](../../internal/eval/runner/generalization.go)、同文件:111 | 先拒绝非法系统、旧批次和目录，再装配 | 原 `RunCLI` |
| `generalizationStore` | [generalization.go:162](../../internal/eval/runner/generalization.go) | 只连接医生库并配置统一脱敏 | `runGeneralizationCLI` |
| `generalizationConfigurations`、`generalizationPlan` | [generalization.go:189](../../internal/eval/runner/generalization.go)、同文件:206 | 核对阶段 9 的两个币种账户，固定版本和随机组序 | freeze、run |
| `freezeGeneralization` | [generalization.go:255](../../internal/eval/runner/generalization.go) | 健康后冻结安排并暂停该系统自动诊断 | freeze |
| `clockSampleArguments`、`demoClockKey` | [generalization_environment.go:28](../../internal/eval/runner/generalization_environment.go)、同文件:25 | 为外部实验选择固定专用医生采样 | `inspectClock`、新入口 |
| `checkDemoIsolation`、`demoMemoryBytes` | [generalization_environment.go:36](../../internal/eval/runner/generalization_environment.go)、同文件:59 | 拒绝另一靶场运行，按实测内存检查余量 | 基线、诊断等待 |
| `diagnosisMemory`、`diagnosisMemoryKey` | [generalization_environment.go:51](../../internal/eval/runner/generalization_environment.go)、同文件:22 | 当前上下文选择资源检查；旧 shop 路径保持原检查 | `waitDiagnosis` |
| `demoRecovery`、`demoRecoveryPath` | [generalization_run.go:31](../../internal/eval/runner/generalization_run.go)、同文件:53 | 为每轮保存独立原始/注入快照和身份 | 注入、恢复 |
| `readDemoRecord`、`createDemoRecord` | [generalization_run.go:61](../../internal/eval/runner/generalization_run.go)、同文件:79 | 核对同一故障时间；独占创建、同步成功后才注入 | 注入和续跑 |
| `demoBaseline`、`runGeneralization` | [generalization_run.go:101](../../internal/eval/runner/generalization_run.go)、同文件:129 | 查真实健康、冻结开关与开放期，按原安排推进 | run |
| `runDemoTrial`、`advanceDemoTrial` | [generalization_run.go:182](../../internal/eval/runner/generalization_run.go)、同文件:205 | 从数据库检查点推进，失败追加中断 | `runGeneralization` |
| `injectDemoRecord`、`activeDemoRecord` | [generalization_run.go:249](../../internal/eval/runner/generalization_run.go)、同文件:275 | 保存快照后改变官方开关；复核当前完整配置 | 轮次推进、周期守卫 |
| `checkDemoUnrestored`、`restoreDemoRecord` | [generalization_run.go:296](../../internal/eval/runner/generalization_run.go)、同文件:308 | 已恢复轮次不能接续旧诊断；仅恢复已知配置 | 活动检查、restore |
| `writeDemoRestoreProof` | [generalization_run.go:335](../../internal/eval/runner/generalization_run.go) | 另写恢复确认，不截断原快照 | `restoreDemoRecord` |
| `recordDemoEvaluator`、`coolDemoTrial` | [generalization_run.go:366](../../internal/eval/runner/generalization_run.go)、同文件:378 | 动作前复核冻结哈希；按数据库时间等待恢复 | 轮次和槽位执行 |
| `demoMatrix`、`demoSlotInput` | [generalization_run.go:401](../../internal/eval/runner/generalization_run.go)、同文件:459 | 固定配对输入，诊断/评分期间周期核对开关 | 诊断阶段 |
| `generalizationGuardKey`、`checkGeneralizationSummary`、`checkDemoSlot` | [generalization_run.go:42](../../internal/eval/runner/generalization_run.go)、同文件:45、481 | 异常先保存再停止收费，阶段 9 不自动补跑 | `executeSlot`、`demoMatrix` |
| `demoScenario`、`loadGeneralizationCases`、`decodeDemoScenario` | [generalization_scenario.go:33](../../internal/eval/runner/generalization_scenario.go)、同文件:48、69 | 读取固定目录中的外部场景，严格验证字段 | CLI |
| `demoScenario.gradingScenario` | [generalization_scenario.go:90](../../internal/eval/runner/generalization_scenario.go) | 仅把标准答案和里程碑交给原评分器 | `demoMatrix` |
| `demoHTTP`、`demoBase` | [generalization_scenario.go:95](../../internal/eval/runner/generalization_scenario.go)、同文件:251 | 有界本机实验 HTTP；不携带医生令牌 | 开关与指标操作 |
| `changeDemoFlag`、`readDemoFlags`、`writeDemoFlags` | [generalization_scenario.go:130](../../internal/eval/runner/generalization_scenario.go)、同文件:168、181 | 保留其他开关及 targeting，异步写后读回确认 | 注入、恢复和守卫 |
| `demoQuery`、`waitDemoQuery` | [generalization_scenario.go:203](../../internal/eval/runner/generalization_scenario.go)、同文件:233 | 用有限非空指标样本验证症状和恢复 | 轮次推进 |
| `newGraderForAccount` | [grading.go:60](../../internal/eval/runner/grading.go) | 在创建收费客户端前固定人民币账本视图 | 新旧评测装配 |
| `sourceHashUsing`（修改） | [manifest.go](../../internal/eval/runner/manifest.go) | 把全部系统说明书加入行为哈希 | 冻结与执行审计 |
| `validEvalTrial` | [eval_trials.go:46](../../internal/doctor/store/eval_trials.go) | 仅阶段 9 批次放行 OT 编号，保留原 S 编号规则 | `EvalTrial`、`RecordEvalExecution` |
| `Markdown`（修改） | [markdown.go:22](../../internal/eval/report/markdown.go) | 外部结果不再标成自建靶场 | export、已有报告调用方 |

本轮追加的采集质量与接入路径符号如下；不改变医生工具或Agent的接口。

| 名称或方法 | 文件 | 作用与调用方 |
|---|---|---|
| `QualityNormal`、`QualityAffected`、`QualityUnknown`、`CollectorAudit`、`Summary`新增字段 | [report/summary.go](../../internal/eval/report/summary.go) | 持久化窗口、容器、原件指纹和分信号计数，供runner与导出使用 |
| `CollectionQuality`、`requiresCollectionAudit` | [report/summary.go](../../internal/eval/report/summary.go) | 阶段9缺审计时保守判unknown，已知丢弃优先affected；阶段8不新增此门槛 |
| `collectorCommand`、`collectorIdentity`、`collectorWindow`、`collectorOutput.Write` | [generalization_collection.go](../../internal/eval/runner/generalization_collection.go) | 固定命令边界、容器身份、不可刷新起点与有界原始输出 |
| `runCollectorCommand`、`inspectCollector`、`collectorLineTime` | 同文件 | 参数数组只读Docker，单条命令有时限；只认Docker首个时间戳 |
| `beginCollectorWindow`、`writeCollectorEvidence`、`collectorWindow.finish` | 同文件 | 先独占写起点，再创建诊断；结束取原件、核身份与保留锚点，不覆写旧失败 |
| `parseCollectorWindow`、`verifyCollectorRaw`、`checkCollectorSummary`、`readCollectorSummary` | 同文件 | 丢弃和重试分计，重启/轮转/损坏/空原件阻止评分及下一收费槽位 |
| `terminalCollectorDetail`、`cancelUnattachedCollector`、`createCollectorTask` | 同文件 | 创建提交不确定或关联失败时先取消原ID，取终态，不新建补跑 |
| `executeDemoDiagnosis` | 同文件 | 阶段9专用执行与审计收尾；SQL、取证、取消使用独立清理时间预算 |
| `statistics`、Markdown质量分组与审计表、盲评候选过滤 | [statistics.go](../../internal/eval/report/statistics.go)、[markdown.go](../../internal/eval/report/markdown.go)、[calibration.go](../../internal/eval/report/calibration.go) | affected/unknown不进入正常成绩或盲评，费用和原报告保留 |
| `jaeger.New`、`Client.read` | [jaeger/client.go](../../internal/doctor/source/jaeger/client.go) | 严格校验配置中的安全路径，再拼接固定只读API；Search/Get与标签保留规则不变 |

### 4. 数据库变更

**无新迁移、表或索引。** 沿用 `eval_runs`、`eval_trials`、`eval_results`、`eval_interruptions`、`eval_execution_audits` 和费用审计设施。`validEvalTrial` 扩展 OT 编号的允许范围；批次必须使用阶段 9 前缀。旧阶段 8 的运行、结果和费用没有覆盖、删除或重评。

MCP 验证追加的是现有 `mcp_audit` 表中的真实调用记录，见 P9-10；不把新增审计行写成表结构变更。

独立部署新增的是专用状态卷，使用现有迁移初始化新医生库，没有新增迁移或业务表设计。切换前原阶段 9 支出与生成请求均为零，旧阶段 8 数据没有导入或修改；保留原数据库卷并停止原数据库，见 P9-27。当前独立页面不读取旧库，不能把界面里没有阶段 8 记录解释为删除了它们。

### 5. 接口变更

没有新增医生 HTTP 路由、gRPC 方法或 Agent 工具。`LogSource.Search(context.Context, LogQuery)` 及 `search_logs` 参数不变。新增宿主 CLI `eval generalization freeze|run|export|finish|restore`，官方开关的读取/修改接口仅在宿主评测器使用，不提供给医生。

Demo 独立回环端口：商店 18000、开关管理 18080、流量页面 18089、Prometheus 19090、Jaeger 19686、OpenSearch 19200、Grafana 13000。专用医生继续使用 8090；这些是部署配置值，不是测量结果。

### 6. 配置变更

| 配置项 | 含义 | 默认或当前固定值 |
|---|---|---|
| `OTEL_DEMO_ALERT_TOKEN` | 第二系统独立告警令牌，真实值仅环境注入 | 无生产默认；`.env.example` 只放演示假值 |
| `TEST_OPENSEARCH_URL/INDEX/FIELDS` | 显式开启真实查询；FIELDS 为实际五字段 JSON | 空，未设置 URL 时跳过 |
| `TEST_OPENSEARCH_SERVICE/KEYWORD` | 已存在日志的服务与关键词 | 空，启用时必填 |
| `TEST_OPENSEARCH_LEVEL` | 可选大写级别，验证真实源大小写差异 | 空；P9-21显式填 INFO |
| `TEST_OPENSEARCH_USERNAME/PASSWORD` | 安全插件开启时的只读账号 | 空，填写时必须成对 |
| target 的 `opensearch.url/index/fields` | 固定后端、指定索引和字段名 | [otel-demo.yaml](../../deploy/doctor/targets/otel-demo.yaml)使用 `otel-logs-*` 及实测字段；与 `loki` 互斥 |
| target 的 `opensearch.username_env/password_env` | 可选只读凭据的变量名 | 当前 Demo 未配置，保留两项成对和系统前缀校验 |
| `LLM_BUDGET_ACCOUNT/CURRENCY/TOTAL_BUDGET` | 专用医生固定新美元账户 | `phase-09-openai` / USD / 10；默认模式仍 Mock |
| `EVAL_COMPARISON_ACCOUNT/TOTAL_CNY` | 第二执行身份及评委的独立人民币额度 | `phase-09-deepseek` / 10；默认关闭真实对比 |
| `DOCTOR_TARGETS_DIR/DOCTOR_TARGET_ENV_FILE` | 通用部署的目标目录和外部系统凭据文件 | 显式指定，只挂所选目标；不在Compose写死SHOP变量 |
| `DOCTOR_HTTP_PORT/BIND_ADDR` | 独立医生宿主端口与监听地址 | 8090 / 127.0.0.1 |

阶段 9 的账户及上限使用现有变量配置，没有更改阶段 7、8 账户。美元统一称“按项目用量账本计”；Key 与其他用途共用，平台账单无法单独核对。授权额度、供应商余额和已经发生的费用分别记录，不能互相替代。

### 7. 部署变更

新增[隔离覆盖配置](../../deploy/opentelemetry/compose.phase09.yaml)、[专用医生](../../deploy/opentelemetry/doctor.phase09.yaml)、[版本文件](../../deploy/opentelemetry/versions.env)、[临时 Loki 对照采集器](../../deploy/opentelemetry/loki-verify.phase09.yaml)及[Collector 候选补丁](../../deploy/opentelemetry/otelcol-config-extras.phase09.yaml)。上游源码位于仓库外临时克隆，按固定提交加载 `compose.yaml`、`compose.full.yaml`、`compose.observability.yaml` 和 `compose.extras.yaml`；OpenSearch 来自 observability 层。

专用医生最初复用原 doctor-mysql，完成 P9-9 的同实例后端对照。用户追加独立部署后，改为[通用双服务](../../deploy/doctor/compose.yaml)加阶段 9 薄覆盖，创建新网络和状态卷，只加载 Demo 的外部接入目录。原医生与原数据库均停止并保留，阶段 8 原卷未挂给新实例。切换已登记目标不需重启；软件升级及接入文件修改的重新加载另作记录。

P9-25 的配置准备、构建至健康为 33.696456 秒，至登录和菜单为 33.8045 秒；源码和镜像已存在，不当作全新下载或完整接入工时。真实联调时仅新 pair 与 Demo 共 30 容器，目标仅 Demo、8 工具、无 SHOP 环境变量，日志查询 5 条；`up` 虽成功却是零序列。另读保留的 span 计数得到非空指标，见 P9-26、P9-29。两份原始结果分别保留，不将空结果当成成功取证。

Collector 补丁在原 `transform/sanitize_logs` 中保留 scope 规则，再重命名日志属性的 `http.` 前缀。原索引和容器身份保留；[上游配置差异](phase-09-upstream-config.diff)、[目标命名空间预检](phase-09-http-rename-precondition.json)和[独立规则复核](phase-09-http-rule-review.json)均已归档。

P9-14 的固定观察窗口为 05:04:46.894786114–05:14:47.000000000 UTC，实际 600.105213886 秒：最终 `Dropping data` 事件为 0、OpenSearch 日志 `dropped_items` 报告和为 0、mapper 错误为 0。预期具备 OTLP 日志接线的源 17/17 在窗口内有日志，改名后的 `http_request.method` 记录为 1884 条；见[固定窗口汇总](phase-09-log-fix-after.json)、[原始日志](phase-09-collector-after-fix.log)和[逐服务响应](phase-09-log-fix-services.json)。image-provider 的官方矩阵为 `logs=False`，不算入这些预期源；email 的 9 条中有 1 条合成标记精确匹配记录，不能冒充自然结账流量覆盖。同期另有 10 条 Jaeger traces 重试，因此不能称整个 Collector 零错误或端到端无静默丢失。

P9-15 的[字段对照](phase-09-log-field-comparison.json)确认医生五个字段的完整 mapping 逐节点 5/5 相同，[修复前 UUID](phase-09-log-fix-before.json)与[修复后 UUID](phase-09-opensearch-indices-after.json)一致，[固定版本配置验证](phase-09-log-config-validation.json)退出码为 0。这个固定窗口验证已经完成，完整[采集修复专项报告](phase-09-logging-fix.md)已获 Claude 审核通过；后续试跑异常见P9-41，不能把采集验证当作整个阶段完成。

扩展到 05:17:07.909028 UTC 的持续观察单列为 P9-18，见[扩展汇总与字节拼接说明](phase-09-log-fix-observation.json)。它没有替换首窗口口径。修复后原适配器的真实查询复验也通过（P9-16），见[集成输出](phase-09-opensearch-after-test.log)。

## 四、测试

### 1. 新增测试清单

| 测试函数 | 文件 | 验证内容 |
|---|---|---|
| `TestLogSourceContract` | [log_contract_test.go](../../internal/doctor/source/log_contract_test.go) | 同表分别验证 Loki/OpenSearch 的服务、级别、关键词、特殊字符、trace ID、时间、上限、空结果、组合条件和超时 |
| `TestConfigBoundary`、`TestResponseBoundary` | [client_test.go](../../internal/doctor/source/opensearch/client_test.go) | 索引/字段/凭据边界；嵌套与点键、缺字段、时间、结构化正文及异常响应 |
| `TestReadOnlyBoundary`、`TestKeywordJSONValues` | [client_test.go](../../internal/doctor/source/opensearch/client_test.go) | 固定搜索路径、只读账号、拒绝跳转、有界读取；关键词始终是 JSON 值 |
| `TestLevelCaseInsensitive` | [client_test.go](../../internal/doctor/source/opensearch/client_test.go) | 大写接口条件匹配来源小写或混合大小写；不同级别仍不命中，服务/trace条件不放宽 |
| `TestIntegrationSearchLogs` | [integration_test.go](../../internal/doctor/source/opensearch/integration_test.go) | 显式配置时对真实索引按服务及关键词查询 |
| `TestLogBackendConfig`、`TestLogBackendIsolation` | [opensearch_test.go](../../internal/doctor/target/opensearch_test.go) | 日志后端互斥、映射完整、同接口绑定不同系统 |
| `TestOpenSearchCredentialIsolation`、`TestOpenSearchCredentialsWiring` | [opensearch_test.go](../../internal/doctor/target/opensearch_test.go) | 环境变量及实际账号隔离，装配不触发探测 |
| `TestGeneralizationTrialIdentity` | [eval_generalization_test.go](../../internal/doctor/store/eval_generalization_test.go) | OT 编号仅属于阶段 9，旧编号边界保持 |
| `TestMarkdownTarget` | [markdown_test.go](../../internal/eval/report/markdown_test.go) | 外部导出来源与旧 shop 行为 |
| `TestDemoFlagMutation`、`TestDemoHTTP` | [generalization_test.go](../../internal/eval/runner/generalization_test.go) | 精确修改 targeting、原始配置不变，官方协议写后读回 |
| `TestDemoQuery`、`TestGeneralizationBoundaries` | [generalization_test.go](../../internal/eval/runner/generalization_test.go) | 不把空值/NaN 当健康；账户、靶场隔离及异常收费停点 |
| `TestDemoRestoreProof`、`TestGeneralizationCLI` | [generalization_test.go](../../internal/eval/runner/generalization_test.go) | 原快照不截断，恢复后不能续接旧注入；非法参数在凭据前拒绝 |
| `TestGeneralizationScenarioFiles`、`TestGeneralizationProfileHash` | [generalization_test.go](../../internal/eval/runner/generalization_test.go) | 实际候选 YAML 可加载；第二说明书变化进入冻结哈希 |

本轮补充测试清单：

| 测试函数 | 文件 | 验证内容 |
|---|---|---|
| `TestCollectorCounts`、`TestCollectorMalformed` | [generalization_collection_test.go](../../internal/eval/runner/generalization_collection_test.go) | 分信号丢弃计数、mapper与重试分开、损坏计数和时间戳拒绝 |
| `TestCollectorWindow`、`TestCollectorGate` | 同文件 | 原件、身份、重启/轮转、窗口完整性与评分前失败关闭 |
| `TestCollectorTerminal`、`TestCollectorAttachFailure`、`TestCollectorCommitUncertainty` | 同文件 | 终态不确定及原ID取消，不产生补跑任务 |
| `TestCollectorRetainedAnchor`、`TestIntegrationCollectorWindow` | 同文件 | tail/until空集回归、有界输出与显式真实只读证明；后者未启用时跳过 |
| `TestCollectionQuality`、`TestStatisticsCollectionQuality` | [summary_test.go](../../internal/eval/report/summary_test.go)、[statistics_test.go](../../internal/eval/report/statistics_test.go) | 质量保守分类与正常统计排除，不改变旧阶段样本 |
| `TestMarkdownCollectionQuality`、`TestCalibrationCollectionQuality` | [markdown_test.go](../../internal/eval/report/markdown_test.go)、[calibration_test.go](../../internal/eval/report/calibration_test.go) | 分表保留费用、审计明细及盲评候选排除 |
| `TestDeploymentPrefix`、`TestInvalidDeploymentPrefix` | [jaeger/client_test.go](../../internal/doctor/source/jaeger/client_test.go) | 根地址兼容、配置前缀实际Search/Get路径、危险路径和重定向拒绝 |

### 2. 运行命令和结果

全仓命令为 `gofmt -l cmd gen internal tools`、`go vet ./...`、`staticcheck ./...`、`go run ./tools/commentcheck ./...`、`go run ./tools/archcheck ./...`、`go test -race ./... -count=1 -timeout=180s -json`。P9-6 记录这轮全部通过，race 顶层通过/跳过为 291/47，含子例为 977/57，失败为 0；commentcheck 检查 441 个文件。运行进程清空全部 `TEST_*`、`LLM_API_KEY`、`DEEPSEEK_API_KEY`，设置 Mock 与 CGO，未格式化用户文件。详情见[检查统计](phase-09-checks.json)。

真实 OpenSearch 集成另行显式开启，P9-3 记录按服务返回 3 条、增加关键词返回 1 条；不把全仓轮次中跳过的集成算作通过。同一医生的真实 CLI 后端对照见 P9-9。新场景的健康查询实查见 P9-7，付费网络及时钟预检见 P9-12。

收尾发现冻结工具接受大写 INFO，而实际 frontend 使用小写 info。原精确 term 翻译漏查；新适配器仅对级别启用 `case_insensitive`，服务/trace不变。同一固定窗口实查从 0 到 1896 条、真实 Go 集成带 INFO 返回 10 条、新 Mock 医生返回 5 条，见 P9-21、P9-23。这个口径是 frontend 的 INFO 总量，不是 P9-14 的 HTTP method 计数。原 uppercase 夹具和未带级别的集成都没有覆盖差异；新增回归后重新完成全部检查，P9-22 为 race292 顶层/985含子例通过、47/57跳过、失败0，见[独立检查统计](phase-09-level-checks.json)。

初版提交 4596540 的[CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41583884321)已经通过，详见[逐步骤结果](phase-09-ci-first.json)和 P9-20。后续级别补丁提交2396d3e的[CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41585632149)也通过，见P9-30和[逐步骤结果](phase-09-ci-level.json)。新增独立部署CI随后已由P9-31验证通过；本轮逐诊断审计及Jaeger接线的CI见P9-46，锚点最小补丁使用自己的新提交CI，不用旧绿色结果替代。

独立部署增加了精确的新数据库隔离白名单和 Mock CI 启动检查。代码变更后全仓再次重验，P9-28 的[检查结果](phase-09-standalone-checks.json)为 race292 顶层/985含子例通过、47/57跳过、失败0；不覆盖前两轮检查。真实独立部署、数据源及资源证据见 P9-25–P9-29。

Collector 修复的固定窗口观察及字段验证已经完成，范围和例外见 P9-14、P9-15 及[专项报告](phase-09-logging-fix.md)。这个窗口没有最终 drop 或 mapper 错误，仍单列 Jaeger traces 的重试；不能将日志检查结论扩大为整个 Collector 没有错误。

采集修复 Claude 审核、OT01 免费注入恢复和 EventStream 摸底已完成。首个空诊断槽位见 P9-41，不算一份模型报告；用户后来授权使用新编号，产生了 P9-51 的真实报告。这是当时试跑未完成的状态；用户本轮已要求不再试跑，当前正式前仍缺历史健康全通过、真实双Mock全矩阵通过及正式E1/E2结果。[凭据、链接与冻结范围检查](phase-09-release-audit.json)保留其原审核时点，不作为本次收费结果或全阶段验收。

### 3. 第二次真实试跑与停止门槛

新批次 `phase09-pilot2-20261103-terra` 只运行 OT01/E1。P9-51 的[医生原报告与步骤](phase-09-pilot2-doctor-analysis.json)记录 `succeeded/voluntary`，5 轮、23 次查证工具及另 1 次 `submit_report`，输入 50,506、输出 1,323 token，耗时 31.070 秒。原组件 `ad` 与标准组件一致，仅记录组件 1/1；报告明确“具体触发原因尚未确定”，不能补写机制已查明或推导泛化准确率。EventStream 没有写入最终根因，是否影响内部假设或调查效率不能从这份轨迹量化，见[事实复核](phase-09-pilot2-doctor-analysis.md)。

| 本次外部试跑记录 | 数量或状态 | 可报告的结果 |
|---|---|---|
| Terra E1 原报告 | 1 份，succeeded/voluntary | 组件 1/1；机制与证据分缺失 |
| 本次 Collector 窗口 | normal | 最终 drop、OpenSearch 日志 dropped_items 报告和及 mapper 行/次数均为 0 |
| DeepSeek / rubric-v1.1 | 1 次 HTTP ok，评分 failed | 有效机制、证据评分 0 份，不能填零分 |
| 正式 E1/E2 | 均未开启 | 无正式成绩 |

P9-52 的窗口为 10:55:15.4018165–10:55:50.0016789 UTC，原始日志 32,799 字节；[审计](phase-09-pilot2-collector-audit.json)、[原始窗口](phase-09-pilot2-collector-window.log)与[指纹](phase-09-pilot2-evidence.json)均保留。normal 只说明这份窗口通过检查，不证明上游端到端零丢失，也不补回历史缺口。

P9-53 的[评分失败专项](phase-09-pilot2-judge-failure.md)定位到 `submit_grade.arguments` 内首项 `reason` 缺少闭合引号。外层响应合法，但冻结校验在 JSON 解码时返回“评分 JSON 格式不合法”。原文中的 `correct` 未被合法采纳，失败默认 `coverage=0` 不能当作实测零覆盖率；不修 JSON、不重评、不为满足放行条件追加一轮。[结果导出](phase-09-pilot2-results.md)保留原报告和失败费用。

P9-54 按新 run 的全部 attempt 账本计费：6 次请求均 HTTP ok、已结算，没有未知预留。医生 USD $0.027179、评委 CNY ¥0.014920，分别乘以 10 得到正式估算 $0.271790、¥0.149200；项目剩余分别 $9.972821、¥9.985080，13:10 的 DeepSeek 余额为 ¥5.66。美元按项目用量账本计，Key 与其他用途共用，平台账单无法单独核对；两币种不相加。尽管费用条件通过，[完整放行检查](phase-09-pilot2-formal-entry-gate.json)仍为 `allow_formal=false`：评委未成功，正常轮次也未完成。

原 flags 已恢复且证明 SHA 相符；首次保存的健康通过记录在 13:10，不能据此证明三分钟内恢复。trial 仍停在 diagnosing、`restored_at` 未填，紧急恢复没有倒填正常恢复与冷却。放行快照中的 runtime 检查为 false，是已经切回 Mock、关闭 comparison 的安全清理结果，不是额外模型失败。P9-55 记录 Demo 的 28 个容器全部 stop，仅 Mock 独立医生与新库运行；旧资源、原中断批次与账本继续保留。

pilot2当时的付费执行未修改Go、提示、手册、评分或场景条件；本轮新授权的eval与限定YAML变更另见P9-58/59。P9-56 的[文档归档 CI](phase-09-ci-archive.json)属于 `695d0d1`，与锚点运行代码的 CI 分开记录，不用它替代尚未完成的诊断验收。

## 五、工程验收结果

| 阶段 9 验收项 | 结果 | 证据 |
|---|---|---|
| 第二张结果表填入真实数据 | 试跑已单列；完整验收未通过 | P9-51–P9-54：1 份真实 E1 报告、1 份失败评分；机制与证据有效评分 0 份，正式批次未运行 |
| 记录接入耗时和卡点 | 部分完成，未通过完整验收 | 启动失败/修正计时见 P9-4、P9-5；Collector 缺口见 P9-11、P9-13。完整接入过程仍进行中，不把并行开发墙钟冒充纯接入耗时 |
| 同格式 `search_logs` 分别查询 shop/Loki 与 Demo/OpenSearch | 通过 | P9-9：同一医生实例分别返回 2、5 条；[两份真实 CLI 证据](phase-09-doctorctl-loki.json)、[OpenSearch 证据](phase-09-doctorctl-opensearch.json) |
| 新增适配器未修改 tools、agent | 本审核停点通过 | [完整 stat 与冻结目录空输出](phase-09-diff-stat.txt)；用户两处修改未暂存 |
| 追加独立部署与接入说明 | 已生成真实试跑报告；完整评测未完成 | P9-25–P9-29、P9-51：独立启动、接线与真实报告已留档；评分失败，未放行正式批次 |

采集修复、锚点修复及其检查均已留档，用户随后批准新编号试跑。pilot2当时停在评委协议失败与完整轮次未完成处，原记录不改；用户已批准新评分失败处置，当前停止在本轮历史健康门槛未全通过处。阶段 9 整体仍未验收，学习验收未勾选。新增台账为 **P9-1–P9-65**；其中 P9-51–P9-56 单列真实医生报告、本次采集窗口、失败评分、费用门槛、晚健康与收尾、CI。试跑记录不等于完整泛化成绩。

## 六、偏离计划之处

用户明确调整了阶段顺序及付费停点，见[决策记录](../decisions.md)末尾“阶段 9 恢复执行与费用授权”：阶段10学习验收不阻挡本阶段。早期安排不做完整Mock彩排、先小试跑再正式；本轮明确改为不再试跑，先已有历史全部通过，再真实双Mock全矩阵和CI通过才直接正式。模型费用账户和诊断冻结版本不变，单元/CI仍Mock。

执行中发现日志进入 OpenSearch 不代表日志完整。用户随后要求先量化、修复采集配置并交 Claude 审核，审核前不付费。用户现已明确转达 Claude 审核通过；新增每份报告诊断窗口内采集监测，任何丢弃均单列为数据受影响，不混入正常统计。补丁只处理 Collector 字段冲突，不修改诊断提示、工具、手册或评分；相关事实见[原始记录](../notes.md)末尾阶段 9 记录与[上游配置差异](phase-09-upstream-config.diff)。

上游 payment 健康探测在本机超过原时限，部署覆盖只延长相同 TCP 探测的等待；派生版本元数据也显式固定，见 P9-4、P9-5。首次失败保留。新场景的健康判断根据真实刷新周期调整观测窗口，见 P9-7；阶段 8 冻结场景不变。

Loki 对照最初返回空结果，原 Alloy 缺少创建时项目环境项。自动审批审查拒绝了停止原 Alloy 并替代其职责的动作，理由是原容器保护要求；被拒动作没有执行。采用其建议的安全替代：新增独立采集器完成查询，保留原 Alloy 配置。对照结束先停新增采集器，再按开工运行状态与启动证据，仅停止本轮临时启动的 shop 容器，恢复开工时未运行状态；该恢复审查通过。此处“恢复”指临时查询环境的回退，**Collector 补丁保持加载、固定窗口验证已完成，随后 Claude 已审核通过；未声称已回滚该补丁**。完整经过见[notes](../notes.md)末尾记录及 P9-9。

## 七、已知问题与遗留事项

1. **日志历史缺口保留。** P9-13 的最终修复前窗口出现 316 个 OpenSearch 最终失败请求，`dropped_items` 报告和为 6209，包含 2293 次单条 mapper 错误。Bulk 可能部分成功，6209 不能称为唯一丢失日志数，也不能按服务或场景平摊。frontend 控制台记录与 mapper 错误相差 3，未逐条关联，原因未知。Prometheus 的其他失败与重试另列，见[最终量化](phase-09-log-impact-final.json)。
2. **采集修复已获 Claude 审核通过。** 固定 600.105213886 秒窗口的最终 drop、日志条目报告和及 mapper 错误均为 0，预期 OTLP 日志源 17/17 有记录，五字段 mapping 5/5 不变；见 P9-14、P9-15 与[专项报告](phase-09-logging-fix.md)。image-provider 官方不启用日志，email 样本含受控合成触发，Jaeger traces 仍有重试。这些例外和历史缺口保留；首轮试跑在模型创建前被独立采集审计阻止，见P9-41。
3. **数据源降级。** 没有 PostgreSQL 检查、Valkey 直查和 pprof。otel-demo 暴露 8 项只读工具，shop 为 16 项（P9-8）。`kafka_consumer_lag`、`recent_changes` 随 Prometheus 注册，但外部系统可能没有它们预置的指标，空结果应如实解释。
4. **仅有试跑报告，没有有效机制与证据评分。** OT01 的真实 Terra E1 原报告已留档，评委内层 JSON 失败，正式 E1/E2 未开启；组件一致不能代替机制查明，医生自己也保留了未知触发原因。跨靶场差异仍无法量化。不针对场景修改提示、工具或手册补分；任何后续版本或追加收费先交用户决定。
5. **MCP 验证范围。** P9-10 是 Codex 经 PowerShell 实现 HTTP 传输，实际列工具并调用一次 `service_topology`；不是桌面原生 MCP 配置，也不是完整自主排障。它补充了 P7-8 的官方 Go SDK 验证，不能扩大成所有外部 Agent 客户端均已测试。
6. **旧学习材料交接给 Claude。** 阶段 0–8 学习材料保持未修改。若补说明，请在阶段 5 的接口/降级部分补 OpenSearch 作为后续实现的入口，在阶段 7 的 MCP 部分区分 Go SDK 与本阶段 Codex HTTP 调用，在阶段 8 的评测部分注明外部系统使用独立批次与币种账户；同时把旧称 `mcp_audits` 对照实际表名 `mcp_audit`。这些是交接建议，本阶段没有替 Claude 修改旧学习正文。
7. **开跑前新发现与边界。** 原 OT01 服务级健康判定被 flagd CLIENT EventStream 干扰，原失败保留；用户仅批准将两条健康/症状查询限定到 GetAds SERVER，症状文本、标准答案和里程碑逐字节不变，见[修订审计](phase-09-ot01-baseline-amendment.json)。免费注入恢复与 EventStream 摸底不创建诊断。Jaeger 固定版 API 有 `/jaeger/ui` 前缀，已在 source/jaeger 支持安全配置路径，真实 search/get 通过；没有改工具、Agent 或共享 HTTP 层。最新收尾 Demo 全部 stop，只有 Mock 独立医生与新库运行，shop 与旧医生库停止，原卷保留（P9-55）。用户两处修改不还原、不暂存、不提交。

## 八、注释自查结论

P9-6 的 commentcheck 已通过，检查 441 个非生成 Go 文件。新增 Go 包有 `doc.go`，新分组目录有 README，生产文件有文件说明，类型、函数和关键分支有中文解释。适配器中的 HTTP 时限、连接数、查询上限和字段长度使用具名常量；字段读取说明为什么要同时处理嵌套对象与点键，恢复说明解释为什么不能截断唯一快照。

开发中的自查发现过新文件清单漏项与长函数缺少内部说明，已补齐；不把机械检查通过等同于初学者已能理解。阶段 9 学习材料单独解释“同一个接口的第二个实现”。最终文档清单和本次新增部署说明仍须随收尾复核，不能据进行中报告勾工程验收。

## 九、学习指引

本阶段自己的[学习材料](../learn/phase-09-generalization.md)保留真实试跑与评分失败的开发回放。先读统一 `LogSource`，并排看 Loki 与 OpenSearch 的请求翻译，再读 target 装配和共享契约测试；随后阅读外部评测的快照、版本守卫、账户和恢复路径，最后对照日志冲突、医生原报告与评委原响应。

建议将接口/适配器阅读与实际查询练习分开安排，完整学习量沿用计划书约八小时的估计。学习验收由用户以后决定；本次报告可以用于核对原文与审计，尚无的正式成绩、机制分和证据分不能作为已测输出。

## 十、下一阶段预告

当前仍停在阶段9：历史健康门槛未100%，因此不启动真实双Mock矩阵或正式；保留真实E1原报告与失败评分，不修补或重评。正式 E1/E2 尚未开始，完整收尾与工程验收均未完成。阶段 10 已完成且合并，本轮不开始新阶段、不合并 `main`，也不勾学习验收。

## 本轮免费准备补记（付费结果另列）

Claude 要求每份诊断保留 Collector 时间窗、原始日志指纹、dropped_items 与 mapper 错误。采集审计从创建诊断前开始，到终态后结束，并核对容器身份与日志锚点；丢弃标为 affected，不能验证的标为 unknown，两类均停止后续收费并从正常统计、盲评候选排除，已有费用和报告仍保留。真实只读集成及全仓 Mock 检查见 P9-35、P9-39。

原 OT01 失败及同窗判定、新旧哈希保存在 P9-36；免费受控开关动作和恢复原件也保留。EventStream 的默认 deadline、其他服务、开关与恢复的关系单独调查，不根据噪声改医生提示、工具或评分。原未应用提案保持原样作为审批历史，批准后实际差异以修订审计为准。

免费摸底结果见 P9-37 与[背景专项报告](phase-09-eventstream-background.md)：当时四个实例的已捕获流符合上游默认 deadline，未检出这次开关造成提前结束或额外短流；有限采样与内存淘汰的边界均已记录。背景噪声保持原样。OT01 免费注入/恢复采样的实际值见 P9-36，不代替 runner 自己的症状/健康验证。P9-40 记录最终取消边界与 Mock 检查；那时收费任务尚未开始，新库零生成、两币种零支出。后来发生的实际请求与费用另列 P9-51–P9-54。

## 首轮试跑中断（P9-41）

`phase09-pilot-20261103-terra` 按获批清单冻结并注入OT01，三分钟内症状检查通过；进入diagnosing后，Collector起点锚点时间未能验证，审计程序在调用store.Evaluation前返回错误。原槽位未绑定诊断ID，生成与评分均零次，美元和人民币支出均零。中断批次、原开关及恢复文件、无诊断的导出表和数据库错误都保留，不覆盖或自动补跑。

随后通过紧急restore还原完整开关、读回比对，并独立查询实际healthy通过；原trial仍停在diagnosing、run为interrupted，没有把紧急恢复倒填成正常完成轮次。医生已切回Mock并关闭comparison，评测模式继续保留暂停自动诊断。原始run日志记录stdout；最终错误依据数据库显式utf8mb4及HEX导出，不冒充完整stderr捕获。首份默认字符集导出存在中文问号，也作为原件保留。

这是当时的开跑前审计设施异常，不是一次模型诊断。后来完成免费定位与最小修复，用户批准新编号试跑；原批次继续 interrupted，没有转换成成功报告。

补核P9-42表明当时逐一读取全部服务operations，命名CLIENT EventStream只有上述四个；其他flagd使用者与无命名流的边界在[覆盖清单](phase-09-eventstream-coverage.json)逐项保留。未观测不等于没有错误。

当时收费暂停期间仅修评测器的 Docker 锚点读取，不改 Collector 配置、医生策略或冻结评分。Mock 与实际只读回归后完整检查见 P9-44；当次 stop 与零支出收尾见 P9-45。修复前版本 CI 见 P9-46，不能冒充后续补丁 CI。用户后来批准新编号 `phase09-pilot2-20261103-terra`，继续复用同一阶段 9 账户及原预算，没有重开额度；实际支出另见 P9-51–P9-54。

P9-43 的同容器只读复现表明，本机 Docker 先取 tail 再按 until 过滤；最后一行晚于截止时，tail1 可返回空，而旧行仍保留。评测器只把起点查询改成 tail/all，并对真正空输出明确失败；全部原有输出、时间、身份和轮转限制保留。原失败命令的输出没有保存，因此只称重复机制复现，不声称已找回原次字节。[修复证据](phase-09-collector-anchor-fix.json)列出原始日志、窗口指纹和完整检查。这是新编号试跑之前的修复历史，不代替本次评分失败分析。

锚点修复运行代码 a7003ff 的完整 CI 已通过（P9-47，[远端运行](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41602337487)及[证据](phase-09-ci-anchor.json)）；P9-56 另列其后文档归档 CI。用户批准新编号后实际发生的报告、评分失败、费用门槛与收尾见 P9-51–P9-55。这是pilot2当时因评委未成功及完整轮次未完成而暂停的历史；当前按新指令因历史健康不全通过停止，原空槽位、原失败和已发生费用均不覆盖。

本次暂停归档的凭据/链接、原件与Git字节核对单列P9-57。运行行为文件与P9-44已检查版本相同；仅为新证据补充换行保留规则和文档，不新增模型请求。此停点的工程与学习验收继续未勾，正式批次未创建。

## 本轮用户限定修订与历史门槛（P9-58–P9-64）

已返回并可确认计量、但被冻结Check拒收的评分记录为failed，机制/覆盖继续缺失，不重评；同一新run前两份允许按原流程恢复冷却，累计第三份仍停止。原有限重试后API仍失败、预算/未知返回、医生执行、采集与时限等原停点保留。`grading.go`仅给阶段9后续结果加可选`response_returned`证明，旧stage8 JSON不添字段，pilot2不回填。冻结judge/scoring没有改变，不按错误文字猜运输结局。相关生产入口为`demoJudgeClient.Complete`、`demoJudgeResponse`、`checkDemoJudgements`，测试为[generalization_grading_test.go](../../internal/eval/runner/generalization_grading_test.go)。

OT03使用免费查询确认的`oteldemo.PaymentService/Charge` SERVER，并与其同一span调用量比较；其余判据保留。五YAML只改within为5m及OT03两选择器，非授权字段和其他字节全部相同，见[审计](phase-09-scenario-window-amendment.json)。仅阶段9恢复等待同步5m；原[2m]、模型描述、GT、里程碑、提示、工具、手册和评分保持原样。

新增[计时helper](../../internal/eval/runner/generalization_timing.go)及其[Mock测试](../../internal/eval/runner/generalization_timing_test.go)保存同进程的原单调锚点与首次通过UTC/poll数。恢复耗时不含其后冷却，重启缺原锚点则标未知，不拿DB时间差近似。新Mock装配只放宽eval对真实存在的primary检查；医生Mock仍不装真实comparison，评委仍为judge.Mock。本轮未启动真实双Mock矩阵，所以没有新症状或恢复用时实测可报告。

历史回放明确区分对应单flag off与严格五开关全off，排除窗口只依据开关证明与环境采样，不因业务调用增量零删点。对应off的OT03/04分别354/390、350/390；严格全off仍分别328/364、326/364。大部分失败点是[2m]调用增量0、错误增量0，不能当业务报错或据此证明没有真实交易；对应off的OT04另含两点checkout CLIENT POST错误，不能仅凭POST猜目标或根因。完整失败点、原始20响应和覆盖边界见[专项](phase-09-historical-baseline-replay.md)。这些是历史点谓词结果，不是5m等待成功率或医生准确率。

全部授权代码已完成免费工程检查。首轮staticcheck文案问题和后续0调用分级复核的前版证据全部保留；最终[verified检查](phase-09-preformal-verified-checks.json)通过，源码指纹、冻结范围与用户文件均已核对（P9-63）。用户要求“还有不到100%的先停下报告”，本轮因此停在历史门槛，没有以单元Mock或CI通过冒充真实全矩阵通过。没有任何新模型请求，旧费用与旧run均保留（P9-64）。阶段9依然未完成，不合并main、不勾学习验收。

发布前完整性、凭据、相对链接与Git字节核对见P9-65；源代码和限定YAML已与最终检查版本匹配，pilot2原文件与MCP原句均未变。当前仅推送可审阅修改和历史门槛失败记录，不以CI代替未通过的历史健康条件。
