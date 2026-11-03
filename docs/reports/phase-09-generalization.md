# 阶段 9 汇报书：陌生系统接入与第二个日志适配器

> **阶段9工程工作与本地工程验收已完成，最终归档分支CI随推送另行核验，Claude待审。学习验收未勾，不合并main。** 从 `main` 的 `231ee48` 切出 `phase-09-generalization`；用户于2026-11-02明确授权开始本阶段。阶段10已由Claude验收并合并，学习验收继续由用户本人决定。
>
> 正式批次 `phase09-formal-20261103T181204Z-terra` 已正常 completed：五场景、E1/E2各一次，十份真实Terra报告与十份有效DeepSeek评分，全部采集normal。E1（tools）与E2（tools+manual）各n=5，组件一致3/5，机制正确/部分/错误各2/2/1，coverage均0.75。正式费用USD $0.325033、CNY ¥0.196047，项目累计分别$0.352212、¥0.210967（P9-77–P9-83）。美元按项目用量账本计；Key与其他用途共用，平台账单无法单独核对，两币种不相加。
>
> 运行代码ac71f30的CI已通过；收尾已停止本轮Demo、保留独立医生和新库并恢复Mock。早期采集缺口、原pilot空槽中断、pilot2报告/失败评分/费用及历史健康门槛失败均保留。净总接入工时没有测量，只报告真实分段计时；不把并行开发墙钟当成净接入工时。

## 一、总览

### 1. 一句话总结

医生保持冻结诊断策略，新增OpenSearch日志实现并独立部署，完成陌生Demo的真实双Mock全流程及Terra E1/E2正式评测，结果与失败边界均已单独归档。

### 2. 做了什么

接入固定3.1.0 OpenTelemetry Demo，新增OpenSearch适配器，与Loki共用同一张契约测试表，不改医生tools/agent。通过同一医生的两后端实际查询和Codex HTTP MCP调用，验证配置装配与只读权限边界。按用户追加要求提供只含doctor和doctor-mysql的独立部署，目标目录和凭据外置，没有SHOP变量也能完成双Mock和真实Terra诊断。用冻结提示v8.0、手册v1.1及rubric-v1.1完成五场E1/E2正式批次；组件、机制、证据覆盖及费用分别统计，不混入阶段8、Mock或pilot。采集修复、异常停点和恢复过程都有原件，结果不佳的OT02/OT04/OT05照实保留，没有调参或重评。

### 3. 计划完成情况

| 计划书阶段9的“做”项 | 状态 | 最终证据与限制 |
|---|---|---|
| 独立Compose部署固定版本Demo，核对日志进入OpenSearch | 已完成 | P9-1、P9-3–P9-5、P9-14/15：资源、启动失败/修正、schema、采集修复与Claude审核均留档；正式每份诊断再核采集窗口。 |
| 第二个日志适配器；接口和上层工具不变 | 已完成 | `source/opensearch`直接HTTP固定索引搜索，支持全部结构化条件、字段映射、大小写级别及共享契约；真实只读查询通过。 |
| 接入配置、说明书与缺失源降级；同医生注册两个系统 | 已完成 | P9-8/9：shop与otel-demo并存，同医生分别查Loki2条/OpenSearch5条；两靶场轮流运行，目标切换不重启医生。 |
| 选择5–8个官方故障开关并标注答案 | 已完成 | OT01–OT05的官方文档和固定3.1.0源码见[场景目录](../../eval/scenarios/otel-demo/README.md)；授权选择器/等待修订有独立审计，不改GT或里程碑。 |
| 不调诊断策略运行E1、E2 | 已完成 | P9-72/75/77：真实双Mock与88项入口核验通过后，新formal完成5trial/10报告，E1/E2各n5，全部单模型、主动提交、normal、有效评分。 |
| 外部结果单列，与阶段8对应组比较 | 已完成 | [第二张结果表及离线比较](phase-09-results-summary.md)、[失败分析](phase-09-failure-analysis.md)；两个系统分母不同，描述性比较，不合并统计或因结果差改分。 |
| 如作通用诊断改进则两靶场重跑 | 未做（未触发） | 提示、工具、agent、手册和评分均未作通用诊断改进；部署/评测器保障变更不回填旧成绩。后续新版本另需用户授权。 |
| 用户追加：医生独立部署与接入自己的系统说明 | 已完成 | [通用双服务](../../deploy/doctor/compose.yaml)、[接入指南](../onboarding.md)，P9-24–P9-29及P9-72/77：无SHOP环境变量完成双Mock和正式Terra评测；旧库不迁移、不删除。 |

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

本阶段新增/修改文件与行数见[git diff --stat](phase-09-diff-stat.txt)，逐文件职责见下表及[代码地图](../代码地图.md)。stat 包含新增证据文件，排除其自身和用户保留的两处修改。

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

冻结目录空差异与用户文件保护已在P9-67的完整检查记录，提交前与最终stat再次核对；原stat命令及输出一并保留。上层工具定义、提示和 Agent 循环均未修改。

### 3. 类型与函数清单

下表来自 `go run ./tools/commentcheck -list ./...`，再对照工作区差异补充调用方。列出全部新增生产导出符号，以及接入、注入、持久化与并发守卫的关键内部符号；新增测试函数完整列在第四节。基线入口已随最终文件位置补齐；完整符号与包内文件清单结合代码地图复核。

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
| `demoBaseline`、`demoBaselinePhase` | [generalization_baseline.go:61](../../internal/eval/runner/generalization_baseline.go)、同文件:70 | 标记freeze/pre-inject/post-cool，保留原检查顺序 | freeze、run、冷却后复核 |
| `demoBaselineSources`、`demoBaselineObservation`、`runDemoBaseline` | [generalization_baseline.go:26](../../internal/eval/runner/generalization_baseline.go)、同文件:36、91 | 模拟依赖边界，独占保存整次及健康等待的单调证据，失败也保留 | `demoBaselinePhase`及Mock测试 |
| `waitDemoBaselineHealth`、`demoBaselineDeadline` | [generalization_baseline.go:150](../../internal/eval/runner/generalization_baseline.go)、同文件:199 | 五秒一轮、五分钟内同轮全部通过；期限覆盖查询，迟到true拒收 | `runDemoBaseline` |
| `runGeneralization` | [generalization_run.go](../../internal/eval/runner/generalization_run.go) | 按冻结安排推进整批，异常保留中断 | run |
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
| `TestDemoBaselineHealthy`、`TestDemoBaselineTimeout` | [generalization_baseline_test.go](../../internal/eval/runner/generalization_baseline_test.go) | 0/0后同轮五场景全部通过、不同轮不拼接；持续不通过在期限内停止 |
| `TestDemoBaselineQueryError`、`TestDemoBaselineFlags` | 同文件 | 查询错误立即停；开关不符不查询健康，原检查顺序保留；freeze元数据为空场景/0轮次 |
| `TestDemoBaselineQueryDeadline`、`TestDemoBaselineEvidence` | 同文件 | 查询本身受五分钟期限限制，晚到true拒收；失败证据和独占写失败均保留/停下 |

### 2. 运行命令和结果

最终运行代码对应P9-67：六项基线定向Mock race通过，全仓gofmt、vet、staticcheck、commentcheck（448文件）、archcheck与race全部通过；race为320顶层/1061含子例通过，48/58跳过，失败0。[完整检查及源码指纹](phase-09-baseline-wait-checks.json)清空所有TEST与付费Key、固定Mock，保护用户两文件及五个冻结目录。运行代码ac71f30的[CI 41679143036](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41679143036)通过（P9-69，[逐步骤原件](phase-09-ci-baseline-wait.json)），含独立医生无SHOP凭据、原shop集成和五轮冷启动。以下早期检查按原时点保留；最终文档归档分支CI待本次推送核验，不能拿旧CI替代。

全仓命令为 `gofmt -l cmd gen internal tools`、`go vet ./...`、`staticcheck ./...`、`go run ./tools/commentcheck ./...`、`go run ./tools/archcheck ./...`、`go test -race ./... -count=1 -timeout=180s -json`。P9-6 记录这轮全部通过，race 顶层通过/跳过为 291/47，含子例为 977/57，失败为 0；commentcheck 检查 441 个文件。运行进程清空全部 `TEST_*`、`LLM_API_KEY`、`DEEPSEEK_API_KEY`，设置 Mock 与 CGO，未格式化用户文件。详情见[检查统计](phase-09-checks.json)。

真实 OpenSearch 集成另行显式开启，P9-3 记录按服务返回 3 条、增加关键词返回 1 条；不把全仓轮次中跳过的集成算作通过。同一医生的真实 CLI 后端对照见 P9-9。新场景的健康查询实查见 P9-7，付费网络及时钟预检见 P9-12。

收尾发现冻结工具接受大写 INFO，而实际 frontend 使用小写 info。原精确 term 翻译漏查；新适配器仅对级别启用 `case_insensitive`，服务/trace不变。同一固定窗口实查从 0 到 1896 条、真实 Go 集成带 INFO 返回 10 条、新 Mock 医生返回 5 条，见 P9-21、P9-23。这个口径是 frontend 的 INFO 总量，不是 P9-14 的 HTTP method 计数。原 uppercase 夹具和未带级别的集成都没有覆盖差异；新增回归后重新完成全部检查，P9-22 为 race292 顶层/985含子例通过、47/57跳过、失败0，见[独立检查统计](phase-09-level-checks.json)。

初版提交 4596540 的[CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41583884321)已经通过，详见[逐步骤结果](phase-09-ci-first.json)和 P9-20。后续级别补丁提交2396d3e的[CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41585632149)也通过，见P9-30和[逐步骤结果](phase-09-ci-level.json)。新增独立部署CI随后已由P9-31验证通过；本轮逐诊断审计及Jaeger接线的CI见P9-46，锚点最小补丁使用自己的新提交CI，不用旧绿色结果替代。

独立部署增加了精确的新数据库隔离白名单和 Mock CI 启动检查。代码变更后全仓再次重验，P9-28 的[检查结果](phase-09-standalone-checks.json)为 race292 顶层/985含子例通过、47/57跳过、失败0；不覆盖前两轮检查。真实独立部署、数据源及资源证据见 P9-25–P9-29。

Collector 修复的固定窗口观察及字段验证已经完成，范围和例外见 P9-14、P9-15 及[专项报告](phase-09-logging-fix.md)。这个窗口没有最终 drop 或 mapper 错误，仍单列 Jaeger traces 的重试；不能将日志检查结论扩大为整个 Collector 没有错误。

采集修复Claude审核、免费注入恢复及EventStream摸底均已完成。P9-41的空槽不是模型报告，P9-51的pilot2报告及失败评分仍保留。历史健康回放曾失败并按约定停止；用户随后批准基线等待修订，实际双Mock、88项入口检查和正式E1/E2依次完成（P9-67–P9-83）。[早期发布审计](phase-09-release-audit.json)只证明其原时点，不冒充本次最终扫描或最终分支CI。

### 3. 第二次真实试跑与当时停止门槛（历史）

新批次 `phase09-pilot2-20261103-terra` 只运行 OT01/E1。P9-51 的[医生原报告与步骤](phase-09-pilot2-doctor-analysis.json)记录 `succeeded/voluntary`，5 轮、23 次查证工具及另 1 次 `submit_report`，输入 50,506、输出 1,323 token，耗时 31.070 秒。原组件 `ad` 与标准组件一致，仅记录组件 1/1；报告明确“具体触发原因尚未确定”，不能补写机制已查明或推导泛化准确率。EventStream 没有写入最终根因，是否影响内部假设或调查效率不能从这份轨迹量化，见[事实复核](phase-09-pilot2-doctor-analysis.md)。

| 本次外部试跑记录 | 数量或状态 | 可报告的结果 |
|---|---|---|
| Terra E1 原报告 | 1 份，succeeded/voluntary | 组件 1/1；机制与证据分缺失 |
| 本次 Collector 窗口 | normal | 最终 drop、OpenSearch 日志 dropped_items 报告和及 mapper 行/次数均为 0 |
| DeepSeek / rubric-v1.1 | 1 次 HTTP ok，评分 failed | 有效机制、证据评分 0 份，不能填零分 |
| 正式E1/E2（pilot2当时） | 当时未开启 | 后续正式另用新编号，见下节 |

P9-52 的窗口为 10:55:15.4018165–10:55:50.0016789 UTC，原始日志 32,799 字节；[审计](phase-09-pilot2-collector-audit.json)、[原始窗口](phase-09-pilot2-collector-window.log)与[指纹](phase-09-pilot2-evidence.json)均保留。normal 只说明这份窗口通过检查，不证明上游端到端零丢失，也不补回历史缺口。

P9-53 的[评分失败专项](phase-09-pilot2-judge-failure.md)定位到 `submit_grade.arguments` 内首项 `reason` 缺少闭合引号。外层响应合法，但冻结校验在 JSON 解码时返回“评分 JSON 格式不合法”。原文中的 `correct` 未被合法采纳，失败默认 `coverage=0` 不能当作实测零覆盖率；不修 JSON、不重评、不为满足放行条件追加一轮。[结果导出](phase-09-pilot2-results.md)保留原报告和失败费用。

P9-54 按新 run 的全部 attempt 账本计费：6 次请求均 HTTP ok、已结算，没有未知预留。医生 USD $0.027179、评委 CNY ¥0.014920，分别乘以 10 得到正式估算 $0.271790、¥0.149200；项目剩余分别 $9.972821、¥9.985080，13:10 的 DeepSeek 余额为 ¥5.66。美元按项目用量账本计，Key 与其他用途共用，平台账单无法单独核对；两币种不相加。尽管费用条件通过，[完整放行检查](phase-09-pilot2-formal-entry-gate.json)仍为 `allow_formal=false`：评委未成功，正常轮次也未完成。

原 flags 已恢复且证明 SHA 相符；首次保存的健康通过记录在 13:10，不能据此证明三分钟内恢复。trial 仍停在 diagnosing、`restored_at` 未填，紧急恢复没有倒填正常恢复与冷却。放行快照中的 runtime 检查为 false，是已经切回 Mock、关闭 comparison 的安全清理结果，不是额外模型失败。P9-55 记录 Demo 的 28 个容器全部 stop，仅 Mock 独立医生与新库运行；旧资源、原中断批次与账本继续保留。

pilot2当时的付费执行未修改Go、提示、手册、评分或场景条件；本轮新授权的eval与限定YAML变更另见P9-58/59。P9-56 的[文档归档 CI](phase-09-ci-archive.json)属于 `695d0d1`，与锚点运行代码的 CI 分开记录，不用那次文档CI替代当时尚未完成的诊断验收；最终正式结果另见下节。

### 4. 最终双Mock与真实正式批次

基线等待修订后，真实Demo双Mock完成五场、E1/E2各一次，11基线/5症状/5恢复均通过，10份Mock诊断和占位评分只作流程验证，不是模型成绩（P9-72）。原生样本在同一完整标签序列内得到spanmetrics相邻间隔59.995/59.997/59.999秒（最小/中位/最大），与固定Collector的60秒flush吻合。OT04焦点窗口业务SERVER成功计数先平台、下一批增加，支持稀疏已完成调用加约一分钟刷新造成等待；不能从中推算逐订单间隔、订单数或零采集延迟，见P9-73及[原生指标专项](phase-09-native-metrics.md)。因此健康基线保持原谓词，每五秒查询完整一轮，在五分钟内同轮五场景全过；不靠拼接各轮成功或把0/0直接当健康。

免费时钟/网络、预算及实际DeepSeek余额核验后，88项入口检查通过，新正式run `phase09-formal-20261103T181204Z-terra` 正常completed（P9-74–P9-78）。独立部署无SHOP依赖，十份Terra均succeeded/voluntary、单模型、采集normal，十份原DeepSeek评分ok，受影响/未知/混合/未完成/评分失败均为零。

| 系统及组 | 合格报告数 | 组件一致 | 原机制正确/部分/错误 | 有效评分 | coverage均值（n） |
|---|---:|---:|---|---:|---:|
| 阶段9 Demo，Terra E1（tools） | 5 | 3/5 | 2/2/1 | 5/5 | 0.75（5） |
| 阶段9 Demo，Terra E2（tools+manual） | 5 | 3/5 | 2/2/1 | 5/5 | 0.75（5） |
| 阶段8 shop，Terra E1 | 39 | 36/39 | 25/9/1 | 35/39，失败4 | 0.6762（35） |
| 阶段8 shop，Terra E2 | 39 | 36/39 | 26/9/0 | 35/39，失败4 | 0.6624（35） |

来源为P9-77/P9-81及[独立结果汇总](phase-09-results-summary.md)、[离线复算源](phase-09-results-summary.json)。阶段8为13场景×3，阶段9为另一系统5场景×1；不合并分母，不把DeepSeek医生或其他组混入。旧评分失败保持缺失；阶段8coverage只从原Markdown三位显示值复算，四位均值不能恢复原未舍入精度。组件比例描述性约由92.3%到60%，不能把差异归因于单一因素或外推生产准确率。

正式全部attempt费用为USD $0.325033（54次）与CNY ¥0.196047（10次），均ok/settled，无未知预留；项目累计USD $0.352212、CNY ¥0.210967，包含旧pilot2的真实费用，未重置账户或重评。两币种分别在原10上限内，美元按项目用量账本计；Key共用导致平台账单无法单独核对。详见P9-78及[原费用桥](phase-09-formal-snapshot.json)。

P9-80的[正式单调计时](phase-09-formal-timing.md)核对11基线、5症状、5恢复全部原件。症状与恢复都在300秒内；OT02恢复0.0411223秒仅是第一次healthy谓词观察，不是41ms业务修复。OT03 post-cool健康等待50.0641826秒与整次调用52.457908秒分列，不把资源检查或冷却混入。十份诊断各自的[正式Collector链](phase-09-formal-evidence/manifest.json)均核原字节/SHA：每份OpenSearch日志dropped_items与mapper错误计数均为0/0，mapper行数/出现次数及所有信号最终drop事件也均为0。normal只表示已保存诊断窗的审计正常，不证明端到端绝对无丢失。

## 五、工程验收结果

| 阶段9验收项 | 结果 | 证据与边界 |
|---|---|---|
| 第二张结果表填入真实数据并分开比较 | 通过 | P9-77/P9-81：[正式原表](phase-09-formal-results.md)、[对应Terra组比较](phase-09-results-summary.md)；10normal/10有效评分，E1/E2各n5，失败案例照录。 |
| 记录接入耗时和卡点 | 通过（分段实测；净总工时未测） | P9-4/5、P9-25、P9-70/80：[启动与失败历史](../../deploy/opentelemetry/README.md)、[正式计时](phase-09-formal-timing.md)。仅分段计时，不冒充纯接入总工时或性能基准。 |
| 同格式search_logs分别查询shop/Loki与Demo/OpenSearch | 通过 | P9-9：同医生分别2条、5条；[Loki原件](phase-09-doctorctl-loki.json)、[OpenSearch原件](phase-09-doctorctl-opensearch.json)。 |
| 新适配器不修改tools/agent，冻结内容保持 | 通过 | P9-67：[完整检查](phase-09-baseline-wait-checks.json)及[diff stat](phase-09-diff-stat.txt)；tools、agent、共享HTTP、judge、scoring对main差异为空。用户两修改保留且不提交。 |
| 独立部署与自己的系统接入说明 | 通过 | P9-25–29、P9-69、P9-72/77：无SHOP变量的[双服务](../../deploy/doctor/compose.yaml)实际完成Mock/真实Terra，[接入指南](../onboarding.md)列出凭据、只读权限和降级。 |
| 工程检查、注释、可复核原件 | 通过 | P9-67/69、P9-80–83：六项全仓检查、ac71完整CI、三层说明、[Mock归档](phase-09-dualmock-evidence/README.md)、[正式归档](phase-09-formal-evidence/README.md)均有证据。最终发布扫描与本次推送CI另核。 |

[最终完整性复核](phase-09-final-integrity.json)的14项检查已通过，运行Go/YAML及指纹与P9-67所验代码保持一致；新增原件的.gitattributes保护单列为非运行代码变化，不以此冒充重新跑Go检查。

阶段9本地工程自查已完成；运行代码ac71的CI已通过，最终文档归档分支CI待本次推送核验，Claude待审。不合并main，不替用户勾学习验收。原pilot、pilot2及历史失败均不覆盖；当前Demo已stop、独立医生恢复Mock（P9-79）。

本阶段新增数据台账为 **P9-1–P9-85**：P9-1–30记录资源、接入、采集修复与早期检查；P9-31–57记录独立部署、审计、锚点中断/修复、pilot2真实报告及失败评分；P9-58–66记录用户限定修订和历史门槛停点；P9-67–76记录最终基线检查、CI、真实双Mock、原生指标、预算/余额与新正式开跑；P9-77–83记录正式成绩/费用、资源收尾、原单调计时、阶段8对应组比较、失败分析与字节归档。P9-84记录最终凭据、链接与源码/原件审计，P9-85记录194份Git索引原始字节匹配和从真实暂存内容的离线复算。详见[发布扫描](phase-09-release-audit.json)、[源码与原件检查](phase-09-final-integrity.json)、[Git索引复算](phase-09-final-index-integrity.json)。

## 六、偏离计划之处

用户明确调整了阶段顺序及付费停点，见[决策记录](../decisions.md)末尾“阶段 9 恢复执行与费用授权”：阶段10学习验收不阻挡本阶段。早期安排不做完整Mock彩排、先小试跑再正式；本轮明确改为不再试跑，最初要求历史全部通过，历史未全通过时先停；随后用户批准只修改基线等待方式，CI、真实双Mock全矩阵、88项入口与预算/余额均通过后直接正式。模型费用账户和诊断冻结版本不变，单元/CI仍Mock。

执行中发现日志进入 OpenSearch 不代表日志完整。用户随后要求先量化、修复采集配置并交 Claude 审核，审核前不付费。用户现已明确转达 Claude 审核通过；新增每份报告诊断窗口内采集监测，任何丢弃均单列为数据受影响，不混入正常统计。补丁只处理 Collector 字段冲突，不修改诊断提示、工具、手册或评分；相关事实见[原始记录](../notes.md)末尾阶段 9 记录与[上游配置差异](phase-09-upstream-config.diff)。

上游 payment 健康探测在本机超过原时限，部署覆盖只延长相同 TCP 探测的等待；派生版本元数据也显式固定，见 P9-4、P9-5。首次失败保留。新场景的健康判断根据真实刷新周期调整观测窗口，见 P9-7；阶段 8 冻结场景不变。

Loki 对照最初返回空结果，原 Alloy 缺少创建时项目环境项。自动审批审查拒绝了停止原 Alloy 并替代其职责的动作，理由是原容器保护要求；被拒动作没有执行。采用其建议的安全替代：新增独立采集器完成查询，保留原 Alloy 配置。对照结束先停新增采集器，再按开工运行状态与启动证据，仅停止本轮临时启动的 shop 容器，恢复开工时未运行状态；该恢复审查通过。此处“恢复”指临时查询环境的回退，**Collector 补丁保持加载、固定窗口验证已完成，随后 Claude 已审核通过；未声称已回滚该补丁**。完整经过见[notes](../notes.md)末尾记录及 P9-9。

### 历史执行记录的阅读方式

以下按发生时点保留旧停点，出现“未完成”“未开启”“无支出”均指该条记录当时，不代表最终状态；最新正式结果见第四、五节。原pilot、pilot2和历史健康失败原件仍在，后来的成功不回填旧记录。

### 本轮免费准备补记（付费结果另列）

Claude 要求每份诊断保留 Collector 时间窗、原始日志指纹、dropped_items 与 mapper 错误。采集审计从创建诊断前开始，到终态后结束，并核对容器身份与日志锚点；丢弃标为 affected，不能验证的标为 unknown，两类均停止后续收费并从正常统计、盲评候选排除，已有费用和报告仍保留。真实只读集成及全仓 Mock 检查见 P9-35、P9-39。

原 OT01 失败及同窗判定、新旧哈希保存在 P9-36；免费受控开关动作和恢复原件也保留。EventStream 的默认 deadline、其他服务、开关与恢复的关系单独调查，不根据噪声改医生提示、工具或评分。原未应用提案保持原样作为审批历史，批准后实际差异以修订审计为准。

免费摸底结果见 P9-37 与[背景专项报告](phase-09-eventstream-background.md)：当时四个实例的已捕获流符合上游默认 deadline，未检出这次开关造成提前结束或额外短流；有限采样与内存淘汰的边界均已记录。背景噪声保持原样。OT01 免费注入/恢复采样的实际值见 P9-36，不代替 runner 自己的症状/健康验证。P9-40 记录最终取消边界与 Mock 检查；那时收费任务尚未开始，新库零生成、两币种零支出。后来发生的实际请求与费用另列 P9-51–P9-54。

### 首轮试跑中断（P9-41）

`phase09-pilot-20261103-terra` 按获批清单冻结并注入OT01，三分钟内症状检查通过；进入diagnosing后，Collector起点锚点时间未能验证，审计程序在调用store.Evaluation前返回错误。原槽位未绑定诊断ID，生成与评分均零次，美元和人民币支出均零。中断批次、原开关及恢复文件、无诊断的导出表和数据库错误都保留，不覆盖或自动补跑。

随后通过紧急restore还原完整开关、读回比对，并独立查询实际healthy通过；原trial仍停在diagnosing、run为interrupted，没有把紧急恢复倒填成正常完成轮次。医生已切回Mock并关闭comparison，评测模式继续保留暂停自动诊断。原始run日志记录stdout；最终错误依据数据库显式utf8mb4及HEX导出，不冒充完整stderr捕获。首份默认字符集导出存在中文问号，也作为原件保留。

这是当时的开跑前审计设施异常，不是一次模型诊断。后来完成免费定位与最小修复，用户批准新编号试跑；原批次继续 interrupted，没有转换成成功报告。

补核P9-42表明当时逐一读取全部服务operations，命名CLIENT EventStream只有上述四个；其他flagd使用者与无命名流的边界在[覆盖清单](phase-09-eventstream-coverage.json)逐项保留。未观测不等于没有错误。

当时收费暂停期间仅修评测器的 Docker 锚点读取，不改 Collector 配置、医生策略或冻结评分。Mock 与实际只读回归后完整检查见 P9-44；当次 stop 与零支出收尾见 P9-45。修复前版本 CI 见 P9-46，不能冒充后续补丁 CI。用户后来批准新编号 `phase09-pilot2-20261103-terra`，继续复用同一阶段 9 账户及原预算，没有重开额度；实际支出另见 P9-51–P9-54。

P9-43 的同容器只读复现表明，本机 Docker 先取 tail 再按 until 过滤；最后一行晚于截止时，tail1 可返回空，而旧行仍保留。评测器只把起点查询改成 tail/all，并对真正空输出明确失败；全部原有输出、时间、身份和轮转限制保留。原失败命令的输出没有保存，因此只称重复机制复现，不声称已找回原次字节。[修复证据](phase-09-collector-anchor-fix.json)列出原始日志、窗口指纹和完整检查。这是新编号试跑之前的修复历史，不代替本次评分失败分析。

锚点修复运行代码 a7003ff 的完整 CI 已通过（P9-47，[远端运行](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41602337487)及[证据](phase-09-ci-anchor.json)）；P9-56 另列其后文档归档 CI。用户批准新编号后实际发生的报告、评分失败、费用门槛与收尾见 P9-51–P9-55。这是pilot2当时因评委未成功及完整轮次未完成而暂停的历史；当时按新指令因历史健康不全通过停止，原空槽位、原失败和已发生费用均不覆盖。

本次暂停归档的凭据/链接、原件与Git字节核对单列P9-57。运行行为文件与P9-44已检查版本相同；仅为新证据补充换行保留规则和文档，不新增模型请求。该历史停点的工程与学习验收未勾，正式批次尚未创建；后续正式完成情况见第五节。

### 本轮用户限定修订与历史门槛（P9-58–P9-64）

已返回并可确认计量、但被冻结Check拒收的评分记录为failed，机制/覆盖继续缺失，不重评；同一新run前两份允许按原流程恢复冷却，累计第三份仍停止。原有限重试后API仍失败、预算/未知返回、医生执行、采集与时限等原停点保留。`grading.go`仅给阶段9后续结果加可选`response_returned`证明，旧stage8 JSON不添字段，pilot2不回填。冻结judge/scoring没有改变，不按错误文字猜运输结局。相关生产入口为`demoJudgeClient.Complete`、`demoJudgeResponse`、`checkDemoJudgements`，测试为[generalization_grading_test.go](../../internal/eval/runner/generalization_grading_test.go)。

OT03使用免费查询确认的`oteldemo.PaymentService/Charge` SERVER，并与其同一span调用量比较；其余判据保留。五YAML只改within为5m及OT03两选择器，非授权字段和其他字节全部相同，见[审计](phase-09-scenario-window-amendment.json)。仅阶段9恢复等待同步5m；原[2m]、模型描述、GT、里程碑、提示、工具、手册和评分保持原样。

新增[计时helper](../../internal/eval/runner/generalization_timing.go)及其[Mock测试](../../internal/eval/runner/generalization_timing_test.go)保存同进程的原单调锚点与首次通过UTC/poll数。恢复耗时不含其后冷却，重启缺原锚点则标未知，不拿DB时间差近似。新Mock装配只放宽eval对真实存在的primary检查；医生Mock仍不装真实comparison，评委仍为judge.Mock。截至P9-58–P9-64，当时未启动真实双Mock矩阵，没有该轮症状或恢复实测；后续新增计时单列P9-72、P9-80。

历史回放明确区分对应单flag off与严格五开关全off，排除窗口只依据开关证明与环境采样，不因业务调用增量零删点。对应off的OT03/04分别354/390、350/390；严格全off仍分别328/364、326/364。大部分失败点是[2m]调用增量0、错误增量0，不能当业务报错或据此证明没有真实交易；对应off的OT04另含两点checkout CLIENT POST错误，不能仅凭POST猜目标或根因。完整失败点、原始20响应和覆盖边界见[专项](phase-09-historical-baseline-replay.md)。这些是历史点谓词结果，不是5m等待成功率或医生准确率。

全部授权代码已完成免费工程检查。首轮staticcheck文案问题和后续0调用分级复核的前版证据全部保留；最终[verified检查](phase-09-preformal-verified-checks.json)通过，源码指纹、冻结范围与用户文件均已核对（P9-63）。用户要求“还有不到100%的先停下报告”，本轮因此停在历史门槛，没有以单元Mock或CI通过冒充真实全矩阵通过。没有任何新模型请求，旧费用与旧run均保留（P9-64）。该历史停点阶段9尚未完成，不合并main、不勾学习验收。

发布前完整性、凭据、相对链接与Git字节核对见P9-65；源代码和限定YAML已与最终检查版本匹配，pilot2原文件与MCP原句均未变。当次仅推送可审阅修改和历史门槛失败记录，不以CI代替未通过的历史健康条件。

运行代码提交`0d221f7`的[完整CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41663273057)已通过，逐步骤结果见[CI原件](phase-09-ci-preformal.json)及P9-66。独立医生、原shop集成与五轮冷启动回归均成功。当次归档只追加CI证据和文档，不改运行代码；当时历史门槛未全通过，因此没有继续真实双Mock矩阵或付费正式批次。

### 基线等待修订（P9-67）

原历史回放未全过后曾按约定停止，原统计与失败点不改。用户复核后仅批准改变健康基线的等待方式：每五秒重查一轮，五分钟内同一轮所有场景通过；未改变原健康谓词、场景、提示、工具、手册、judge或scoring。原memory→flags→health→fault检查顺序保留，配置不符或查询错误立即停止。新增[基线实现](../../internal/eval/runner/generalization_baseline.go)及[Mock测试](../../internal/eval/runner/generalization_baseline_test.go)覆盖0/0后通过、持续失败、查询错误、开关不符、迟到结果和独占证据失败。

整次调用耗时包含资源检查、开关和故障开放期；健康轮询另有单调耗时字段，对应五分钟上限。freeze没有场景轮次，证据明确空场景和0轮次；同场景注入前和冷却后分别保存，失败也不覆盖。源码、五份未改YAML与本地检查见[限定审计](phase-09-baseline-wait-amendment.json)和[全仓检查](phase-09-baseline-wait-checks.json)。该修订时点仍需等待CI、真实双Mock及原生采样核验；随后P9-69、P9-72、P9-73分别完成这些核验，正式结果另列P9-77–P9-83。不以配置默认值或用户推测代替本轮原生观测。

## 七、已知问题与遗留事项

1. **日志历史缺口保留。** P9-13的修复前窗口有316个OpenSearch最终失败请求，dropped_items报告和6209、mapper错误出现2293次；Bulk可能部分成功，不能称6209条唯一丢失日志，更不能按服务/场景平摊。frontend记录差3未逐条关联，原因未知，见[最终量化](phase-09-log-impact-final.json)。修复、Claude审核及正式窗口审计不会抹去这段历史。
2. **泛化结果有限且并未全部查明机制。** E1/E2各五份，仅组件3/5、机制2/2/1。OT02精确`cart->valkey-cart`与GT `cart`不相等，且漏了EmptyCart专用badhost存储；OT04两组`unknown`且实际get_trace截断；OT05树未字节截断但缺商品OLJCESPC7Z与关键状态。原评分与里程碑裁量均保留，见P9-82及[失败分析](phase-09-failure-analysis.md)。DeepSeek是自动评委，不是独立人工金标准；不能声称检索组普遍更优。
3. **采集与查询可见范围不同。** P9-14/15的17个OTLP日志源和五字段验证有明确窗口；image-provider官方logs=False，email包含合成记录，Jaeger仍有重试。正式十窗normal只是未报告最终drop/mapper，不证明端到端零丢失，也不保证工具摘要展示全部原始字段。EventStream进入过OT01及OT05/E1的工具证据和后续请求历史，十份终报及可见LLM响应没有将它列成根因，也没有明确排除；不能推内部无干扰。
4. **数据源降级与独立部署边界。** Demo没有PostgreSQL检查、Valkey直查或pprof，暴露8项只读工具，shop为16项（P9-8）；通用lag/changes查询可能缺预置指标，空值不能冒充取证。独立新库不迁移旧阶段8数据；`up`接线曾成功但0序列，P9-29另有非空span指标，原结果分别保留。
5. **接入净总工时未测。** 启动、部署、故障观察等只有各自起止和实测范围；没有连续记录纯接入劳动时间，不能把并行开发墙钟或单次启动当成完整接入工时。正式恢复0.0411223秒也只是一阶观察结果，不是业务修复性能。
6. **MCP与旧学习材料边界。** P9-10是Codex通过PowerShell实现HTTP传输，实际列工具并调用service_topology，不是桌面原生MCP或完整自主排障；P7-8仍是官方Go SDK。阶段0–8学习材料本次不动；后续Claude可在阶段5接口/降级和阶段8结果来源处补向本阶段的入口，不倒写旧实验结论。用户两修改不还原、不暂存、不提交。
7. **后续工作需另行授权。** 针对呈现截断、证据字段或组件粒度的通用改进会形成新版本，应按计划先获批，再在两个系统重跑并记录费用；本阶段未实施这些改进。原pilot2失败评分不补评。收尾只stop本轮Demo，原卷、20测试项目/138测试卷及QQ保持保留与保护，见P9-79。

## 八、注释自查结论

最终P9-67的commentcheck检查448个非生成Go文件通过，包文件清单与[代码地图](../代码地图.md)已同步。目录层有doc.go/README，生产文件有问题与职责说明，类型、函数及关键分支用中文解释；HTTP和查询边界使用具名常量，说明点键/嵌套字段兼容、只指定索引、恢复快照不可截断及单调时间不可补造的原因。

开发中发现过文件清单漏项、长函数说明不足及ST1005文案问题，修正后的检查与原失败均留档。最终基线文件解释“同轮全部健康”与“整次/仅健康等待”的差别，六项Mock测试覆盖失败路径；新原件目录均有README和来源映射。人工核对三层说明与学习材料的“同一个接口的第二个实现”、真实失败回放已完成。机械检查不代替用户理解；最终凭据、相对链接、原件与Git字节核对见第五节发布审计，独立列明范围。

## 九、学习指引

阅读[阶段9学习材料](../learn/phase-09-generalization.md)，先从统一LogSource并排看Loki/OpenSearch的请求翻译、target装配及共用契约测试；再按[接入指南](../onboarding.md)理解独立医生、外置凭据、只读权限和缺失源降级。随后读宿主runner的快照/版本/账户/恢复与基线等待，最后对照[正式成绩](phase-09-results-summary.md)、[失败分析](phase-09-failure-analysis.md)及[原始计时](phase-09-formal-timing.md)，区分组件、机制、coverage和观察时长。

完整学习量仍沿用计划书约八小时的估计，并非实测学习耗时。真实回放包含采集缺口、锚点空槽、pilot2失败评分、历史门槛暂停、双Mock与最终正式，不能写成一路成功的优化故事。学习验收只由用户本人决定，本报告不代勾。

## 十、下一阶段预告

阶段9工程工作与本地验收已完成，收尾提交由分支CI核验，随后等待Claude验收，然后由用户学习和决定是否合并。阶段10此前已经完成并合并，没有新的阶段要自动开始；本轮不合并main、不勾学习验收，也不追加模型调用。若用户以后选择处理泛化失败，再先批准新版本范围、两个靶场重跑与预算。
