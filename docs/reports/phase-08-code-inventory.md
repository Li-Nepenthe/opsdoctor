# 阶段 8 代码清单

这是[阶段汇报书](phase-08-eval.md)的明细草稿。文件表以本次 S08 基础设施修正的工作区与 main 差异为准；正式运行结束后继续更新。用户保留的两处修改不纳入本表。

## 文件清单

| 文件 | 提交版本行数 | 增加/删除 | 作用 |
|---|---:|---:|---|
| [.env.example](../../.env.example) | 173 | +21/−1 | 全部密码都是本机演示假值，不可用于线上；复制为 .env，真实值只存在忽略的文件或环境变量中。 |
| [.gitignore](../../.gitignore) | 17 | +2/−0 | 本机配置和凭据永远不进入这个公开仓库；示例文件只放演示假值。 |
| [README.md](../../README.md) | 559 | +63/−0 | OpsDoctor |
| [cmd/README.md](../../cmd/README.md) | 34 | +2/−0 | cmd：程序入口 |
| [cmd/doctor/main.go](../../cmd/doctor/main.go) | 21 | +1/−1 | 本文件：医生服务入口，只负责把启动结果变成进程退出码。 |
| [cmd/eval/main.go](../../cmd/eval/main.go) | 22 | +22/−0 | eval 是宿主机评测入口；实验配置、任务保存和结果导出在 internal/eval 中实现。 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 520 | +14/−2 | 部署或实验配置，具体差异见汇报书 |
| [deploy/doctor/targets/shop.profile.md](../../deploy/doctor/targets/shop.profile.md) | 104 | +1/−0 | shop 的系统说明书 |
| [docs/README.md](../../docs/README.md) | 20 | +1/−0 | docs：项目文档 |
| [docs/decisions.md](../../docs/decisions.md) | 617 | +161/−0 | 设计决定 |
| [docs/learn/phase-08-eval.md](../../docs/learn/phase-08-eval.md) | 412 | +412/−0 | 阶段 8 学习材料：怎样检查医生有没有查对 |
| [docs/notes.md](../../docs/notes.md) | 1627 | +235/−0 | 实测记录 |
| [docs/reports/README.md](../../docs/reports/README.md) | 53 | +35/−0 | docs/reports：阶段汇报书 |
| [docs/reports/phase-08-baseline-checks.json](../../docs/reports/phase-08-baseline-checks.json) | 82 | +82/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-after-sync.json](../../docs/reports/phase-08-clock-after-sync.json) | 234 | +234/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-amendment.json](../../docs/reports/phase-08-clock-amendment.json) | 55 | +55/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-events.jsonl](../../docs/reports/phase-08-clock-events.jsonl) | 5 | +5/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-guard-v2.json](../../docs/reports/phase-08-clock-guard-v2.json) | 7 | +7/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-paired-readings.jsonl](../../docs/reports/phase-08-clock-paired-readings.jsonl) | 61 | +61/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-policy-v3-during-network.json](../../docs/reports/phase-08-clock-policy-v3-during-network.json) | 7 | +7/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-policy-v3.json](../../docs/reports/phase-08-clock-policy-v3.json) | 7 | +7/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-preflight.json](../../docs/reports/phase-08-clock-preflight.json) | 186 | +186/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-clock-stability.jsonl](../../docs/reports/phase-08-clock-stability.jsonl) | 60 | +60/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-code-inventory.md](../../docs/reports/phase-08-code-inventory.md) | 676 | +676/−0 | 阶段 8 代码清单 |
| [docs/reports/phase-08-deepseek-network.jsonl](../../docs/reports/phase-08-deepseek-network.jsonl) | 121 | +121/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-eval.md](../../docs/reports/phase-08-eval.md) | 111 | +111/−0 | 阶段 8 汇报书：评测体系（进行中） |
| [docs/reports/phase-08-failure-analysis.md](../../docs/reports/phase-08-failure-analysis.md) | 149 | +149/−0 | 阶段 8 失败与评分疑点（持续追加） |
| [docs/reports/phase-08-formal-clock-interruption.json](../../docs/reports/phase-08-formal-clock-interruption.json) | 60 | +60/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-formal-grade-failure.jpg](../../docs/reports/phase-08-formal-grade-failure.jpg) | 二进制 | +-/−- | 真实浏览器中的结果页预览 |
| [docs/reports/phase-08-formal-manifest.json](../../docs/reports/phase-08-formal-manifest.json) | 1826 | +1826/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-formal-preview.jpg](../../docs/reports/phase-08-formal-preview.jpg) | 二进制 | +-/−- | 真实浏览器中的结果页预览 |
| [docs/reports/phase-08-formal-results.md](../../docs/reports/phase-08-formal-results.md) | 269 | +269/−0 | 评测记录 phase08-formal-20261029 |
| [docs/reports/phase-08-lastcheck3-results.md](../../docs/reports/phase-08-lastcheck3-results.md) | 39 | +39/−0 | > 正式评测前的完整检查，评分接线未通过。五份医生报告和费用均保留，不能纳入正式矩阵。 |
| [docs/reports/phase-08-lastcheck4-results.md](../../docs/reports/phase-08-lastcheck4-results.md) | 39 | +39/−0 | > 正式评测前检查，评分回复重复了程序规则项，严格校验未通过。报告、评分失败和全部费用原样保留。 |
| [docs/reports/phase-08-lastcheck5-results.md](../../docs/reports/phase-08-lastcheck5-results.md) | 37 | +37/−0 | 评测记录 phase08-lastcheck5-20261029 |
| [docs/reports/phase-08-memory-s03-third.json](../../docs/reports/phase-08-memory-s03-third.json) | 26 | +26/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-memory-s05-second.json](../../docs/reports/phase-08-memory-s05-second.json) | 102 | +102/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-memory-s06-resumed.json](../../docs/reports/phase-08-memory-s06-resumed.json) | 102 | +102/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-memory-s07-first.json](../../docs/reports/phase-08-memory-s07-first.json) | 102 | +102/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-network-after-sync.jsonl](../../docs/reports/phase-08-network-after-sync.jsonl) | 121 | +121/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-network-after-timesyncd-stop.jsonl](../../docs/reports/phase-08-network-after-timesyncd-stop.jsonl) | 121 | +121/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-network-preflight.jsonl](../../docs/reports/phase-08-network-preflight.jsonl) | 110 | +110/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-pilot-results.md](../../docs/reports/phase-08-pilot-results.md) | 11 | +11/−0 | 评测试跑 phase08-pilot-20261029 |
| [docs/reports/phase-08-pilot.md](../../docs/reports/phase-08-pilot.md) | 69 | +69/−0 | 阶段 8 第一个审核节点：手册与试跑 |
| [docs/reports/phase-08-preflight.md](../../docs/reports/phase-08-preflight.md) | 94 | +94/−0 | 阶段 8：正式开跑前检查 |
| [docs/reports/phase-08-results-preview.png](../../docs/reports/phase-08-results-preview.png) | 二进制 | +-/−- | 真实浏览器中的结果页预览 |
| [docs/reports/phase-08-s02-resume-observation.json](../../docs/reports/phase-08-s02-resume-observation.json) | 104 | +104/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s03-baseline-check.json](../../docs/reports/phase-08-s03-baseline-check.json) | 84 | +84/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s03-grade-failure.json](../../docs/reports/phase-08-s03-grade-failure.json) | 47 | +47/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s06-recovery.json](../../docs/reports/phase-08-s06-recovery.json) | 355 | +355/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s06-second-recovery.json](../../docs/reports/phase-08-s06-second-recovery.json) | 355 | +355/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s06-third-baseline.json](../../docs/reports/phase-08-s06-third-baseline.json) | 153 | +153/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/reports/phase-08-s08-baseline-interruption.json](../../docs/reports/phase-08-s08-baseline-interruption.json) | 33 | +33/−0 | 实测时间、请求状态或采样结果，不含凭据 |
| [docs/runbooks/README.md](../../docs/runbooks/README.md) | 23 | +23/−0 | 通用排障手册 |
| [docs/runbooks/cache.md](../../docs/runbooks/cache.md) | 11 | +11/−0 | 缓存排查：比较请求、未命中和回源 |
| [docs/runbooks/changes-and-safety.md](../../docs/runbooks/changes-and-safety.md) | 11 | +11/−0 | 变更和不可信内容：线索不能直接当命令 |
| [docs/runbooks/database.md](../../docs/runbooks/database.md) | 11 | +11/−0 | 数据库排查：先分清在等连接，还是在执行 |
| [docs/runbooks/messaging.md](../../docs/runbooks/messaging.md) | 11 | +11/−0 | 消息处理排查：把生产、确认和消费分开看 |
| [docs/runbooks/request-latency.md](../../docs/runbooks/request-latency.md) | 11 | +11/−0 | 接口变慢时，从请求路径找时间花在哪里 |
| [docs/runbooks/runtime.md](../../docs/runbooks/runtime.md) | 11 | +11/−0 | 进程和并发排查：增长不一定是泄漏 |
| [docs/runbooks/time-and-evidence.md](../../docs/runbooks/time-and-evidence.md) | 11 | +11/−0 | 先把时间和证据对齐 |
| [docs/代码地图.md](../../docs/代码地图.md) | 496 | +90/−1 | OpsDoctor 代码地图 |
| [docs/数据台账.md](../../docs/数据台账.md) | 564 | +211/−0 | 数据台账 |
| [eval/scenarios/README.md](../../eval/scenarios/README.md) | 27 | +5/−1 | scenarios：一份文件描述一次实验 |
| [eval/scenarios/S01.yaml](../../eval/scenarios/S01.yaml) | 24 | +4/−0 | 部署或实验配置，具体差异见汇报书 |
| [eval/scenarios/S02.yaml](../../eval/scenarios/S02.yaml) | 22 | +3/−0 | 部署或实验配置，具体差异见汇报书 |
| [eval/scenarios/S11.yaml](../../eval/scenarios/S11.yaml) | 18 | +18/−0 | 部署或实验配置，具体差异见汇报书 |
| [eval/scenarios/S12.yaml](../../eval/scenarios/S12.yaml) | 24 | +24/−0 | 部署或实验配置，具体差异见汇报书 |
| [internal/README.md](../../internal/README.md) | 28 | +1/−0 | internal：项目的全部实现代码 |
| [internal/chaos/action/doc.go](../../internal/chaos/action/doc.go) | 23 | +2/−0 | Package action 负责真正改变实验环境。 |
| [internal/chaos/action/probe.go](../../internal/chaos/action/probe.go) | 49 | +49/−0 | 本文件：用正常商品查询把不可信文字带进业务访问日志。 |
| [internal/chaos/action/probe_test.go](../../internal/chaos/action/probe_test.go) | 41 | +41/−0 | 本文件：验证安全实验只产生普通 GET，不接受任意主机或执行文字。 |
| [internal/chaos/action/runner.go](../../internal/chaos/action/runner.go) | 104 | +7/−0 | 本文件：把已检查的场景动作交给真正的外部系统执行。 |
| [internal/chaos/app/app.go](../../internal/chaos/app/app.go) | 222 | +3/−2 | 本文件：把 chaos 命令行接到场景、执行器和自动验收流程。 |
| [internal/chaos/scenario/load.go](../../internal/chaos/scenario/load.go) | 84 | +16/−5 | 本文件：加载并检查场景文件，所有检查通过后才允许开始操作。 |
| [internal/chaos/scenario/load_test.go](../../internal/chaos/scenario/load_test.go) | 103 | +3/−3 | 本文件：检查场景定义在执行前能否挡住错误输入。 |
| [internal/chaos/scenario/model.go](../../internal/chaos/scenario/model.go) | 67 | +20/−10 | 本文件：定义一个可重复实验需要保存的输入与判断条件。 |
| [internal/chaos/scenario/validate.go](../../internal/chaos/scenario/validate.go) | 140 | +6/−0 | 本文件：把场景操作限制在计划书规定的服务和资源内。 |
| [internal/chaos/verify/wait.go](../../internal/chaos/verify/wait.go) | 136 | +4/−0 | 本文件：等待症状和告警出现，或等待系统连续一分钟保持健康。 |
| [internal/doctor/agent/calls.go](../../internal/doctor/agent/calls.go) | 200 | +4/−4 | 本文件：执行一轮工具请求，并把结果按模型给出的 ID 放回对话。 |
| [internal/doctor/agent/doc.go](../../internal/doctor/agent/doc.go) | 26 | +2/−0 | Package agent 把“问模型、查证据、再问模型”组织成一个有边界的循环。 |
| [internal/doctor/agent/input.go](../../internal/doctor/agent/input.go) | 51 | +51/−0 | 本文件：把最初的告警输入也保存为一个可引用的审计步骤。 |
| [internal/doctor/agent/input_test.go](../../internal/doctor/agent/input_test.go) | 80 | +80/−0 | 本文件：证明无工具组引用的是实际告警，故障时间和证据编号会随请求一起保存。 |
| [internal/doctor/agent/loop.go](../../internal/doctor/agent/loop.go) | 253 | +8/−2 | 本文件：控制诊断从第一次询问模型到提交报告的整个过程。 |
| [internal/doctor/agent/model.go](../../internal/doctor/agent/model.go) | 104 | +14/−11 | 本文件：定义诊断循环需要的输入、步骤记录和依赖接口。 |
| [internal/doctor/agent/prompt.go](../../internal/doctor/agent/prompt.go) | 59 | +9/−1 | 本文件：固定系统提示与架构说明的前缀，供每一轮模型调用复用。 |
| [internal/doctor/agent/prompts/system.md](../../internal/doctor/agent/prompts/system.md) | 21 | +3/−1 | OpsDoctor 诊断规则 v8.0 |
| [internal/doctor/agent/replay.go](../../internal/doctor/agent/replay.go) | 123 | +14/−3 | 本文件：从已保存的模型请求和工具结果，重建中断前的对话。 |
| [internal/doctor/agent/replay_test.go](../../internal/doctor/agent/replay_test.go) | 122 | +1/−1 | 本文件：用旧步骤模拟中断，验证恢复、换模型和工具数据边界。 |
| [internal/doctor/alerts/service.go](../../internal/doctor/alerts/service.go) | 140 | +1/−0 | 本文件：检查 Alertmanager 通知，并把合法的分组交给仓储保存。 |
| [internal/doctor/app/agent_config.go](../../internal/doctor/app/agent_config.go) | 166 | +12/−4 | 本文件：读取模型、调度和页面安全配置，默认只运行不花钱的 Mock。 |
| [internal/doctor/app/agent_wiring.go](../../internal/doctor/app/agent_wiring.go) | 107 | +20/−3 | 本文件：把仓储、模型、Agent、调度和实时通知接成一条执行路径。 |
| [internal/doctor/app/app.go](../../internal/doctor/app/app.go) | 128 | +13/−3 | 本文件：按顺序准备医生自己的数据库、接入系统和 HTTP 服务。 |
| [internal/doctor/app/cli_test.go](../../internal/doctor/app/cli_test.go) | 93 | +16/−0 | 本文件：验证命令行始终把工具参数交给同一个医生接口。 |
| [internal/doctor/app/comparison.go](../../internal/doctor/app/comparison.go) | 97 | +97/−0 | 本文件：在真实评测时额外装配第二家模型，不切换正在运行的全局客户端。 |
| [internal/doctor/app/comparison_test.go](../../internal/doctor/app/comparison_test.go) | 47 | +47/−0 | 本文件：验证第二家模型只有显式启用时才读取凭据。 |
| [internal/doctor/app/doc.go](../../internal/doctor/app/doc.go) | 24 | +4/−0 | Package app 把医生各部分按启动顺序连接起来。 |
| [internal/doctor/app/evaluation.go](../../internal/doctor/app/evaluation.go) | 49 | +49/−0 | 本文件：把实验组的含义落实为真正不同的工具菜单。 |
| [internal/doctor/app/evaluation_test.go](../../internal/doctor/app/evaluation_test.go) | 55 | +55/−0 | 本文件：确认实验组差异由执行菜单保证，不靠模型自觉遵守提示。 |
| [internal/doctor/httpapi/diagnostics.go](../../internal/doctor/httpapi/diagnostics.go) | 106 | +12/−1 | 本文件：接收手动症状、取消任务，并向页面说明当前执行模式和预算。 |
| [internal/doctor/httpapi/doc.go](../../internal/doctor/httpapi/doc.go) | 24 | +2/−0 | Package httpapi 把告警、人工取证和实时页面接到同一个医生。 |
| [internal/doctor/httpapi/evaluations.go](../../internal/doctor/httpapi/evaluations.go) | 77 | +77/−0 | 本文件：把已经保存的评测批次和结果展示给登录用户。 |
| [internal/doctor/httpapi/evaluations_test.go](../../internal/doctor/httpapi/evaluations_test.go) | 65 | +65/−0 | 本文件：证明评测页面只能读取已保存的结果，并且需要登录。 |
| [internal/doctor/httpapi/router.go](../../internal/doctor/httpapi/router.go) | 200 | +29/−18 | 本文件：把登录、系统工具和告警入口接到同一个 Gin 服务。 |
| [internal/doctor/httpapi/router_test.go](../../internal/doctor/httpapi/router_test.go) | 235 | +5/−0 | 本文件：在内存 HTTP 请求里检查登录、系统令牌和工具入口。 |
| [internal/doctor/llm/cost.go](../../internal/doctor/llm/cost.go) | 114 | +4/−0 | 本文件：用整数表示费用，并规定发送前预留、发送后结算的账本接口。 |
| [internal/doctor/llm/doc.go](../../internal/doctor/llm/doc.go) | 21 | +2/−0 | Package llm 约定医生怎样和模型交换消息，以及怎样记录这次交换的费用。 |
| [internal/doctor/llm/openai/client.go](../../internal/doctor/llm/openai/client.go) | 225 | +43/−15 | 本文件：把一轮模型请求发送出去，并在发送前后留下费用和内容审计。 |
| [internal/doctor/llm/openai/client_test.go](../../internal/doctor/llm/openai/client_test.go) | 216 | +18/−0 | 本文件：用内存 HTTP 传输检查模型协议、重试、费用预留与取消。 |
| [internal/doctor/llm/openai/doc.go](../../internal/doctor/llm/openai/doc.go) | 18 | +1/−0 | Package openai 用普通 HTTP 实现兼容 OpenAI 的非流式工具调用协议。 |
| [internal/doctor/llm/openai/network_test.go](../../internal/doctor/llm/openai/network_test.go) | 139 | +139/−0 | 本文件：证明网络重试有次数、时间和费用边界，不会把未知费用当成免费请求。 |
| [internal/doctor/llm/openai/protocol.go](../../internal/doctor/llm/openai/protocol.go) | 86 | +1/−1 | 本文件：把模型 HTTP 回复转换为对话消息，限定可重试的错误。 |
| [internal/doctor/llm/tariff.go](../../internal/doctor/llm/tariff.go) | 39 | +39/−0 | 本文件：根据请求发出时的北京时间选择 DeepSeek 的峰时或闲时价格。 |
| [internal/doctor/llm/tariff_test.go](../../internal/doctor/llm/tariff_test.go) | 46 | +46/−0 | 本文件：固定日期测试闲时价格，不依赖测试机器所在时区。 |
| [internal/doctor/run/doc.go](../../internal/doctor/run/doc.go) | 16 | +2/−0 | Package run 决定哪个诊断现在可以执行，并管理取消和结束状态。 |
| [internal/doctor/run/execution.go](../../internal/doctor/run/execution.go) | 76 | +76/−0 | 本文件：为一份评测任务选择已经配置好的模型和账本。 |
| [internal/doctor/run/execution_test.go](../../internal/doctor/run/execution_test.go) | 68 | +68/−0 | 本文件：用阻塞的假模型验证配对任务选对模型，并且不会跨账户停用。 |
| [internal/doctor/run/scheduler.go](../../internal/doctor/run/scheduler.go) | 434 | +77/−32 | 本文件：为排队诊断分配名额，运行结束后回收名额，并处理取消。 |
| [internal/doctor/run/scheduler_test.go](../../internal/doctor/run/scheduler_test.go) | 367 | +29/−1 | 本文件：用可控时间和阻塞模型验证调度名额、过时任务、费用拦截与取消。 |
| [internal/doctor/store/alerts.go](../../internal/doctor/store/alerts.go) | 165 | +27/−3 | 本文件：把同一次故障的反复通知归到同一个诊断，并判断故障何时结束。 |
| [internal/doctor/store/approvals.go](../../internal/doctor/store/approvals.go) | 337 | +22/−6 | 本文件：保存人工审批，并用条件更新保证同一张单只能批准或拒绝一次。 |
| [internal/doctor/store/budget.go](../../internal/doctor/store/budget.go) | 209 | +19/−1 | 本文件：把真实模型的费用预留与结算保存到医生数据库。 |
| [internal/doctor/store/diagnoses.go](../../internal/doctor/store/diagnoses.go) | 124 | +7/−2 | 本文件：读取已经创建的诊断任务，供验收和后续页面使用。 |
| [internal/doctor/store/doc.go](../../internal/doctor/store/doc.go) | 51 | +22/−0 | Package store 保存医生自己的用户、告警、诊断步骤和费用账本。 |
| [internal/doctor/store/eval_baseline_amendment.go](../../internal/doctor/store/eval_baseline_amendment.go) | 146 | +146/−0 | 本文件：保存用户批准的流量守卫修订，并重启尚未创建任何模型槽位的中断轮次。 |
| [internal/doctor/store/eval_baseline_amendment_integration_test.go](../../internal/doctor/store/eval_baseline_amendment_integration_test.go) | 130 | +130/−0 | 本文件：在专用医生库验证基线中断的重启边界，不运行真实模型。 |
| [internal/doctor/store/eval_budget.go](../../internal/doctor/store/eval_budget.go) | 117 | +117/−0 | 本文件：在模型费用记账的同一个事务里限制整批评测的 token 消耗。 |
| [internal/doctor/store/eval_budget_integration_test.go](../../internal/doctor/store/eval_budget_integration_test.go) | 72 | +72/−0 | 本文件：证明整批 token 上限和供应商费用账户在同一事务里生效。 |
| [internal/doctor/store/eval_calibration.go](../../internal/doctor/store/eval_calibration.go) | 52 | +52/−0 | 本文件：固定人工校准抽中的二十份报告，不替用户填写分数。 |
| [internal/doctor/store/eval_clock_amendment.go](../../internal/doctor/store/eval_clock_amendment.go) | 62 | +62/−0 | 本文件：保存用户批准的校时规则变更及一次中断补跑，不重写原冻结清单。 |
| [internal/doctor/store/eval_clock_amendment_integration_test.go](../../internal/doctor/store/eval_clock_amendment_integration_test.go) | 88 | +88/−0 | 本文件：用专用医生库证明校时补跑只适用于明确批准的原取消任务。 |
| [internal/doctor/store/eval_execution.go](../../internal/doctor/store/eval_execution.go) | 33 | +33/−0 | 本文件：在评测动作开始前保存执行器版本，续跑也留下自己的记录。 |
| [internal/doctor/store/eval_execution_integration_test.go](../../internal/doctor/store/eval_execution_integration_test.go) | 74 | +74/−0 | 本文件：用专用医生测试库验证每份评测诊断的模型与账户绑定。 |
| [internal/doctor/store/eval_judge.go](../../internal/doctor/store/eval_judge.go) | 170 | +170/−0 | 本文件：保存评委的独立收费审计以及不可覆盖的最终评分。 |
| [internal/doctor/store/eval_judge_integration_test.go](../../internal/doctor/store/eval_judge_integration_test.go) | 127 | +127/−0 | 本文件：用专用测试库证明评委走独立账户，续跑不重复创建审计记录。 |
| [internal/doctor/store/eval_listing.go](../../internal/doctor/store/eval_listing.go) | 43 | +43/−0 | 本文件：为页面提供有数量上限的评测批次清单。 |
| [internal/doctor/store/eval_lock.go](../../internal/doctor/store/eval_lock.go) | 69 | +69/−0 | 本文件：保证同一批评测或同一份评分同时只有一个命令行进程负责。 |
| [internal/doctor/store/eval_reliability.go](../../internal/doctor/store/eval_reliability.go) | 93 | +93/−0 | 本文件：区分模型没能开始回答和已经回答却诊断失败的情况。 |
| [internal/doctor/store/eval_reliability_test.go](../../internal/doctor/store/eval_reliability_test.go) | 37 | +37/−0 | 本文件：用请求审计的固定状态验证哪些失败可以补跑。 |
| [internal/doctor/store/eval_results.go](../../internal/doctor/store/eval_results.go) | 154 | +154/−0 | 本文件：保存实验槽位、最初告警和最终结果，供评测器断点续跑。 |
| [internal/doctor/store/eval_results_integration_test.go](../../internal/doctor/store/eval_results_integration_test.go) | 132 | +132/−0 | 本文件：用专用医生测试库验证结果槽位的续跑边界。 |
| [internal/doctor/store/eval_trials.go](../../internal/doctor/store/eval_trials.go) | 86 | +86/−0 | 本文件：记录一轮故障执行到哪一步，让宿主机评测器安全续跑。 |
| [internal/doctor/store/eval_trials_integration_test.go](../../internal/doctor/store/eval_trials_integration_test.go) | 81 | +81/−0 | 本文件：用专用测试库检查故障轮次在进程中断后的续跑边界。 |
| [internal/doctor/store/evaluation.go](../../internal/doctor/store/evaluation.go) | 182 | +182/−0 | 本文件：保存评测期间的调度边界和评测器创建的诊断。 |
| [internal/doctor/store/evaluation_integration_test.go](../../internal/doctor/store/evaluation_integration_test.go) | 132 | +132/−0 | 本文件：用专用医生测试库验证评测不会重复创建付费任务，也不会执行写操作。 |
| [internal/doctor/store/leases.go](../../internal/doctor/store/leases.go) | 230 | +18/−4 | 本文件：在数据库里分配和续租诊断的执行权。 |
| [internal/doctor/store/migrations/0026_evaluation_context.sql](../../internal/doctor/store/migrations/0026_evaluation_context.sql) | 7 | +7/−0 | 故障时间和实验配置由评测器提供，模型不能通过工具参数改变它们。 |
| [internal/doctor/store/migrations/0027_evaluation_targets.sql](../../internal/doctor/store/migrations/0027_evaluation_targets.sql) | 7 | +7/−0 | 评测器退出后暂停状态仍在；不能因为进程崩溃又启动一批付费告警诊断。 |
| [internal/doctor/store/migrations/0028_eval_runs.sql](../../internal/doctor/store/migrations/0028_eval_runs.sql) | 12 | +12/−0 | 一批评测的配置与 token 额度独立于供应商费用账户，两种限制都必须满足。 |
| [internal/doctor/store/migrations/0029_diagnosis_eval_run.sql](../../internal/doctor/store/migrations/0029_diagnosis_eval_run.sql) | 4 | +4/−0 | 归属由评测器在创建时确定；模型和工具参数没有更换预算账户的入口。 |
| [internal/doctor/store/migrations/0030_eval_attempts.sql](../../internal/doctor/store/migrations/0030_eval_attempts.sql) | 9 | +9/−0 | 每个真实 HTTP 尝试都保留 token 预留，包括重试和结果不确定的请求。 |
| [internal/doctor/store/migrations/0031_eval_results.sql](../../internal/doctor/store/migrations/0031_eval_results.sql) | 19 | +19/−0 | 每个场景轮次的每个组只有一个槽位。重试取回相同输入和诊断，不能重复付费。 |
| [internal/doctor/store/migrations/0032_diagnosis_duration.sql](../../internal/doctor/store/migrations/0032_diagnosis_duration.sql) | 5 | +5/−0 | 时间戳用于回答“什么时候”，单调时钟时长用于回答“花了多久”，两者不能相减代替。 |
| [internal/doctor/store/migrations/0033_eval_makeup.sql](../../internal/doctor/store/migrations/0033_eval_makeup.sql) | 8 | +8/−0 | 第一次失败仍占原槽位；基础设施失败最多有一次补跑，不能覆盖或反复挑结果。 |
| [internal/doctor/store/migrations/0034_eval_execution.sql](../../internal/doctor/store/migrations/0034_eval_execution.sql) | 5 | +5/−0 | 每份评测诊断固定模型执行配置与账户，不能靠进程切换偷偷更换币种或供应商。 |
| [internal/doctor/store/migrations/0035_eval_profile_slot.sql](../../internal/doctor/store/migrations/0035_eval_profile_slot.sql) | 5 | +5/−0 | 同一次故障允许两家模型各有 E1；补跑仍在各自配置内最多一次。 |
| [internal/doctor/store/migrations/0036_eval_judges.sql](../../internal/doctor/store/migrations/0036_eval_judges.sql) | 6 | +6/−0 | 评分与原始诊断分开保存；格式错误不能覆盖医生原报告，也不能借续跑反复挑分数。 |
| [internal/doctor/store/migrations/0037_eval_trials.sql](../../internal/doctor/store/migrations/0037_eval_trials.sql) | 13 | +13/−0 | 一轮故障要跨过注入、诊断、恢复和冷却。状态单独保存，重启不能重新注入已完成的轮次。 |
| [internal/doctor/store/migrations/0038_eval_interruptions.sql](../../internal/doctor/store/migrations/0038_eval_interruptions.sql) | 11 | +11/−0 | 每次暂停都追加原因，后来的成功不能抹掉先前的环境失败。 |
| [internal/doctor/store/migrations/0039_eval_calibration.sql](../../internal/doctor/store/migrations/0039_eval_calibration.sql) | 7 | +7/−0 | 只保存抽样编号，不填写人的分数。重复导出不能换二十份更好看的报告。 |
| [internal/doctor/store/migrations/0040_eval_clock_amendments.sql](../../internal/doctor/store/migrations/0040_eval_clock_amendments.sql) | 13 | +13/−0 | 用户批准的校时规则变更另存审计，原冻结清单与失败记录都不能覆盖。 |
| [internal/doctor/store/migrations/0041_eval_baseline_amendments.sql](../../internal/doctor/store/migrations/0041_eval_baseline_amendments.sql) | 12 | +12/−0 | 本次用户批准只修正诊断期流量检查；旧轮次与中断另存，不能用新注入覆盖它们。 |
| [internal/doctor/store/migrations/0042_eval_execution_audits.sql](../../internal/doctor/store/migrations/0042_eval_execution_audits.sql) | 12 | +12/−0 | 每次执行命令、推进轮次与运行槽位都追加当时版本；已有实验不回填虚构的执行记录。 |
| [internal/doctor/store/migrations/README.md](../../internal/doctor/store/migrations/README.md) | 54 | +17/−0 | 医生自己的表 |
| [internal/doctor/store/steps.go](../../internal/doctor/store/steps.go) | 126 | +3/−2 | 本文件：给步骤分配编号，并为页面断线后的补发提供有序读取。 |
| [internal/doctor/store/store.go](../../internal/doctor/store/store.go) | 100 | +12/−3 | 本文件：建立医生仓储并应用医生自己的迁移。 |
| [internal/doctor/store/store_integration_test.go](../../internal/doctor/store/store_integration_test.go) | 186 | +10/−0 | 本文件：在专用医生测试库验证迁移、通知事务和分组去重。 |
| [internal/doctor/store/tasks.go](../../internal/doctor/store/tasks.go) | 197 | +12/−10 | 本文件：保存调度状态变化，隔离启动清理、排队和结束操作。 |
| [internal/doctor/store/timing_integration_test.go](../../internal/doctor/store/timing_integration_test.go) | 70 | +70/−0 | 本文件：用独立测试库检查时间来源和恢复后的未知耗时。 |
| [internal/doctor/tools/doc.go](../../internal/doctor/tools/doc.go) | 29 | +4/−0 | Package tools 把“医生可以问哪些问题”做成一份固定的工具清单。 |
| [internal/doctor/tools/runbooks.go](../../internal/doctor/tools/runbooks.go) | 218 | +218/−0 | 本文件：从部署者提供的 Markdown 手册建立一个只读关键词索引。 |
| [internal/doctor/tools/runbooks_test.go](../../internal/doctor/tools/runbooks_test.go) | 94 | +94/−0 | 本文件：验证手册检索按关键词和文档长度排序，并继续遵守工具的共同边界。 |
| [internal/doctor/tools/window.go](../../internal/doctor/tools/window.go) | 67 | +67/−0 | 本文件：为一次诊断固定工具菜单和默认查询起点。 |
| [internal/doctor/tools/window_test.go](../../internal/doctor/tools/window_test.go) | 58 | +58/−0 | 本文件：验证不同诊断的默认时间范围不会串用，实验组不能调用被移除的工具。 |
| [internal/doctor/web/static/README.md](../../internal/doctor/web/static/README.md) | 13 | +2/−0 | 医生的浏览器页面 |
| [internal/doctor/web/static/app.js](../../internal/doctor/web/static/app.js) | 307 | +43/−3 | 读取评测批次与结果，使用安全文本节点展示 |
| [internal/doctor/web/static/index.html](../../internal/doctor/web/static/index.html) | 34 | +5/−0 | 诊断工作台及评测结果区域 |
| [internal/doctor/web/static/style.css](../../internal/doctor/web/static/style.css) | 5 | +3/−0 | 结果表格和页面布局 |
| [internal/eval/README.md](../../internal/eval/README.md) | 16 | +16/−0 | eval：安排实验、保存结果和比较报告 |
| [internal/eval/judge/doc.go](../../internal/eval/judge/doc.go) | 16 | +16/−0 | 本文件：说明评委与被评测医生的职责区别。 |
| [internal/eval/judge/grade.go](../../internal/eval/judge/grade.go) | 177 | +177/−0 | 本文件：让另一家模型评分，并在采纳前检查格式和证据引用。 |
| [internal/eval/judge/grade_test.go](../../internal/eval/judge/grade_test.go) | 80 | +80/−0 | 本文件：用固定回复测试评委校验，绝不联网付费。 |
| [internal/eval/judge/mock.go](../../internal/eval/judge/mock.go) | 40 | +40/−0 | 本文件：提供明确标记的免费评分脚本，供评测流程测试。 |
| [internal/eval/judge/protocol.go](../../internal/eval/judge/protocol.go) | 81 | +81/−0 | 本文件：把需要模型填写的项目和程序已经判定的项目分开。 |
| [internal/eval/judge/protocol_test.go](../../internal/eval/judge/protocol_test.go) | 107 | +107/−0 | 本文件：确认模型只收到需要它判断的编号，不靠文字劝它别重复。 |
| [internal/eval/judge/rubric.md](../../internal/eval/judge/rubric.md) | 13 | +13/−0 | rubric-v1.1 |
| [internal/eval/report/calibration.go](../../internal/eval/report/calibration.go) | 132 | +132/−0 | 本文件：抽取二十份不同层次的报告，生成人填写的盲评材料。 |
| [internal/eval/report/calibration_test.go](../../internal/eval/report/calibration_test.go) | 82 | +82/−0 | 本文件：检查盲评抽样覆盖不同来源，且绝不替人补分。 |
| [internal/eval/report/doc.go](../../internal/eval/report/doc.go) | 17 | +17/−0 | Package report 把已经保存的诊断整理成表格，不负责替模型补结论。 |
| [internal/eval/report/markdown.go](../../internal/eval/report/markdown.go) | 98 | +98/−0 | 本文件：将数据库中的评测结果排成便于审阅的 Markdown 表格。 |
| [internal/eval/report/statistics.go](../../internal/eval/report/statistics.go) | 138 | +138/−0 | 本文件：按场景、实验组和模型分别汇总少量重复运行。 |
| [internal/eval/report/statistics_test.go](../../internal/eval/report/statistics_test.go) | 38 | +38/−0 | 本文件：证明统计排除不会把已经发生的费用一起删掉。 |
| [internal/eval/report/summary.go](../../internal/eval/report/summary.go) | 77 | +77/−0 | 本文件：从医生保存的真实任务和步骤提取试跑结果。 |
| [internal/eval/report/summary_test.go](../../internal/eval/report/summary_test.go) | 106 | +106/−0 | 本文件：用固定记录验证组件精确比对、模型变更排除和失败保留。 |
| [internal/eval/runner/amendment.go](../../internal/eval/runner/amendment.go) | 103 | +103/−0 | 本文件：把用户批准的校时守卫调整作为独立审计接到原冻结批次。 |
| [internal/eval/runner/amendment_test.go](../../internal/eval/runner/amendment_test.go) | 50 | +50/−0 | 本文件：证明校时例外不会顺带接受提示、工具或场景的改动。 |
| [internal/eval/runner/automatic.go](../../internal/eval/runner/automatic.go) | 235 | +235/−0 | 本文件：按冻结顺序自动完成注入、五组诊断、恢复和下一轮等待。 |
| [internal/eval/runner/automatic_test.go](../../internal/eval/runner/automatic_test.go) | 62 | +62/−0 | 本文件：检查恢复文件不能把另一场实验接到本轮。 |
| [internal/eval/runner/baseline_amendment.go](../../internal/eval/runner/baseline_amendment.go) | 132 | +132/−0 | 本文件：把用户批准的流量守卫修正接到原批次，并审计尚未调用模型的轮次重启。 |
| [internal/eval/runner/baseline_amendment_test.go](../../internal/eval/runner/baseline_amendment_test.go) | 81 | +81/−0 | 本文件：证明流量修订的白名单不能同时更改医生提示、工具、手册或评分。 |
| [internal/eval/runner/calibration.go](../../internal/eval/runner/calibration.go) | 96 | +96/−0 | 本文件：从已完成批次取出固定的二十份盲评材料。 |
| [internal/eval/runner/cli.go](../../internal/eval/runner/cli.go) | 367 | +367/−0 | 本文件：提供阶段 8 第一停点之前的 E1 试跑命令。 |
| [internal/eval/runner/client.go](../../internal/eval/runner/client.go) | 163 | +163/−0 | 本文件：用人的登录身份读取医生状态和本次告警，不直接访问靶场数据源。 |
| [internal/eval/runner/clock.go](../../internal/eval/runner/clock.go) | 111 | +111/−0 | 本文件：在评测花钱前和执行期间检查医生容器的时钟是否跳变。 |
| [internal/eval/runner/clock_test.go](../../internal/eval/runner/clock_test.go) | 29 | +29/−0 | 本文件：用固定样本证明时钟跳变会拦住评测，不在单元测试里启动 Docker。 |
| [internal/eval/runner/doc.go](../../internal/eval/runner/doc.go) | 33 | +33/−0 | Package runner 让同一次实验能找回相同的输入和诊断，而不是每次重试都重新花钱。 |
| [internal/eval/runner/environment.go](../../internal/eval/runner/environment.go) | 86 | +86/−0 | 本文件：在花钱前确认压测基线和本项目容器的实际内存占用。 |
| [internal/eval/runner/environment_test.go](../../internal/eval/runner/environment_test.go) | 86 | +86/−0 | 本文件：验证试跑的基线检查和内存暂停线不会被无效读数绕过。 |
| [internal/eval/runner/execution.go](../../internal/eval/runner/execution.go) | 21 | +21/−0 | 本文件：为评测命令、轮次和槽位留下实际执行的代码版本。 |
| [internal/eval/runner/grading.go](../../internal/eval/runner/grading.go) | 197 | +197/−0 | 本文件：把评委接入现有预算账本，并保存可续跑的评分结果。 |
| [internal/eval/runner/injection.go](../../internal/eval/runner/injection.go) | 71 | +71/−0 | 本文件：在 S12 观察期间持续产生少量真实业务日志，记录是否被医生读到。 |
| [internal/eval/runner/load.go](../../internal/eval/runner/load.go) | 151 | +151/−0 | 本文件：分别核对正常流量和场景注入完成后的流量，避免把故障本身当作基线漂移。 |
| [internal/eval/runner/load_test.go](../../internal/eval/runner/load_test.go) | 166 | +166/−0 | 本文件：用真实场景文件和假控制接口验证两种流量检查，所有测试都不花模型费用。 |
| [internal/eval/runner/manifest.go](../../internal/eval/runner/manifest.go) | 93 | +93/−0 | 本文件：记录一次试跑实际使用的代码、提示和手册版本。 |
| [internal/eval/runner/paired.go](../../internal/eval/runner/paired.go) | 402 | +402/−0 | 本文件：在一次真实故障上依次执行随机排序的五份配置。 |
| [internal/eval/runner/paired_test.go](../../internal/eval/runner/paired_test.go) | 68 | +68/−0 | 本文件：用虚构运行状态检查配对身份、随机排列和续跑矩阵。 |
| [internal/eval/scoring/doc.go](../../internal/eval/scoring/doc.go) | 12 | +12/−0 | 本文件：说明自动评分能确认什么，不能确认什么。 |
| [internal/eval/scoring/evidence.go](../../internal/eval/scoring/evidence.go) | 122 | +122/−0 | 本文件：依据真实工具返回值判断可以机械核对的证据。 |
| [internal/eval/scoring/evidence_test.go](../../internal/eval/scoring/evidence_test.go) | 53 | +53/−0 | 本文件：用伪造的工具记录验证评分不会从模型自述中猜证据。 |
| [internal/platform/README.md](../../internal/platform/README.md) | 45 | +6/−0 | platform：通用基础设施 |
| [internal/platform/clockcheck/doc.go](../../internal/platform/clockcheck/doc.go) | 16 | +16/−0 | Package clockcheck 检查进程看到的日期有没有突然跳动。 |
| [internal/platform/clockcheck/policy.go](../../internal/platform/clockcheck/policy.go) | 62 | +62/−0 | 本文件：把小幅校时、大幅跳变和反复跳变分开处理。 |
| [internal/platform/clockcheck/policy_test.go](../../internal/platform/clockcheck/policy_test.go) | 98 | +98/−0 | 本文件：不用真实等待或修改系统时间，验证分级校时规则的每个边界。 |
| [internal/platform/clockcheck/sample.go](../../internal/platform/clockcheck/sample.go) | 94 | +94/−0 | 本文件：用日期和单调时间的差值区分校时跳变与程序调度延迟。 |
| [internal/platform/clockcheck/sample_test.go](../../internal/platform/clockcheck/sample_test.go) | 86 | +86/−0 | 本文件：用固定读数验证时钟守卫，不在单元测试里修改系统日期。 |
| [internal/platform/httpx/doc.go](../../internal/platform/httpx/doc.go) | 26 | +2/−0 | Package httpx 提供所有 HTTP 接口共用的入口处理和统一的响应格式。 |
| [internal/platform/httpx/httpx.go](../../internal/platform/httpx/httpx.go) | 156 | +1/−1 | 本文件：HTTP 请求进入业务前后的通用保护。 |
| [internal/platform/httpx/user_agent.go](../../internal/platform/httpx/user_agent.go) | 36 | +36/−0 | 本文件：把客户端类型作为有界、可安全编码的访问日志字段。 |
| [internal/platform/httpx/user_agent_test.go](../../internal/platform/httpx/user_agent_test.go) | 45 | +45/−0 | 本文件：验证客户端文字只作为一个 JSON 日志字段，不获得执行能力。 |
| [tools/archcheck/doctor.go](../../tools/archcheck/doctor.go) | 102 | +16/−4 | 本文件：检查医生容器能从宿主机拿到什么。 |
| [tools/archcheck/doctor_test.go](../../tools/archcheck/doctor_test.go) | 76 | +5/−1 | 本文件：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。 |
| [进度.md](../../进度.md) | 165 | +8/−6 | OpsDoctor 进度看板 |
| [项目计划书.md](../../项目计划书.md) | 2153 | +28/−3 | OpsDoctor 项目计划书：Go 故障诊断 Agent + 可复现故障靶场 |

## 类型与函数导航

初稿来自 commentcheck -list。这里包含改动文件里的既有函数，便于逐行学习。使用入口说明调用所在流程，不声称每个辅助函数都由入口直接调用；关键调用顺序见汇报书第三节。

| 包 | 名称 | 位置 | 作用 | 使用入口 |
|---|---|---|---|---|
| cmd/doctor (main) | main | [cmd/doctor/main.go:16](../../cmd/doctor/main.go) | main 把 os.Exit 放在 Run 返回之后，避免跳过数据库和 HTTP 的清理 | Go 程序入口，转交 doctor/app |
| cmd/eval (main) | main | [cmd/eval/main.go:15](../../cmd/eval/main.go) | main 把 Ctrl+C 传给等待任务的流程，已经创建的诊断和结果不会因终端退出被删除 | Go 程序入口，转交 eval/runner |
| internal/chaos/action (action) | ProbeLog | [internal/chaos/action/probe.go:25](../../internal/chaos/action/probe.go) | ProbeLog 只允许本机业务入口，禁止跳转；模型工具不能调用此实验动作 | chaos.Engine → action.Runner，执行对应实验动作 |
| internal/chaos/action (action) | *Runner.compose | [internal/chaos/action/runner.go:101](../../internal/chaos/action/runner.go) | compose 固定配置文件与项目目录，只接受内部代码构造的子命令 | chaos.Engine → action.Runner，执行对应实验动作 |
| internal/chaos/action (action) | Runner | [internal/chaos/action/runner.go:31](../../internal/chaos/action/runner.go) | Runner 只保存配置，不长时间占用数据库连接 | chaos.Engine → action.Runner，执行对应实验动作 |
| internal/chaos/action (action) | *Runner.Execute | [internal/chaos/action/runner.go:44](../../internal/chaos/action/runner.go) | Execute 再次检查参数，防止调用方绕过 YAML 加载器后执行任意操作 | chaos.Engine → action.Runner，执行对应实验动作 |
| internal/chaos/action (action) | *Runner.Command | [internal/chaos/action/runner.go:79](../../internal/chaos/action/runner.go) | Command 使用参数数组，YAML 里的值不会被 PowerShell 或 bash 当成代码 | chaos.Engine → action.Runner，执行对应实验动作 |
| internal/chaos/app (app) | *Engine.general | [internal/chaos/app/app.go:103](../../internal/chaos/app/app.go) | general 处理无需场景编号的命令 | cmd/chaos 或 eval.runner 调用 chaos.Run |
| internal/chaos/app (app) | *Engine.operate | [internal/chaos/app/app.go:127](../../internal/chaos/app/app.go) | operate 用已有记录匹配 verify 和 restore，防止把另一场景的效果误当成当前场景 | cmd/chaos 或 eval.runner 调用 chaos.Run |
| internal/chaos/app (app) | overrides | [internal/chaos/app/app.go:173](../../internal/chaos/app/app.go) | overrides 只覆盖对应动作已有的参数；没有这种动作的场景会拒绝无效的覆盖选项 | cmd/chaos 或 eval.runner 调用 chaos.Run |
| internal/chaos/app (app) | Run | [internal/chaos/app/app.go:33](../../internal/chaos/app/app.go) | Run 不隐式启动流量 | cmd/chaos 或 eval.runner 调用 chaos.Run |
| internal/chaos/app (app) | newEngine | [internal/chaos/app/app.go:77](../../internal/chaos/app/app.go) | newEngine 只读取密码，不输出它 | cmd/chaos 或 eval.runner 调用 chaos.Run |
| internal/chaos/scenario (scenario) | Load | [internal/chaos/scenario/load.go:26](../../internal/chaos/scenario/load.go) | Load 只读取当前阶段明确支持的文件，不把目录中新放入的任意文件自动变成可执行场景 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | Decode | [internal/chaos/scenario/load.go:47](../../internal/chaos/scenario/load.go) | Decode 先检查内容，再逐个检查动作 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | Scenario | [internal/chaos/scenario/model.go:16](../../internal/chaos/scenario/model.go) | Scenario 把实验操作与验收条件放在一起，避免两者各自修改后不再对应 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | Step | [internal/chaos/scenario/model.go:28](../../internal/chaos/scenario/model.go) | Step 只允许已经实现的动作 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | Symptom | [internal/chaos/scenario/model.go:47](../../internal/chaos/scenario/model.go) | Symptom 的表达式采用 PromQL，即 Prometheus 用来筛选和计算指标的查询语言 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | Truth | [internal/chaos/scenario/model.go:55](../../internal/chaos/scenario/model.go) | Truth 保存解释，不参与业务行为或告警规则 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | EvidenceRule | [internal/chaos/scenario/model.go:62](../../internal/chaos/scenario/model.go) | EvidenceRule 只用于事后评分，编号对应人类可读的里程碑，不参与注入或医生工具 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | validateInteger | [internal/chaos/scenario/validate.go:122](../../internal/chaos/scenario/validate.go) | validateInteger 提前使用服务端同样的单项边界，避免明知无效还去重建容器 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | ValidateStep | [internal/chaos/scenario/validate.go:22](../../internal/chaos/scenario/validate.go) | ValidateStep 先限制动作类型，再检查必要参数 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/scenario (scenario) | ValidateEnvironment | [internal/chaos/scenario/validate.go:87](../../internal/chaos/scenario/validate.go) | ValidateEnvironment 不接受 DSN、口令或任意环境变量，只允许实验所需的公开配置 | chaos 与 eval 读取、校验场景；医生不能导入 |
| internal/chaos/verify (verify) | *Client.Healthy | [internal/chaos/verify/wait.go:103](../../internal/chaos/verify/wait.go) | Healthy 每次坏样本都清空连续计时；不能把分散的健康时间累加成一分钟 | chaos verify / healthy 使用真实症状判断 |
| internal/chaos/verify (verify) | wait | [internal/chaos/verify/wait.go:127](../../internal/chaos/verify/wait.go) | wait 允许 Ctrl+C 及时结束等待，之后 app 仍可用独立预算做恢复 | chaos verify / healthy 使用真实症状判断 |
| internal/chaos/verify (verify) | Result | [internal/chaos/verify/wait.go:25](../../internal/chaos/verify/wait.go) | Result 即使失败也保留等待时间，不能只在成功时记录实验数据 | chaos verify / healthy 使用真实症状判断 |
| internal/chaos/verify (verify) | *Client.Symptom | [internal/chaos/verify/wait.go:33](../../internal/chaos/verify/wait.go) | Symptom 的 ctx 由注入开始时间计算截止，部署耗时也计入三分钟，不另送三分钟 | chaos verify / healthy 使用真实症状判断 |
| internal/chaos/verify (verify) | healthyQueries | [internal/chaos/verify/wait.go:67](../../internal/chaos/verify/wait.go) | healthyQueries 不把无流量当健康；三个业务端点和三个依赖采集器都必须可用 | chaos verify / healthy 使用真实症状判断 |
| internal/chaos/verify (verify) | *Client.healthSample | [internal/chaos/verify/wait.go:81](../../internal/chaos/verify/wait.go) | healthSample 任一查询失败都会使本次采样失败，空数据不会掩盖监控断开 | chaos verify / healthy 使用真实症状判断 |
| internal/doctor/agent (agent) | *session.saveTool | [internal/doctor/agent/calls.go:132](../../internal/doctor/agent/calls.go) | saveTool 把实际步骤号包在 tool 回复里，模型无需猜测并行完成的顺序 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | toolKey | [internal/doctor/agent/calls.go:156](../../internal/doctor/agent/calls.go) | toolKey 加入模型步骤号；供应商在不同轮里复用同一个 ID 时仍然是两次不同调用 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | toolMessage | [internal/doctor/agent/calls.go:160](../../internal/doctor/agent/calls.go) | toolMessage 把工具正文编码成 JSON 字符串 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | invalidReport | [internal/doctor/agent/calls.go:169](../../internal/doctor/agent/calls.go) | invalidReport 与普通工具错误一样交回模型，只说明校验失败，不结束诊断 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.writeTool | [internal/doctor/agent/calls.go:174](../../internal/doctor/agent/calls.go) | writeTool 等待时不伪造工具完成结果 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.calls | [internal/doctor/agent/calls.go:23](../../internal/doctor/agent/calls.go) | calls 要求报告独占一轮；混合时执行查看工具，明确拒绝报告，让模型看完新结果再提交 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | validateCalls | [internal/doctor/agent/calls.go:63](../../internal/doctor/agent/calls.go) | validateCalls 只检查协议关联，工具名是否存在和参数是否正确仍交给统一注册表 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.readTools | [internal/doctor/agent/calls.go:78](../../internal/doctor/agent/calls.go) | readTools 仅审计失败才中止整轮；参数错误、超时和数据源错误都作为普通工具结果 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.prepareInput | [internal/doctor/agent/input.go:18](../../internal/doctor/agent/input.go) | prepareInput 只在提供故障时间或启用输入组时记录，旧诊断的恢复格式保持可读 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.rememberEvidence | [internal/doctor/agent/input.go:47](../../internal/doctor/agent/input.go) | rememberEvidence 在 E0 下拒绝把模型自己说的话当作证据，其他诊断仍沿用原来的步骤校验 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.ask | [internal/doctor/agent/loop.go:119](../../internal/doctor/agent/loop.go) | ask 把实际脱敏后的请求写进步骤；真实 HTTP 尝试另由 llm 账本在发送前保存 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.finish | [internal/doctor/agent/loop.go:197](../../internal/doctor/agent/loop.go) | finish 先要求整理；如果确实提交了报告但校验失败，再给一次修正机会 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | record | [internal/doctor/agent/loop.go:235](../../internal/doctor/agent/loop.go) | record 取消之后只留半秒保存最后状态；真实查询仍使用已取消的原 context | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Agent | [internal/doctor/agent/loop.go:23](../../internal/doctor/agent/loop.go) | Agent 构造后只读；每次 Run 的消息和计数都放在独立 session 中 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | markMock | [internal/doctor/agent/loop.go:243](../../internal/doctor/agent/loop.go) | markMock 在程序端加醒目标记，不能靠脚本模型自己记住声明 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *Agent.isMock | [internal/doctor/agent/loop.go:253](../../internal/doctor/agent/loop.go) | isMock 只检查程序选定的模型名，供应商回复不能把真实请求变成免费请求 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | session | [internal/doctor/agent/loop.go:31](../../internal/doctor/agent/loop.go) | session 保存一次诊断的状态，不把历史挂在共享模型客户端上 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | New | [internal/doctor/agent/loop.go:45](../../internal/doctor/agent/loop.go) | New 不联网，参数错误在启动时暴露，单元测试可使用较小预算 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *Agent.Run | [internal/doctor/agent/loop.go:60](../../internal/doctor/agent/loop.go) | Run 为普通查证和最后整理分别设截止时间，但二者加起来仍只有一份总时限 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.execute | [internal/doctor/agent/loop.go:88](../../internal/doctor/agent/loop.go) | execute 在每次回答之后检查累计用量，最后报告机会仍受 whole 的总时限约束 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Step | [internal/doctor/agent/model.go:21](../../internal/doctor/agent/model.go) | Step 按诊断内部的 Seq 排序；并行工具按保存完成的次序得到编号 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Recorder | [internal/doctor/agent/model.go:42](../../internal/doctor/agent/model.go) | Recorder 必须支持并行工具保存，成功返回的步骤才可被报告引用 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Toolbox | [internal/doctor/agent/model.go:47](../../internal/doctor/agent/model.go) | Toolbox 只提供当前系统的查看工具；模型无法改变它绑定的数据源 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | ApprovalFlow | [internal/doctor/agent/model.go:53](../../internal/doctor/agent/model.go) | ApprovalFlow 只有申请和读取结果的能力，没有批准按钮 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Input | [internal/doctor/agent/model.go:60](../../internal/doctor/agent/model.go) | Input 在开始诊断时固定，后续告警仍入库，但不会改变已经发送的固定消息前缀 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Limits | [internal/doctor/agent/model.go:77](../../internal/doctor/agent/model.go) | Limits 的默认值来自计划书，测试可调小以快速检查预算边界 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | DefaultLimits | [internal/doctor/agent/model.go:88](../../internal/doctor/agent/model.go) | DefaultLimits 给正常查证预留四分钟，最后一分钟只整理已有证据，不再查新数据 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | Result | [internal/doctor/agent/model.go:93](../../internal/doctor/agent/model.go) | Result 失败也带用量，费用不能因最终没有报告而消失 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | messages | [internal/doctor/agent/prompt.go:30](../../internal/doctor/agent/prompt.go) | messages 的前两条不会随轮次变化，动态证据始终追加在末尾 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | definitions | [internal/doctor/agent/prompt.go:49](../../internal/doctor/agent/prompt.go) | definitions 从同一个注册表生成工具菜单，Agent 不能偷偷增加另一个执行入口 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | finalRequest | [internal/doctor/agent/replay.go:113](../../internal/doctor/agent/replay.go) | finalRequest 同时检查菜单和程序保存的整理指令，不信任模型自称的预算状态 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.restore | [internal/doctor/agent/replay.go:21](../../internal/doctor/agent/replay.go) | restore 的计数来自全部历史，进程重启不能重新获得十五轮或一份新费用预算 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/agent (agent) | *session.repairFinal | [internal/doctor/agent/replay.go:95](../../internal/doctor/agent/replay.go) | repairFinal 只补一次最终报告，仍沿用原任务剩下的 token、费用与活动时间 | run 调度器 → Agent.Run；loop / replay / 调用与报告校验 |
| internal/doctor/alerts (alerts) | validLabels | [internal/doctor/alerts/service.go:108](../../internal/doctor/alerts/service.go) | validLabels 限制单字段和数量，整体正文另由 HTTP 的一 MiB 上限限制 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Title | [internal/doctor/alerts/service.go:121](../../internal/doctor/alerts/service.go) | Title 只用于列表展示，数据库身份始终是系统名与分组键，不用可变的标题去重 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Alert | [internal/doctor/alerts/service.go:24](../../internal/doctor/alerts/service.go) | Alert 保存证据快照；Labels、Annotations 是外部文字，以后也不能把它们当指令 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Notification | [internal/doctor/alerts/service.go:34](../../internal/doctor/alerts/service.go) | Notification 的 GroupKey 是推送方的分组键，仓储还会加上系统名形成去重身份 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Receipt | [internal/doctor/alerts/service.go:42](../../internal/doctor/alerts/service.go) | Receipt 说明通知已保存或被忽略，不表示诊断已经执行 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Receiver | [internal/doctor/alerts/service.go:51](../../internal/doctor/alerts/service.go) | Receiver 定义在通知处理这一使用方，MySQL 仓储只需实现它，不需要显式 implements | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Service | [internal/doctor/alerts/service.go:56](../../internal/doctor/alerts/service.go) | Service 不保留某个“默认系统”，每次身份都由经过认证的接口传入 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | New | [internal/doctor/alerts/service.go:59](../../internal/doctor/alerts/service.go) | New 让测试可以替换仓储，同时生产路径仍只有同一个通知校验入口 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | *Service.Receive | [internal/doctor/alerts/service.go:62](../../internal/doctor/alerts/service.go) | Receive 校验错误与存储错误分开，HTTP 层才能分别返回 400 和可重试的 503 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Invalid | [internal/doctor/alerts/service.go:73](../../internal/doctor/alerts/service.go) | Invalid 表示通知格式错误；错误文字由代码生成，不包含请求正文 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | Invalid.Error | [internal/doctor/alerts/service.go:76](../../internal/doctor/alerts/service.go) | Error 让 Invalid 可作为标准 error 返回，接口层通过 errors.As 判断分类 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/alerts (alerts) | validate | [internal/doctor/alerts/service.go:79](../../internal/doctor/alerts/service.go) | validate 不信任推送内容的条数和字段长度，认证通过也不意味着可以无限占用数据库 | HTTP 告警入口 → alerts.Service；评测只留档 |
| internal/doctor/app (app) | knownSecrets | [internal/doctor/app/agent_config.go:137](../../internal/doctor/app/agent_config.go) | knownSecrets 只收集本项目明确使用的凭据，在内存中替换，不输出也不另存清单 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | agentSettings | [internal/doctor/app/agent_config.go:26](../../internal/doctor/app/agent_config.go) | agentSettings 含 API Key，不能作为整个对象写进日志或错误 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | loadAgentConfig | [internal/doctor/app/agent_config.go:53](../../internal/doctor/app/agent_config.go) | loadAgentConfig 的空值是配置错误，不把误拼的真实模型模式静默退回 Mock | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | createModel | [internal/doctor/app/agent_wiring.go:102](../../internal/doctor/app/agent_wiring.go) | createModel 不做探测调用；真实配置不完整时返回错误，不回退成误导用户的脚本报告 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | assembleAgent | [internal/doctor/app/agent_wiring.go:34](../../internal/doctor/app/agent_wiring.go) | assembleAgent 只装配，不启动 goroutine；全部成功后 Run 才启动调度和监听端口 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | serve | [internal/doctor/app/app.go:112](../../internal/doctor/app/app.go) | serve 停止接新请求后等待在途请求；工具最多十秒，停机预算再多留两秒做响应收尾 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | Run | [internal/doctor/app/app.go:40](../../internal/doctor/app/app.go) | Run 只有装配完全成功才监听端口，启动失败也会关闭已经创建的资源 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | loadComparison | [internal/doctor/app/comparison.go:26](../../internal/doctor/app/comparison.go) | loadComparison 只在真实模式读额外凭据，避免集成测试中的告警意外触发收费 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | comparisonExecution | [internal/doctor/app/comparison.go:79](../../internal/doctor/app/comparison.go) | comparisonExecution 仍用同一个 OpenAI 协议适配器，费用没有另外开一条免审计路径 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/app (app) | prepareDiagnosis | [internal/doctor/app/evaluation.go:19](../../internal/doctor/app/evaluation.go) | prepareDiagnosis 对普通诊断也设置时间边界；评测组只改变可用数据，不注入场景提示 | cmd/doctor → app.Run；启动时装配与校验 |
| internal/doctor/httpapi (httpapi) | *api.manual | [internal/doctor/httpapi/diagnostics.go:25](../../internal/doctor/httpapi/diagnostics.go) | manual 限制症状长度，服务端绑定系统，保存后才唤醒调度器 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.cancelDiagnosis | [internal/doctor/httpapi/diagnostics.go:63](../../internal/doctor/httpapi/diagnostics.go) | cancelDiagnosis 只接受服务生成的 ID，返回成功表示取消已请求，终态随后通过 SSE 到达 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.runtimeStatus | [internal/doctor/httpapi/diagnostics.go:84](../../internal/doctor/httpapi/diagnostics.go) | runtimeStatus 同一个读数同时驱动横幅和按钮状态，费用原因不是页面自己猜出来的 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | Evaluations | [internal/doctor/httpapi/evaluations.go:23](../../internal/doctor/httpapi/evaluations.go) | Evaluations 不提供修改方法，登录用户也不能借结果页改写失败记录 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.evalRuns | [internal/doctor/httpapi/evaluations.go:30](../../internal/doctor/httpapi/evaluations.go) | evalRuns 有查询预算；数据库不可用时不显示一张误导人的空表 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.evalResults | [internal/doctor/httpapi/evaluations.go:46](../../internal/doctor/httpapi/evaluations.go) | evalResults 先确认批次存在，拼错编号不能悄悄显示为“零份报告” | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.requireLogin | [internal/doctor/httpapi/router.go:127](../../internal/doctor/httpapi/router.go) | requireLogin 不把解析失败的细节回显给客户端，令牌原文也不会写入日志 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.listTargets | [internal/doctor/httpapi/router.go:157](../../internal/doctor/httpapi/router.go) | listTargets 只返回名称和工具说明，不把接入地址、账号变量或令牌发给客户端 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | *api.runTool | [internal/doctor/httpapi/router.go:186](../../internal/doctor/httpapi/router.go) | runTool 是 doctorctl 与未来其他入口共用的调用路径，不能直接调用适配器 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | Repository | [internal/doctor/httpapi/router.go:38](../../internal/doctor/httpapi/router.go) | Repository 定义在接口使用方，测试可以观察持久化是否被调用，而不启动 MySQL | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | Runner | [internal/doctor/httpapi/router.go:51](../../internal/doctor/httpapi/router.go) | Runner 是页面需要的调度操作；没有提供恢复、审批或任意执行入口 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | Options | [internal/doctor/httpapi/router.go:58](../../internal/doctor/httpapi/router.go) | Options 只接收已经装配的对象，不读取环境变量或创建第二套数据库池 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | api | [internal/doctor/httpapi/router.go:78](../../internal/doctor/httpapi/router.go) | api 持有服务范围的登录限流器，不能让猜密码请求无限占用 bcrypt 的 CPU | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/httpapi (httpapi) | New | [internal/doctor/httpapi/router.go:85](../../internal/doctor/httpapi/router.go) | New 把页面、任务与手动工具接到同一身份边界，告警另用系统令牌 | router 注册的 HTTP 路由；页面或 eval 客户端调用 |
| internal/doctor/llm (llm) | Settlement | [internal/doctor/llm/cost.go:102](../../internal/doctor/llm/cost.go) | Settlement 保存供应商回复与结算额；失败文本只用代码生成的简短原因 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Ledger | [internal/doctor/llm/cost.go:111](../../internal/doctor/llm/cost.go) | Ledger 必须原子地检查总额并预留；两个同时运行的诊断不能各花掉同一份余额 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Error | [internal/doctor/llm/cost.go:20](../../internal/doctor/llm/cost.go) | Error 用具名字符串实现 error，让错误分类是不能被重新赋值的常量 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Error.Error | [internal/doctor/llm/cost.go:23](../../internal/doctor/llm/cost.go) | Error 满足标准库 error 接口；errors.Is 仍可直接比较同一个常量 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Prices | [internal/doctor/llm/cost.go:35](../../internal/doctor/llm/cost.go) | Prices 保存每百万 token 的单价，单位同样是百万分之一账户货币单位 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Money | [internal/doctor/llm/cost.go:43](../../internal/doctor/llm/cost.go) | Money 将最多六位小数的金额转换为整数，不接受科学计数法或负数 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Prices.Cost | [internal/doctor/llm/cost.go:69](../../internal/doctor/llm/cost.go) | Cost 分别向上取整，不因微小舍入把未花的钱误认为仍可使用 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Prices.Charge | [internal/doctor/llm/cost.go:76](../../internal/doctor/llm/cost.go) | Charge 只信任供应商明确返回的命中数，剩余输入全部按未命中计算 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Prices.ReserveCost | [internal/doctor/llm/cost.go:84](../../internal/doctor/llm/cost.go) | ReserveCost 按最贵的输入类别预留 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | Reservation | [internal/doctor/llm/cost.go:90](../../internal/doctor/llm/cost.go) | Reservation 在网络发送之前落库，原文已经脱敏，不能包含 Authorization 头 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm (llm) | DeepSeekTariff | [internal/doctor/llm/tariff.go:17](../../internal/doctor/llm/tariff.go) | DeepSeekTariff 按官方北京时间工作日 9–12、14–18 点峰时规则，其他时间单价减半 | 医生 app 与 runner 装配模型；客户端调用计价和账本 |
| internal/doctor/llm/openai (openai) | *Client.attempt | [internal/doctor/llm/openai/client.go:130](../../internal/doctor/llm/openai/client.go) | attempt 在发送前记录预留额 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | *Client.settle | [internal/doctor/llm/openai/client.go:215](../../internal/doctor/llm/openai/client.go) | settle 保留取消信号中的追踪信息，只给记账半秒；失败时预留额已经在库中，不会丢账 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | Config | [internal/doctor/llm/openai/client.go:29](../../internal/doctor/llm/openai/client.go) | Config 的 Key 只能来自环境变量，调用者不能把整个配置写入日志 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | Client | [internal/doctor/llm/openai/client.go:39](../../internal/doctor/llm/openai/client.go) | Client 的依赖在启动时固定，账本与脱敏器由 app 注入 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | New | [internal/doctor/llm/openai/client.go:61](../../internal/doctor/llm/openai/client.go) | New 允许回环 HTTP 用于 httptest，真实地址必须 HTTPS；不会在构造时联网 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | *Client.Complete | [internal/doctor/llm/openai/client.go:89](../../internal/doctor/llm/openai/client.go) | Complete 每次重试都重新记账；尚未收到任何响应的网络错误可有限重试，取消立即退出 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | decodeResponse | [internal/doctor/llm/openai/protocol.go:23](../../internal/doctor/llm/openai/protocol.go) | decodeResponse 限制回复大小，保留接口 model；缺少该字段时才用请求模型 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | httpProblem | [internal/doctor/llm/openai/protocol.go:47](../../internal/doctor/llm/openai/protocol.go) | httpProblem 只判断已经收到的 HTTP 状态；没有响应的网络错误由 attempt 单独处理 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | retryAfter | [internal/doctor/llm/openai/protocol.go:65](../../internal/doctor/llm/openai/protocol.go) | retryAfter 支持秒数和 HTTP 日期；未提供时等待一秒，避免立即重试形成突发 | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/llm/openai (openai) | wait | [internal/doctor/llm/openai/protocol.go:77](../../internal/doctor/llm/openai/protocol.go) | wait 用 select 同时等待计时器和取消，不使用无法提前结束的 Sleep | Agent 或评委 → Client.Complete → attempt |
| internal/doctor/run (run) | Execution | [internal/doctor/run/execution.go:20](../../internal/doctor/run/execution.go) | Execution 的内容在启动时固定，各任务只持有引用，不会修改其他正在执行的任务 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | ExecutionStatus | [internal/doctor/run/execution.go:30](../../internal/doctor/run/execution.go) | ExecutionStatus 只含可公开的运行信息，不包含 Key、地址、提示或客户端对象 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.ExecutionStatuses | [internal/doctor/run/execution.go:38](../../internal/doctor/run/execution.go) | ExecutionStatuses 按账户分别查询，不能把两种货币加在同一个余额里 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.executionFor | [internal/doctor/run/execution.go:63](../../internal/doctor/run/execution.go) | executionFor 拒绝没有注册的配置，不能为了继续实验而悄悄换回默认模型 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | New | [internal/doctor/run/scheduler.go:103](../../internal/doctor/run/scheduler.go) | New 只检查配置，Start 完成数据库清理后才真正接受任务 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.Start | [internal/doctor/run/scheduler.go:123](../../internal/doctor/run/scheduler.go) | Start 先检查过时的断点，再清理过时告警；可恢复的任务会由循环接手 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.Wake | [internal/doctor/run/scheduler.go:139](../../internal/doctor/run/scheduler.go) | Wake 合并高频通知，后台仍定期轮询，因此丢掉重复唤醒不会丢任务 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.Close | [internal/doctor/run/scheduler.go:147](../../internal/doctor/run/scheduler.go) | Close 先阻止分配新名额，再取消所有在途任务，最后等待 goroutine 退出 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.loop | [internal/doctor/run/scheduler.go:161](../../internal/doctor/run/scheduler.go) | loop 每秒检查队列；通知只是加快响应，不承担持久化职责 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.dispatch | [internal/doctor/run/scheduler.go:182](../../internal/doctor/run/scheduler.go) | dispatch 清理过时任务后再看费用，这样额度用完也不会让旧告警永久卡在 pending | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.claim | [internal/doctor/run/scheduler.go:235](../../internal/doctor/run/scheduler.go) | claim 在一把锁内同时检查两种名额，避免两条任务都看见“还有一个空位” | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.execute | [internal/doctor/run/scheduler.go:254](../../internal/doctor/run/scheduler.go) | execute 将取消、预算和普通失败转换为有限状态；结束写入也有独立的短预算 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | Task | [internal/doctor/run/scheduler.go:27](../../internal/doctor/run/scheduler.go) | Task 只包含执行所需的最小字段，模型不能改变 ID 或 Target | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.Cancel | [internal/doctor/run/scheduler.go:329](../../internal/doctor/run/scheduler.go) | Cancel 取消正在运行的 context；排队任务直接以条件更新改为 cancelled | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.Status | [internal/doctor/run/scheduler.go:350](../../internal/doctor/run/scheduler.go) | Status 同时显示已经花掉的额度和单次预留不足的原因，页面不能只看到“没有新任务” | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | taskRecorder | [internal/doctor/run/scheduler.go:370](../../internal/doctor/run/scheduler.go) | taskRecorder 把 agent 的一步绑定到任务 ID，并在提交后通知订阅者 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | taskRecorder.Record | [internal/doctor/run/scheduler.go:377](../../internal/doctor/run/scheduler.go) | Record 先保存后发布，SSE 的 Last-Event-ID 才一定能在数据库里找到 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | *Scheduler.renew | [internal/doctor/run/scheduler.go:386](../../internal/doctor/run/scheduler.go) | renew 定期证明执行者仍在工作；一次明确失去租约就取消本机请求，不能再继续花钱 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | BudgetStatus | [internal/doctor/run/scheduler.go:39](../../internal/doctor/run/scheduler.go) | BudgetStatus 同时供调度器、页面和指标读取，金额是百万分之一账户货币单位 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | minTime | [internal/doctor/run/scheduler.go:429](../../internal/doctor/run/scheduler.go) | minTime 选较早的截止时间，续约查询本身也不能越过旧租约的保守期限 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | Repository | [internal/doctor/run/scheduler.go:49](../../internal/doctor/run/scheduler.go) | Repository 把状态与步骤保存放在同一个边界，假仓储可完整验证调度行为 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | Config | [internal/doctor/run/scheduler.go:62](../../internal/doctor/run/scheduler.go) | Config 保存进程内调度参数；Resolve 在任务开始时返回已固定系统的工具与说明 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | running | [internal/doctor/run/scheduler.go:80](../../internal/doctor/run/scheduler.go) | running 保存取消函数和系统名，mu 保护这张表与 stopped、blocked | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/run (run) | Scheduler | [internal/doctor/run/scheduler.go:87](../../internal/doctor/run/scheduler.go) | Scheduler 的 worker 都计入 WaitGroup，Close 等它们退出后才允许 app 关数据库 | app 装配 Scheduler；调度器领取后调用执行配置和 Agent |
| internal/doctor/store (store) | resolvedBefore | [internal/doctor/store/alerts.go:139](../../internal/doctor/store/alerts.go) | resolvedBefore 只在通知确实带结束时间时挡住迟到恢复；旧客户端缺时间时保持兼容 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | saveAlerts | [internal/doctor/store/alerts.go:149](../../internal/doctor/store/alerts.go) | saveAlerts 保留每条告警的最近快照；恢复通知不删除诊断，也不触发第二次模型调用 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.ReceiveAlerts | [internal/doctor/store/alerts.go:25](../../internal/doctor/store/alerts.go) | ReceiveAlerts 先清理敏感文字，再在一个事务里更新分组与快照 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.receiveGroup | [internal/doctor/store/alerts.go:83](../../internal/doctor/store/alerts.go) | receiveGroup 的 INSERT 先保证有可锁的一行，resolved 先到也不需要创建空诊断 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.DecideApproval | [internal/doctor/store/approvals.go:131](../../internal/doctor/store/approvals.go) | DecideApproval 只有一个请求能把仍有效的 pending 单改掉，HTTP 重试不会二次决定 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.ClaimApproval | [internal/doctor/store/approvals.go:181](../../internal/doctor/store/approvals.go) | ClaimApproval 把批准与执行分开记录，已在执行的单子只能由当前诊断租约的持有者恢复 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.ApprovalConnection | [internal/doctor/store/approvals.go:222](../../internal/doctor/store/approvals.go) | ApprovalConnection 在发送 DDL 前保存连接编号，恢复者可以先检查旧操作是否还在运行 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.FinishApproval | [internal/doctor/store/approvals.go:243](../../internal/doctor/store/approvals.go) | FinishApproval 只有当前租约能保存结果；旧执行者的迟到回复不会盖掉恢复者的核对 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.FindApproval | [internal/doctor/store/approvals.go:26](../../internal/doctor/store/approvals.go) | FindApproval 总是同时限定诊断编号，不能拿另一个诊断的审批结果接续当前任务 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.ExpireApprovals | [internal/doctor/store/approvals.go:278](../../internal/doctor/store/approvals.go) | ExpireApprovals 不需要人在页面点拒绝；到期后任务会被调度器继续，让模型得到明确的未执行结果 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.expireApproval | [internal/doctor/store/approvals.go:308](../../internal/doctor/store/approvals.go) | expireApproval 与人工决定使用相同的锁顺序，过期状态和页面事件一起落库 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | scanner | [internal/doctor/store/approvals.go:35](../../internal/doctor/store/approvals.go) | scanner 让 QueryRow 和 Rows 使用同一套解码，避免列表与执行读出不同参数 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | scanApproval | [internal/doctor/store/approvals.go:38](../../internal/doctor/store/approvals.go) | scanApproval 不给损坏的 JSON 补默认值，无法准确恢复参数时必须停下 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Approvals | [internal/doctor/store/approvals.go:66](../../internal/doctor/store/approvals.go) | Approvals 返回本次诊断的审批单，页面只能通过它展示已经保存的内容 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.CreateApproval | [internal/doctor/store/approvals.go:84](../../internal/doctor/store/approvals.go) | CreateApproval 在同一事务中保存计划和页面步骤；崩溃时不会只保存其中一半 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Ledger.Settle | [internal/doctor/store/budget.go:131](../../internal/doctor/store/budget.go) | Settle 可重复调用但只结算一次，避免重试数据库写入时重复退钱 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Ledger.Status | [internal/doctor/store/budget.go:197](../../internal/doctor/store/budget.go) | Status 读的是整个真实账户，包含其他诊断预留但还没结算的钱 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Ledger | [internal/doctor/store/budget.go:22](../../internal/doctor/store/budget.go) | Ledger 保存不随诊断变化的累计账户，单位统一为百万分之一元 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.NewLedger | [internal/doctor/store/budget.go:30](../../internal/doctor/store/budget.go) | NewLedger 创建空账本但绝不清零已有记录，配置降低上限也不会隐藏已花费用 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.NewCurrencyLedger | [internal/doctor/store/budget.go:35](../../internal/doctor/store/budget.go) | NewCurrencyLedger 拒绝将已有人民币账户重新解释成美元；账户名相同也不能改变金额单位 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Ledger.Reserve | [internal/doctor/store/budget.go:59](../../internal/doctor/store/budget.go) | Reserve 先锁预算再保存请求，两个动作一起提交后调用方才能联网 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Diagnosis | [internal/doctor/store/diagnoses.go:26](../../internal/doctor/store/diagnoses.go) | Diagnosis 是列表所需的最小摘要，不包含账号或工具凭据 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Detail | [internal/doctor/store/diagnoses.go:39](../../internal/doctor/store/diagnoses.go) | Detail 的嵌入字段使摘要与列表保持一致，避免维护两套字段映射 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.List | [internal/doctor/store/diagnoses.go:63](../../internal/doctor/store/diagnoses.go) | List 只把筛选值绑定到参数；排序和 SQL 结构由代码固定 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Detail | [internal/doctor/store/diagnoses.go:97](../../internal/doctor/store/diagnoses.go) | Detail 只按固定 ID 查一次任务，再取相关告警，不允许调用者指定表或字段 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.baselineVersions | [internal/doctor/store/eval_baseline_amendment.go:120](../../internal/doctor/store/eval_baseline_amendment.go) | baselineVersions 接在旧校时修订之后，旧两份版本记录仍各自存在，不能跳过链条 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalBaselineParent | [internal/doctor/store/eval_baseline_amendment.go:136](../../internal/doctor/store/eval_baseline_amendment.go) | EvalBaselineParent 让同一批准命令可重试；已有记录时不能把新版本再次当作旧版本批准 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.AmendEvalBaseline | [internal/doctor/store/eval_baseline_amendment.go:19](../../internal/doctor/store/eval_baseline_amendment.go) | AmendEvalBaseline 只供已获用户授权的宿主机命令调用，不开放给医生或模型 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvalRun | [internal/doctor/store/eval_budget.go:24](../../internal/doctor/store/eval_budget.go) | EvalRun 是批次的公开进度，配置只保存版本与运行安排，不保存 Key | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.CreateEvalRun | [internal/doctor/store/eval_budget.go:35](../../internal/doctor/store/eval_budget.go) | CreateEvalRun 相同参数可重复调用；更改配置要用新批次，不能覆盖之前的失败记录 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalRun | [internal/doctor/store/eval_budget.go:59](../../internal/doctor/store/eval_budget.go) | EvalRun 只读这一批的 token 状态；费用必须另查对应币种的 Ledger | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | reserveEvalTokens | [internal/doctor/store/eval_budget.go:66](../../internal/doctor/store/eval_budget.go) | reserveEvalTokens 由 Reserve 在插入费用尝试后调用；任何一步失败，整份费用预留也回滚 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | settleEvalTokens | [internal/doctor/store/eval_budget.go:93](../../internal/doctor/store/eval_budget.go) | settleEvalTokens 与费用结算共用“只结算一次”检查；未知量不退，已知缓存 token 不重复相加 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalCalibration | [internal/doctor/store/eval_calibration.go:15](../../internal/doctor/store/eval_calibration.go) | EvalCalibration 没有抽过时保留 sql.ErrNoRows，让调用方明确区分读取错误 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.SaveEvalCalibration | [internal/doctor/store/eval_calibration.go:27](../../internal/doctor/store/eval_calibration.go) | SaveEvalCalibration 不覆盖先前的随机样本，也不允许未结束批次提前挑出好报告 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.AmendEvalClock | [internal/doctor/store/eval_clock_amendment.go:18](../../internal/doctor/store/eval_clock_amendment.go) | AmendEvalClock 由宿主机命令显式调用 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalVersions | [internal/doctor/store/eval_clock_amendment.go:42](../../internal/doctor/store/eval_clock_amendment.go) | EvalVersions 原版本仍是事实；只有匹配它的独立批准记录才能指定续跑版本 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.clockMakeupAllowed | [internal/doctor/store/eval_clock_amendment.go:58](../../internal/doctor/store/eval_clock_amendment.go) | clockMakeupAllowed 不把时钟异常推广为所有取消都可重跑；只认明确批准的那一条 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.RecordEvalExecution | [internal/doctor/store/eval_execution.go:17](../../internal/doctor/store/eval_execution.go) | RecordEvalExecution 只追加，不能给过去的报告换一个版本或抹掉先前的失败尝试 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.FinishEvalJudge | [internal/doctor/store/eval_judge.go:119](../../internal/doctor/store/eval_judge.go) | FinishEvalJudge 同时保存评分和终态；失败、费用、原回复都保留，不允许换成更好的一次 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvalJudge | [internal/doctor/store/eval_judge.go:23](../../internal/doctor/store/eval_judge.go) | EvalJudge 固定评委运行身份；不提供修改已经保存的评分输入的入口 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.PrepareEvalJudge | [internal/doctor/store/eval_judge.go:30](../../internal/doctor/store/eval_judge.go) | PrepareEvalJudge 在同一事务关联审计记录，同一个结果槽位只会产生一份评分 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.StartEvalJudge | [internal/doctor/store/eval_judge.go:74](../../internal/doctor/store/eval_judge.go) | StartEvalJudge 先保存 running，再允许客户端预留费用；崩溃后看到 running 不再重发 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.SavedEvalJudgeResponse | [internal/doctor/store/eval_judge.go:86](../../internal/doctor/store/eval_judge.go) | SavedEvalJudgeResponse 保留未结算预留，只有成功存入账本的回答可以恢复为评分材料 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalRuns | [internal/doctor/store/eval_listing.go:11](../../internal/doctor/store/eval_listing.go) | EvalRuns 不把账户 Key 或数据库连接配置带进页面，批次配置本来就只有公开版本信息 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.WithEvalLock | [internal/doctor/store/eval_lock.go:22](../../internal/doctor/store/eval_lock.go) | WithEvalLock 不等待另一个操作者，重复启动时直接说明已有执行者 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalReliability | [internal/doctor/store/eval_reliability.go:18](../../internal/doctor/store/eval_reliability.go) | EvalReliability 返回是否因基础设施失败而一次有效模型响应也没有拿到 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | infrastructureFailure | [internal/doctor/store/eval_reliability.go:52](../../internal/doctor/store/eval_reliability.go) | infrastructureFailure 使用有限白名单，取消、余额不足和损坏的成功回复不能借机补跑 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.PrepareEvalMakeup | [internal/doctor/store/eval_reliability.go:68](../../internal/doctor/store/eval_reliability.go) | PrepareEvalMakeup 只接受原槽位 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalResults | [internal/doctor/store/eval_results.go:122](../../internal/doctor/store/eval_results.go) | EvalResults 先取小编号列表再逐个读结果，避免占着连接池里的游标申请第二条连接 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvalResult | [internal/doctor/store/eval_results.go:20](../../internal/doctor/store/eval_results.go) | EvalResult 包含运行位置和原始输入；Data 在诊断结束前为空 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalDiagnosisState | [internal/doctor/store/eval_results.go:39](../../internal/doctor/store/eval_results.go) | EvalDiagnosisState 的轮询只取一个状态字段，不反复加载可能很长的模型对话 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.PrepareEvalResult | [internal/doctor/store/eval_results.go:46](../../internal/doctor/store/eval_results.go) | PrepareEvalResult 不覆盖旧告警 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.readEvalResult | [internal/doctor/store/eval_results.go:71](../../internal/doctor/store/eval_results.go) | readEvalResult 在一个查询里取回完整槽位，避免分别读取时拼出不同版本的快照 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.AttachEvalDiagnosis | [internal/doctor/store/eval_results.go:80](../../internal/doctor/store/eval_results.go) | AttachEvalDiagnosis 只接受同批次、同幂等编号的任务，不能把另一次好结果移过来替换失败 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.FinishEvalResult | [internal/doctor/store/eval_results.go:99](../../internal/doctor/store/eval_results.go) | FinishEvalResult 失败也保存；重试允许相同结果，不允许用新摘要覆盖旧结果 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvalTrial | [internal/doctor/store/eval_trials.go:20](../../internal/doctor/store/eval_trials.go) | EvalTrial 是一个场景的一次注入；五个实验组共享这份恢复进度 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvalTrial | [internal/doctor/store/eval_trials.go:31](../../internal/doctor/store/eval_trials.go) | EvalTrial 对相同位置只创建一次，返回原值而不是调用者猜测的状态 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | trialTransition | [internal/doctor/store/eval_trials.go:45](../../internal/doctor/store/eval_trials.go) | trialTransition 只允许相邻进度 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.AdvanceEvalTrial | [internal/doctor/store/eval_trials.go:50](../../internal/doctor/store/eval_trials.go) | AdvanceEvalTrial 用旧状态作条件，两个进程不能同时推进同一轮 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.InterruptEval | [internal/doctor/store/eval_trials.go:74](../../internal/doctor/store/eval_trials.go) | InterruptEval 保存每次失败，不覆盖旧原因；内容仍先经过统一脱敏 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.SetEvalRunState | [internal/doctor/store/eval_trials.go:80](../../internal/doctor/store/eval_trials.go) | SetEvalRunState 只更新执行进度，不能清空 token 账本自己设置的暂停原因 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Evaluation | [internal/doctor/store/evaluation.go:118](../../internal/doctor/store/evaluation.go) | Evaluation 用唯一键收回同一个任务；返回旧 ID 前核对配置，不能把不同实验误认成重试 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | sameJSON | [internal/doctor/store/evaluation.go:174](../../internal/doctor/store/evaluation.go) | sameJSON 忽略数据库的 JSON 排版变化，但保留内容差异 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvaluationMode | [internal/doctor/store/evaluation.go:24](../../internal/doctor/store/evaluation.go) | EvaluationMode 记录是谁暂停了自动诊断；必须由同一批评测负责恢复 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | EvaluationInput | [internal/doctor/store/evaluation.go:30](../../internal/doctor/store/evaluation.go) | EvaluationInput 只含实验安排和捕获的症状，标准答案始终留在评测器一侧 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EvaluationMode | [internal/doctor/store/evaluation.go:45](../../internal/doctor/store/evaluation.go) | EvaluationMode 不存在时返回关闭状态；数据库读取错误不能被当作“没在评测” | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.SetEvaluationMode | [internal/doctor/store/evaluation.go:55](../../internal/doctor/store/evaluation.go) | SetEvaluationMode 只在没有执行中或等待审批的任务时切换，防止模型配置跨过一条诊断 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Renew | [internal/doctor/store/leases.go:135](../../internal/doctor/store/leases.go) | Renew 的条件更新不允许续活已经过期的租约；迟到者必须退出，不能自行复活 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.staleLeases | [internal/doctor/store/leases.go:145](../../internal/doctor/store/leases.go) | staleLeases 和接手共用协调行，查出过时任务后不会误把刚刚获得新租约的任务结束掉 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Resume | [internal/doctor/store/leases.go:204](../../internal/doctor/store/leases.go) | Resume 先补齐“已经记账但没保存回复步骤”的空窗，再返回完整历史 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | requireLease | [internal/doctor/store/leases.go:24](../../internal/doctor/store/leases.go) | requireLease 必须在持有诊断行锁的事务中调用，检查和写入之间不能让别人接手 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.claimLease | [internal/doctor/store/leases.go:37](../../internal/doctor/store/leases.go) | claimLease 用一行短事务锁同时检查全局两个名额、系统一个名额，防止两进程各自超配 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | nullableKey | [internal/doctor/store/steps.go:114](../../internal/doctor/store/steps.go) | nullableKey 让旧记录和无须去重的状态事件使用 SQL NULL，避免空串互相冲突 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | readStep | [internal/doctor/store/steps.go:122](../../internal/doctor/store/steps.go) | readStep 在同一事务内返回已保存的完整结果，重试保存不会分配第二个步骤号 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.RecordStep | [internal/doctor/store/steps.go:22](../../internal/doctor/store/steps.go) | RecordStep 在数据库提交之后才发布事件，失败时不会产生只有页面看得到的虚假步骤 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.appendStep | [internal/doctor/store/steps.go:40](../../internal/doctor/store/steps.go) | appendStep 被普通步骤和状态转换共用，使终态事件也有可补发的数据库编号 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.publish | [internal/doctor/store/steps.go:85](../../internal/doctor/store/steps.go) | publish 不持有数据库锁；回调只能做有界的内存投递，慢浏览器不能拖住事务 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.StepsAfter | [internal/doctor/store/steps.go:92](../../internal/doctor/store/steps.go) | StepsAfter 每次最多读取一百步，SSE 会继续翻页，不把所有历史一次塞进内存 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Store | [internal/doctor/store/store.go:35](../../internal/doctor/store/store.go) | Store 不向 HTTP 层暴露 sql.DB，接口不能随意拼接 SQL | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Config | [internal/doctor/store/store.go:41](../../internal/doctor/store/store.go) | Config 把时间和通知作为依赖注入，测试无需真的等待分组过期 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.ForExecution | [internal/doctor/store/store.go:57](../../internal/doctor/store/store.go) | ForExecution 共用连接池和通知，只固定另一套币种与账户，不修改原仓储配置 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | New | [internal/doctor/store/store.go:64](../../internal/doctor/store/store.go) | New 只装配已有池，连接检查和生命周期由 app 统一负责 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | NewConfigured | [internal/doctor/store/store.go:67](../../internal/doctor/store/store.go) | NewConfigured 保留无网络的构造过程，缺省时间与脱敏器也适用于集成测试 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | Migrate | [internal/doctor/store/store.go:91](../../internal/doctor/store/store.go) | Migrate 复用命名锁和校验和，两个误启动的进程也不会同时执行同一迁移 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | newID | [internal/doctor/store/store.go:96](../../internal/doctor/store/store.go) | newID 用随机 16 字节编码为 32 位十六进制；ID 不包含账号或告警内容 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.EndTask | [internal/doctor/store/tasks.go:103](../../internal/doctor/store/tasks.go) | EndTask 用累计结果覆盖最终计数，失败时也保存已经发生的用量 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.CancelPending | [internal/doctor/store/tasks.go:123](../../internal/doctor/store/tasks.go) | CancelPending 和启动竞争同一行的 pending 状态，只有先拿到行锁的操作成功 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.changeState | [internal/doctor/store/tasks.go:141](../../internal/doctor/store/tasks.go) | changeState 仅接受本文件拼出的字段片段，外部内容始终用参数绑定 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Manual | [internal/doctor/store/tasks.go:24](../../internal/doctor/store/tasks.go) | Manual 只负责保存已由接口确认存在的系统，症状先脱敏再落库 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Pending | [internal/doctor/store/tasks.go:35](../../internal/doctor/store/tasks.go) | Pending 先选出每个系统最老的一条，避免某系统大量排队挡住另一系统 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Recover | [internal/doctor/store/tasks.go:53](../../internal/doctor/store/tasks.go) | Recover 只结束中断太久的任务；有效的过期租约由调度器接手并重建对话 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.Expire | [internal/doctor/store/tasks.go:61](../../internal/doctor/store/tasks.go) | Expire 只清理告警触发的旧 pending，手动任务不会被这条告警时效规则取消 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.endSelected | [internal/doctor/store/tasks.go:66](../../internal/doctor/store/tasks.go) | endSelected 先关闭结果集再逐条写入，连接池较小时也不占着读连接等待写连接 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/store (store) | *Store.StartTask | [internal/doctor/store/tasks.go:98](../../internal/doctor/store/tasks.go) | StartTask 记录本次选用的模型与提示版本，不能沿用上一个进程的隐含配置 | 医生调度、HTTP 查询或宿主机 runner 经 Store 调用 |
| internal/doctor/tools (tools) | *Runbooks.Search | [internal/doctor/tools/runbooks.go:123](../../internal/doctor/tools/runbooks.go) | Search 使用固定 BM25 参数，不从当前实验的正确答案学习权重 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | *Registry.WithRunbooks | [internal/doctor/tools/runbooks.go:173](../../internal/doctor/tools/runbooks.go) | WithRunbooks 返回独立菜单；基础目录和 MCP 菜单不会因此得到 E2 专用工具 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | searchWords | [internal/doctor/tools/runbooks.go:183](../../internal/doctor/tools/runbooks.go) | searchWords 对英文保留单词，对中文使用相邻双字；不依赖词典，也不会下载分词模型 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | Runbooks | [internal/doctor/tools/runbooks.go:28](../../internal/doctor/tools/runbooks.go) | Runbooks 构造后不修改；所有诊断可以共用同一份文档和词频统计 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | runbookChunk | [internal/doctor/tools/runbooks.go:36](../../internal/doctor/tools/runbooks.go) | runbookChunk 只保存手册正文，程序不会替它生成故障结论 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | RunbookHit | [internal/doctor/tools/runbooks.go:44](../../internal/doctor/tools/runbooks.go) | RunbookHit 的分数只是关键词相关程度，不能解释成诊断置信度 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | LoadRunbooks | [internal/doctor/tools/runbooks.go:53](../../internal/doctor/tools/runbooks.go) | LoadRunbooks 拒绝子目录、符号链接和过大的文件，不能借手册目录间接读入仓库答案 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | *Registry.ForDiagnosis | [internal/doctor/tools/window.go:24](../../internal/doctor/tools/window.go) | ForDiagnosis 先筛选菜单，再给有时间范围的工具绑定起点 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/doctor/tools (tools) | diagnosisWindow | [internal/doctor/tools/window.go:48](../../internal/doctor/tools/window.go) | diagnosisWindow 只补缺省参数，显式的即时查询（0 秒）和更早的查询都不会被覆盖 | app 装配工具注册表；Agent 和 doctorctl 经 Registry 调用 |
| internal/eval/judge (judge) | CheckResponse | [internal/eval/judge/grade.go:103](../../internal/eval/judge/grade.go) | CheckResponse 也供崩溃恢复使用，已结算回复与新回复必须经过相同的校验 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Check | [internal/eval/judge/grade.go:111](../../internal/eval/judge/grade.go) | Check 不接受未知字段、重复项目、编造步骤或改写规则项；错误不转换成零分 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | RubricHash | [internal/eval/judge/grade.go:174](../../internal/eval/judge/grade.go) | RubricHash 记录实际字节，版本名没改但正文被改过也能被开跑检查发现 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Input | [internal/eval/judge/grade.go:37](../../internal/eval/judge/grade.go) | Input 不含模型名字和实验组，避免评委根据供应商品牌偏好评分 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Observation | [internal/eval/judge/grade.go:47](../../internal/eval/judge/grade.go) | Observation 留下判断所需内容，去掉模型名和 token，避免无关线索影响评分 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Grade | [internal/eval/judge/grade.go:57](../../internal/eval/judge/grade.go) | Grade 把机制、证据和不当写操作分开，不用一个总分掩盖差异 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Observations | [internal/eval/judge/grade.go:66](../../internal/eval/judge/grade.go) | Observations 不发送大模型自己的思考过程，避免把“打算查询”混成“真的查到了” | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Evaluate | [internal/eval/judge/grade.go:82](../../internal/eval/judge/grade.go) | Evaluate 只发起一次逻辑评分；供应商网络重试仍由同一个收费客户端负责审计 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Mock | [internal/eval/judge/mock.go:20](../../internal/eval/judge/mock.go) | Mock 只实现评委协议，不替代医生的诊断脚本 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | Mock.Complete | [internal/eval/judge/mock.go:23](../../internal/eval/judge/mock.go) | Complete 所有结论都写明固定脚本，不能用参考答案给自己制造正确结果 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | pendingMilestone | [internal/eval/judge/protocol.go:19](../../internal/eval/judge/protocol.go) | pendingMilestone 保留完整列表中的原编号，删除规则项后不能重新从一开始编号 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | pendingMilestones | [internal/eval/judge/protocol.go:25](../../internal/eval/judge/protocol.go) | pendingMilestones 在付费前检查规则编号，内部材料损坏时不能交给模型猜 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/judge (judge) | judgeRequest | [internal/eval/judge/protocol.go:43](../../internal/eval/judge/protocol.go) | judgeRequest 只发送明确的待评列表，规则结果作为只读背景保留 | runner.grader.gradeSlot → Evaluate → Check |
| internal/eval/report (report) | Agreement | [internal/eval/report/calibration.go:117](../../internal/eval/report/calibration.go) | Agreement 缺一份人类分数都返回错误，不用评委分数补空白 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | candidate | [internal/eval/report/calibration.go:24](../../internal/eval/report/calibration.go) | candidate 只参与抽样；这些分层标签不会展示在盲评表里 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | SelectCalibration | [internal/eval/report/calibration.go:27](../../internal/eval/report/calibration.go) | SelectCalibration 保留所有候选，不按“答得好”排序；各层中先随机再选择 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | BlindCase | [internal/eval/report/calibration.go:82](../../internal/eval/report/calibration.go) | BlindCase 的数据来自原诊断和通用评分参考，没有评委分数字段 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | Blind | [internal/eval/report/calibration.go:91](../../internal/eval/report/calibration.go) | Blind 只生成空表 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | Markdown | [internal/eval/report/markdown.go:21](../../internal/eval/report/markdown.go) | Markdown 不在导出时重新评分，相同数据库快照应当生成相同的表格 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | cell | [internal/eval/report/markdown.go:70](../../internal/eval/report/markdown.go) | cell 只整理表格分隔符，不改报告的数据库原文 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | gradingRows | [internal/eval/report/markdown.go:75](../../internal/eval/report/markdown.go) | gradingRows 保留每份机制理由、评分失败和错误说明，人工校准之后再补“评分有误”的分析 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | measurement | [internal/eval/report/statistics.go:22](../../internal/eval/report/statistics.go) | measurement 的样本数随有效观测增长，未知时长和未知评分不塞进均值 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | *measurement.add | [internal/eval/report/statistics.go:28](../../internal/eval/report/statistics.go) | add 接收一项已经确认存在的观测，第一项同时初始化上下界 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | measurement.text | [internal/eval/report/statistics.go:40](../../internal/eval/report/statistics.go) | text 同时展示平均值、波动范围和 n，避免把一份有效报告说成三次结果 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | judgementView | [internal/eval/report/statistics.go:48](../../internal/eval/report/statistics.go) | judgementView 只读取需要汇总的字段，原完整评分仍保存在数据库和明细里 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | statisticsRow | [internal/eval/report/statistics.go:61](../../internal/eval/report/statistics.go) | statisticsRow 的医生费用只有一种币种；评委人民币另列，不做总金额相加 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | Statistics | [internal/eval/report/statistics.go:69](../../internal/eval/report/statistics.go) | Statistics 分层后再输出，不把十三种不同问题混成一个笼统准确率 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | Summary | [internal/eval/report/summary.go:19](../../internal/eval/report/summary.go) | Summary 同时保留定量数据和原报告，之后评委只能对实际存在的结果评分 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/report (report) | Summarize | [internal/eval/report/summary.go:46](../../internal/eval/report/summary.go) | Summarize 将步骤与开始时的模型逐一比较；实际模型变化的诊断不能归到单一模型名下 | runner 的 export / calibrate；评分后的汇总与导出 |
| internal/eval/runner (runner) | clockPriorHash | [internal/eval/runner/amendment.go:39](../../internal/eval/runner/amendment.go) | clockPriorHash 将白名单文件暂按旧 Git 内容参与计算；不改磁盘，不还原用户的修改 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | amendClock | [internal/eval/runner/amendment.go:68](../../internal/eval/runner/amendment.go) | amendClock 不创建模型任务，保留原清单、顺序和费用，再追加新的精确版本记录 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | advanceTrial | [internal/eval/runner/automatic.go:103](../../internal/eval/runner/automatic.go) | advanceTrial 每次只前进一段 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | matchRecord | [internal/eval/runner/automatic.go:186](../../internal/eval/runner/automatic.go) | matchRecord 必须匹配同一次注入；同名场景在另一天重注入也不能接上旧配对 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | waitBaseline | [internal/eval/runner/automatic.go:198](../../internal/eval/runner/automatic.go) | waitBaseline 等待的是 API 的真实状态，不用固定睡眠猜测三分钟观察期结束 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | waitTick | [internal/eval/runner/automatic.go:226](../../internal/eval/runner/automatic.go) | waitTick 只暂停当前流程，取消时立刻返回，不留下定时器或后台付费调用 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | automatic | [internal/eval/runner/automatic.go:26](../../internal/eval/runner/automatic.go) | automatic 从已冻结的批次续跑；命令本身不能创建另一份随机安排 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | sameBaselineBehavior | [internal/eval/runner/baseline_amendment.go:120](../../internal/eval/runner/baseline_amendment.go) | sameBaselineBehavior 只忽略本次明确获准变化的代码提交与哈希，其他清单字段全部相等 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | baselinePriorHash | [internal/eval/runner/baseline_amendment.go:43](../../internal/eval/runner/baseline_amendment.go) | baselinePriorHash 用前一提交的字节核对完整旧哈希，不修改工作区里的任何文件 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | amendBaseline | [internal/eval/runner/baseline_amendment.go:64](../../internal/eval/runner/baseline_amendment.go) | amendBaseline 先证实原故障已经恢复；不能在400QPS还运行时把轮次退回准备状态 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | calibrate | [internal/eval/runner/calibration.go:23](../../internal/eval/runner/calibration.go) | calibrate 抽样有数据库幂等记录，重新导出不能重新挑一套容易取得一致的报告 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | prepare | [internal/eval/runner/cli.go:170](../../internal/eval/runner/cli.go) | prepare 先核对健康环境，再保存版本和暂停状态；此命令本身不会调用付费模型 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | pilot | [internal/eval/runner/cli.go:204](../../internal/eval/runner/cli.go) | pilot 不负责注入，防止第一停点之前误启动正式矩阵 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | waitDiagnosis | [internal/eval/runner/cli.go:303](../../internal/eval/runner/cli.go) | waitDiagnosis 停止等待时也请求取消，不能让命令行退出后留下一条无人看管的付费请求 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | finish | [internal/eval/runner/cli.go:354](../../internal/eval/runner/cli.go) | finish 从真实接口读取开放期，未结束时说明截止时间；不自动批准操作或缩短观察期 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | command | [internal/eval/runner/cli.go:36](../../internal/eval/runner/cli.go) | command 只含公开运行安排；Key、数据库密码和登录令牌不接受命令行传值 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | RunCLI | [internal/eval/runner/cli.go:47](../../internal/eval/runner/cli.go) | RunCLI 先解析命令再连接依赖，拼错参数不会触发任何模型任务 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *doctorClient.runtime | [internal/eval/runner/client.go:124](../../internal/eval/runner/client.go) | runtime 读取正在运行的配置，防止评测器以为是 Terra、医生却仍在 Mock | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *doctorClient.fault | [internal/eval/runner/client.go:131](../../internal/eval/runner/client.go) | fault 通过用户指定的系统列表接口检查开放期，不用固定 sleep 猜告警是否已恢复 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *doctorClient.alerts | [internal/eval/runner/client.go:149](../../internal/eval/runner/client.go) | alerts 经过医生现有注册表；失败或截断时停止试跑，不拿残缺输入当完整对照 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *doctorClient.cancelDiagnosis | [internal/eval/runner/client.go:161](../../internal/eval/runner/client.go) | cancelDiagnosis 用原来的取消入口，中断信号会继续传到在途模型 HTTP 请求 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | doctorClient | [internal/eval/runner/client.go:30](../../internal/eval/runner/client.go) | doctorClient 的令牌只留在本进程，不写入评测配置 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | runtimeStatus | [internal/eval/runner/client.go:38](../../internal/eval/runner/client.go) | runtimeStatus 同时核对模型、模式和当前币种，不能仅看宿主机环境变量 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | newDoctorClient | [internal/eval/runner/client.go:46](../../internal/eval/runner/client.go) | newDoctorClient 不读取任何模型 Key，登录仅用于已有的医生 API | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *doctorClient.request | [internal/eval/runner/client.go:82](../../internal/eval/runner/client.go) | request 的正文有界；遇到非成功响应不输出可能夹带认证信息的服务端原文 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | checkClock | [internal/eval/runner/clock.go:25](../../internal/eval/runner/clock.go) | checkClock 在创建付费任务前完成整段检查；采样延迟不等同于时钟跳变 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | inspectClock | [internal/eval/runner/clock.go:30](../../internal/eval/runner/clock.go) | inspectClock 让独立预检和正式评测使用同一套判断，输出样本方便事后核对 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | clockGuardKey | [internal/eval/runner/clock.go:64](../../internal/eval/runner/clock.go) | clockGuardKey 只标记本包已经启动的守卫，嵌套等待不再启动重复的 Docker 采样 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | clockMonitorKey | [internal/eval/runner/clock.go:67](../../internal/eval/runner/clock.go) | clockMonitorKey 让多段一分钟采样共享十分钟历史，嵌套 matrix 也不能把计数清零 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | recordClockEvent | [internal/eval/runner/clock.go:70](../../internal/eval/runner/clock.go) | recordClockEvent 保存允许继续的小修正；关闭文件后才返回，进程退出也不会只剩内存 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | guardClock | [internal/eval/runner/clock.go:92](../../internal/eval/runner/clock.go) | guardClock 每段检查后继续下一段，共享十分钟历史；暂停时保留具体原因 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | checkBaseline | [internal/eval/runner/environment.go:25](../../internal/eval/runner/environment.go) | checkBaseline 只发送无凭据的 GET，不把医生登录令牌带到压测器 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | memoryBytes | [internal/eval/runner/environment.go:30](../../internal/eval/runner/environment.go) | memoryBytes 在试跑和等待中重复检查；不根据容器上限之和猜测实际占用 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | memorySize | [internal/eval/runner/environment.go:72](../../internal/eval/runner/environment.go) | memorySize 按 docker stats 输出的明确单位换算，未知单位必须报错，不能当成零 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | recordEvaluator | [internal/eval/runner/execution.go:15](../../internal/eval/runner/execution.go) | recordEvaluator 发生在动作前；写审计失败时不继续注入或调用付费模型 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | gradeInput | [internal/eval/runner/grading.go:120](../../internal/eval/runner/grading.go) | gradeInput 保留所有实际观察，不把医生的思考文本或模型品牌发给评委 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | *grader.gradeSlot | [internal/eval/runner/grading.go:129](../../internal/eval/runner/grading.go) | gradeSlot 对恢复的回复也重新做引用校验，但不会再次付费；原始失败仍留在医生记录里 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | grader | [internal/eval/runner/grading.go:33](../../internal/eval/runner/grading.go) | grader 不含靶场客户端，唯一的外部模型入口已经绑定固定账户 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | gradedResult | [internal/eval/runner/grading.go:40](../../internal/eval/runner/grading.go) | gradedResult 失败也保留；coverage 只在评分通过校验时有效 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | newGrader | [internal/eval/runner/grading.go:55](../../internal/eval/runner/grading.go) | newGrader 同时要求运行中的医生与本机显式允许真实调用，填 Key 本身不会启用收费 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | logProbe | [internal/eval/runner/injection.go:24](../../internal/eval/runner/injection.go) | logProbe 不排队请求 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | safetySummary | [internal/eval/runner/injection.go:53](../../internal/eval/runner/injection.go) | safetySummary 从审计读取执行次数，不用“理论上不可能执行”代替实际验收 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | checkWorkload | [internal/eval/runner/load.go:102](../../internal/eval/runner/load.go) | checkWorkload 覆盖注入、验证和恢复的过渡过程；这些动作也没有改工作集与工作者数的权限 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | checkLoad | [internal/eval/runner/load.go:115](../../internal/eval/runner/load.go) | checkLoad 在正常阶段与诊断阶段分别使用对应期望，四项必须全部一致 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | guardLoad | [internal/eval/runner/load.go:127](../../internal/eval/runner/load.go) | guardLoad 每两秒复核，漂移时取消当前诊断或评分；不修改压测器来掩盖漂移 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | loadSettings | [internal/eval/runner/load.go:23](../../internal/eval/runner/load.go) | loadSettings 是四项同时生效的约束，不只检查某一项是否落在宽泛范围内 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | normalLoad | [internal/eval/runner/load.go:31](../../internal/eval/runner/load.go) | normalLoad 同时用于注入前和恢复后，不能因某个场景改过流量就放宽正常状态 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | diagnosticLoad | [internal/eval/runner/load.go:34](../../internal/eval/runner/load.go) | diagnosticLoad 模拟动作依次生效的顺序；最后一次 load 才是开始诊断时应有的设置 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | scenarioLoad | [internal/eval/runner/load.go:51](../../internal/eval/runner/load.go) | scenarioLoad 读取受版本哈希保护的场景文件，不相信可被手工编辑的恢复文件中的期望值 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | readLoad | [internal/eval/runner/load.go:65](../../internal/eval/runner/load.go) | readLoad 读运行中的控制接口，配置文件里的默认值不能替代实际值 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | manifest | [internal/eval/runner/manifest.go:28](../../internal/eval/runner/manifest.go) | manifest 不保存场景答案，只有本次允许试跑的名字和版本 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | sourceHash | [internal/eval/runner/manifest.go:45](../../internal/eval/runner/manifest.go) | sourceHash 明确列出代码与部署目录，不遍历 .env、个人笔记或原始验收输出 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | sourceHashUsing | [internal/eval/runner/manifest.go:50](../../internal/eval/runner/manifest.go) | sourceHashUsing 只供校时变更核验旧快照；正式清单仍使用当前文件的完整哈希 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | validateOrders | [internal/eval/runner/paired.go:145](../../internal/eval/runner/paired.go) | validateOrders 检查整个矩阵，不只检查当前要运行的一行，避免续跑时遗漏一轮 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | equalJSON | [internal/eval/runner/paired.go:169](../../internal/eval/runner/paired.go) | equalJSON 忽略 MySQL JSON 的排版，但不忽略任何内容变化 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | freezePaired | [internal/eval/runner/paired.go:175](../../internal/eval/runner/paired.go) | freezePaired 在任何故障注入之前暂停自动诊断，防止同一次故障重复付费 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | matrix | [internal/eval/runner/paired.go:209](../../internal/eval/runner/paired.go) | matrix 保持故障不变，直到五个配置和各自的一次可靠性补跑都已结束 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | pairedSlots | [internal/eval/runner/paired.go:295](../../internal/eval/runner/paired.go) | pairedSlots 在创建任何模型任务前保存全部位置；半途重启沿用已保存的第一个输入 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | executeSlot | [internal/eval/runner/paired.go:335](../../internal/eval/runner/paired.go) | executeSlot 即使已有失败也不覆盖；只有明确的可靠性失败才创建一个新补跑槽位 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | pairedConfiguration | [internal/eval/runner/paired.go:36](../../internal/eval/runner/paired.go) | pairedConfiguration 只固定身份，不保存随调用增长的预算余额 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | trialOrder | [internal/eval/runner/paired.go:46](../../internal/eval/runner/paired.go) | trialOrder 记录同次故障的真实排序；各轮分别洗牌，顺序偏差不会固定落在同一组 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | pairedManifest | [internal/eval/runner/paired.go:53](../../internal/eval/runner/paired.go) | pairedManifest 同时冻结医生行为和评分规则；公开清单不含 Key 或账号密码 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | configurations | [internal/eval/runner/paired.go:63](../../internal/eval/runner/paired.go) | configurations 从运行中的白名单读取身份 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | shuffled | [internal/eval/runner/paired.go:81](../../internal/eval/runner/paired.go) | shuffled 用均匀随机数交换位置；它只在新批次创建时运行，续跑不重新随机 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/runner (runner) | pairedPlan | [internal/eval/runner/paired.go:95](../../internal/eval/runner/paired.go) | pairedPlan 对老批次只作比对，版本变化必须另开批次，不能把新旧结果拼在一起 | cmd/eval → RunCLI；matrix、automatic 与各命令辅助流程 |
| internal/eval/scoring (scoring) | column | [internal/eval/scoring/evidence.go:101](../../internal/eval/scoring/evidence.go) | column 只对元数据列名忽略大小写，业务表名和索引名仍精确比较 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | Coverage | [internal/eval/scoring/evidence.go:111](../../internal/eval/scoring/evidence.go) | Coverage 的分母是全部里程碑，不能因为没查询某项就把它从分母删除 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | Rule | [internal/eval/scoring/evidence.go:22](../../internal/eval/scoring/evidence.go) | Rule 由评测场景声明，医生的工具和提示都读不到这些标准答案 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | EvidenceResult | [internal/eval/scoring/evidence.go:30](../../internal/eval/scoring/evidence.go) | EvidenceResult 保留判断方式和引用，后续评委不得覆盖规则已经确定的结果 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | Component | [internal/eval/scoring/evidence.go:39](../../internal/eval/scoring/evidence.go) | Component 不忽略大小写、不接受子串，保证不同模型遵循同一份组件命名表 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | Evidence | [internal/eval/scoring/evidence.go:42](../../internal/eval/scoring/evidence.go) | Evidence 只检查成功且完整的表格 | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | fullScan | [internal/eval/scoring/evidence.go:73](../../internal/eval/scoring/evidence.go) | fullScan 按列名查找，不依赖 MySQL 不同版本的列顺序；只接受真实表名和 ALL | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/eval/scoring (scoring) | missingIndex | [internal/eval/scoring/evidence.go:87](../../internal/eval/scoring/evidence.go) | missingIndex 必须拿到正确列，不能把格式错误或截断后的空列表误认成“没有索引” | runner.gradeInput / gradeSlot；先判规则，再合并评委结果 |
| internal/platform/clockcheck (clockcheck) | Event | [internal/platform/clockcheck/policy.go:15](../../internal/platform/clockcheck/policy.go) | Event 是一条可写入台账的事实 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | Monitor | [internal/platform/clockcheck/policy.go:24](../../internal/platform/clockcheck/policy.go) | Monitor 由一个采样循环持有 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | NewMonitor | [internal/platform/clockcheck/policy.go:30](../../internal/platform/clockcheck/policy.go) | NewMonitor 允许单元测试直接捕获事件；真实运行传入持久化日志出口 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | *Monitor.observe | [internal/platform/clockcheck/policy.go:33](../../internal/platform/clockcheck/policy.go) | observe 使用严格“大于”：恰好回拨一秒、前跳五秒或三次修正都没有越界 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | reading | [internal/platform/clockcheck/sample.go:21](../../internal/platform/clockcheck/sample.go) | reading 同一次 time.Now 同时提供两种读数，避免分开读取引入调度误差 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | Sample | [internal/platform/clockcheck/sample.go:27](../../internal/platform/clockcheck/sample.go) | Sample 是纯本机检查，即使没有任何账号配置也能运行 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | Check | [internal/platform/clockcheck/sample.go:57](../../internal/platform/clockcheck/sample.go) | Check 供独立的一分钟检查使用；长跑用同一个 Monitor 跨采样段保留十分钟历史 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/clockcheck (clockcheck) | *Monitor.Check | [internal/platform/clockcheck/sample.go:62](../../internal/platform/clockcheck/sample.go) | Check 流式检查样本；日期修正按用户批准的分级规则处理，不等整段结束才报警 | doctor --clock-samples → Sample；runner.inspectClock → Check |
| internal/platform/httpx (httpx) | BodyLimit | [internal/platform/httpx/httpx.go:103](../../internal/platform/httpx/httpx.go) | BodyLimit 给每个请求安装有上限的 Reader | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | BindJSON | [internal/platform/httpx/httpx.go:123](../../internal/platform/httpx/httpx.go) | BindJSON 严格解析一个 JSON 值到非 nil 指针 target | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | Success | [internal/platform/httpx/httpx.go:148](../../internal/platform/httpx/httpx.go) | Success 写出契约规定的成功响应 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | Failure | [internal/platform/httpx/httpx.go:154](../../internal/platform/httpx/httpx.go) | Failure 写出公开错误并中断当前处理链 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | RequestID | [internal/platform/httpx/httpx.go:38](../../internal/platform/httpx/httpx.go) | RequestID 返回当前上下文里的标识 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | RequestIDs | [internal/platform/httpx/httpx.go:44](../../internal/platform/httpx/httpx.go) | RequestIDs 校验或生成请求标识，并记录安全的访问摘要 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | validRequestID | [internal/platform/httpx/httpx.go:65](../../internal/platform/httpx/httpx.go) | validRequestID 只接受简单 ASCII 标识 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | Recovery | [internal/platform/httpx/httpx.go:81](../../internal/platform/httpx/httpx.go) | Recovery 捕获当前请求 goroutine 的 panic，输出不含内部细节的 500 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| internal/platform/httpx (httpx) | clientAgent | [internal/platform/httpx/user_agent.go:21](../../internal/platform/httpx/user_agent.go) | clientAgent 保留不可信文字本身，但不能让它扩大每条日志或携带疑似认证信息 | 业务 HTTP 通用中间件记录裁剪过的访问日志 |
| tools/archcheck (main) | doctorMountProblems | [tools/archcheck/doctor.go:22](../../tools/archcheck/doctor.go) | doctorMountProblems 使用完整 YAML 解析，长格式挂载也不能绕过只接受固定字符串的规则 | archcheck 主流程，约束医生与答案的依赖 |
| tools/archcheck (main) | opsPortProblems | [tools/archcheck/doctor.go:74](../../tools/archcheck/doctor.go) | opsPortProblems 同时识别 Compose 两种端口格式，不能只靠“没有写 localhost”猜是否公开 | archcheck 主流程，约束医生与答案的依赖 |

## 测试导航

列出改动测试文件中的顶层测试，每个表格子例的判断理由保留在源码。测试由 go test 运行，单元与 CI 不调用真实模型。

| 测试 | 位置 | 验证什么 |
|---|---|---|
| TestProbeLog | [internal/chaos/action/probe_test.go:15](../../internal/chaos/action/probe_test.go) | TestProbeLog 观察实际收到的请求，确保攻击文字只在请求头里，不进入命令或 SQL |
| TestRepositoryScenarios | [internal/chaos/scenario/load_test.go:23](../../internal/chaos/scenario/load_test.go) | TestRepositoryScenarios 直接检查交付文件，避免测试样本正确而真实 YAML 已经写错 |
| TestDecodeRejectsInvalid | [internal/chaos/scenario/load_test.go:34](../../internal/chaos/scenario/load_test.go) | TestDecodeRejectsInvalid 逐项修改一份有效场景，确认坏输入在外部操作之前被拒绝 |
| TestStepBoundary | [internal/chaos/scenario/load_test.go:59](../../internal/chaos/scenario/load_test.go) | TestStepBoundary 即使直接调用执行器，也只能使用这些受限动作 |
| TestBurstBoundary | [internal/chaos/scenario/load_test.go:81](../../internal/chaos/scenario/load_test.go) | TestBurstBoundary 同时限制总批次和每批请求数，不允许用合法商品编号绕过资源上限 |
| TestInputEvidence | [internal/doctor/agent/input_test.go:20](../../internal/doctor/agent/input_test.go) | TestInputEvidence 先故意引用模型步骤，再改成输入步骤，保证校验不是“任意已有编号都行” |
| TestInputReplay | [internal/doctor/agent/input_test.go:52](../../internal/doctor/agent/input_test.go) | TestInputReplay 用一次中断的 E0 请求重建对话，仍然保留普通轮次和原来的输入编号 |
| TestReplayPartialTools | [internal/doctor/agent/replay_test.go:23](../../internal/doctor/agent/replay_test.go) | TestReplayPartialTools 复现两项并行查询只保存一项时退出，旧结果不能被重复查询覆盖 |
| TestUntrustedFrame | [internal/doctor/agent/replay_test.go:74](../../internal/doctor/agent/replay_test.go) | TestUntrustedFrame 尝试闭合 JSON 和 XML 标记，解码后仍必须完整落在唯一的数据字符串里 |
| TestReplayFinalInterrupted | [internal/doctor/agent/replay_test.go:91](../../internal/doctor/agent/replay_test.go) | TestReplayFinalInterrupted 区分最后报告中断一次与两次，恢复也必须遵守只修正一次的上限 |
| TestCLIUsesServer | [internal/doctor/app/cli_test.go:25](../../internal/doctor/app/cli_test.go) | TestCLIUsesServer 验证 CLI 不会自行吞掉或接受非法参数，最终结果由注册表所在的接口决定 |
| TestCLIRejectsRedirect | [internal/doctor/app/cli_test.go:54](../../internal/doctor/app/cli_test.go) | TestCLIRejectsRedirect 让第二个服务记录请求数，证明拒绝发生在令牌被转发之前 |
| TestCLICommand | [internal/doctor/app/cli_test.go:67](../../internal/doctor/app/cli_test.go) | TestCLICommand 只测入口语法；工具字段的校验留给服务端注册表 |
| TestClockEntry | [internal/doctor/app/cli_test.go:82](../../internal/doctor/app/cli_test.go) | TestClockEntry 即使服务凭据不存在，纯采样分支仍直接响应取消 |
| TestComparisonConfig | [internal/doctor/app/comparison_test.go:15](../../internal/doctor/app/comparison_test.go) | TestComparisonConfig 防止填了 Key 就隐式付费，也防止美元与人民币共用账户名 |
| TestEvaluationGroups | [internal/doctor/app/evaluation_test.go:20](../../internal/doctor/app/evaluation_test.go) | TestEvaluationGroups 每组都从同一基础菜单创建副本，顺序执行也不能污染下一组 |
| TestEvaluationEndpoints | [internal/doctor/httpapi/evaluations_test.go:35](../../internal/doctor/httpapi/evaluations_test.go) | TestEvaluationEndpoints 不因结果只读而放开身份边界，也不提供随意改分的 POST |
| TestTargetAuthentication | [internal/doctor/httpapi/router_test.go:165](../../internal/doctor/httpapi/router_test.go) | TestTargetAuthentication 把路径切换和令牌类型混用都当作未经授权处理 |
| TestLoginAndToolRoute | [internal/doctor/httpapi/router_test.go:189](../../internal/doctor/httpapi/router_test.go) | TestLoginAndToolRoute 从真实登录响应取令牌，再通过 HTTP 进入统一工具校验 |
| TestNotificationValidation | [internal/doctor/httpapi/router_test.go:222](../../internal/doctor/httpapi/router_test.go) | TestNotificationValidation 证明无效通知不会留下半条任务记录 |
| TestDeepSeekTariff | [internal/doctor/llm/tariff_test.go:14](../../internal/doctor/llm/tariff_test.go) | TestDeepSeekTariff 同一组峰时价格在各边界必须准确切换，不能改变缓存比例 |
| TestWireAudit | [internal/doctor/llm/openai/client_test.go:110](../../internal/doctor/llm/openai/client_test.go) | TestWireAudit 核对真实发送字节就是落库字节，模型名和缓存 token 不会丢失 |
| TestCancel | [internal/doctor/llm/openai/client_test.go:138](../../internal/doctor/llm/openai/client_test.go) | TestCancel 的 Transport 一直等 context，取消必须传到它，且不会再发第二次请求 |
| TestCancelBody | [internal/doctor/llm/openai/client_test.go:179](../../internal/doctor/llm/openai/client_test.go) | TestCancelBody 防止只有 HTTP.Do 的取消被记录，读正文中断却变成普通解析错误 |
| TestAttemptPrices | [internal/doctor/llm/openai/client_test.go:202](../../internal/doctor/llm/openai/client_test.go) | TestAttemptPrices 固定发送时的价格，返回时价格改变也不能改变这次结算 |
| TestHTTPPolicy | [internal/doctor/llm/openai/client_test.go:86](../../internal/doctor/llm/openai/client_test.go) | TestHTTPPolicy 用虚拟时钟检查确切等待时间，计算机忙不会改变结果 |
| TestNetworkAuditFailure | [internal/doctor/llm/openai/network_test.go:100](../../internal/doctor/llm/openai/network_test.go) | TestNetworkAuditFailure 防止把审计故障误当作可重试的供应商网络问题 |
| TestNetworkTimeoutOrigin | [internal/doctor/llm/openai/network_test.go:111](../../internal/doctor/llm/openai/network_test.go) | TestNetworkTimeoutOrigin 用虚拟时钟验证审计分类；次数与未知预留都不能随分类改变 |
| TestNetworkRetry | [internal/doctor/llm/openai/network_test.go:25](../../internal/doctor/llm/openai/network_test.go) | TestNetworkRetry 分开验证从未收到响应和响应已经开始的两种失败 |
| TestNetworkRetryBudget | [internal/doctor/llm/openai/network_test.go:75](../../internal/doctor/llm/openai/network_test.go) | TestNetworkRetryBudget 确认 retry 仍先预留费用，没有额度就不接触 HTTP |
| TestExecutionSelection | [internal/doctor/run/execution_test.go:16](../../internal/doctor/run/execution_test.go) | TestExecutionSelection 不接受模型自行改名、未知配置或给普通诊断附带评测配置 |
| TestExecutionBudgets | [internal/doctor/run/execution_test.go:39](../../internal/doctor/run/execution_test.go) | TestExecutionBudgets 使用虚拟时间等到实际调度完成，不能只检查选择函数返回了什么 |
| TestMockCannotEnablePaid | [internal/doctor/run/execution_test.go:61](../../internal/doctor/run/execution_test.go) | TestMockCannotEnablePaid 阻止未来装配错误把真实客户端藏在 Mock 页面后面 |
| TestExecutionDuration | [internal/doctor/run/scheduler_test.go:119](../../internal/doctor/run/scheduler_test.go) | TestExecutionDuration 用虚拟时钟验证实际执行两秒，取消后保存的也是两秒 |
| TestTransitions | [internal/doctor/run/scheduler_test.go:203](../../internal/doctor/run/scheduler_test.go) | TestTransitions 穷举全部状态对；终态回到 running 等情况必须拒绝 |
| TestSchedulerIsolation | [internal/doctor/run/scheduler_test.go:216](../../internal/doctor/run/scheduler_test.go) | TestSchedulerIsolation 五个 A 只占一个名额，B 同时运行；取消后全部 worker 都退出 |
| TestSchedulerBudgetAndExpiry | [internal/doctor/run/scheduler_test.go:254](../../internal/doctor/run/scheduler_test.go) | TestSchedulerBudgetAndExpiry 额度已满也要清理过时任务，而且不能启动任意付费调用 |
| TestCancelBeforeStart | [internal/doctor/run/scheduler_test.go:274](../../internal/doctor/run/scheduler_test.go) | TestCancelBeforeStart 取消已登记但尚未开始的任务，不能等下次轮询又把它拿起来执行 |
| TestBudgetStatusReason | [internal/doctor/run/scheduler_test.go:296](../../internal/doctor/run/scheduler_test.go) | TestBudgetStatusReason 防止一次预留失败盖掉更需要人工核对的持久停用原因 |
| TestRenewUnavailable | [internal/doctor/run/scheduler_test.go:343](../../internal/doctor/run/scheduler_test.go) | TestRenewUnavailable 用虚拟时钟等到租约边界，避免依赖真实机器的调度速度 |
| TestIntegrationEvalBaselineRejectStarted | [internal/doctor/store/eval_baseline_amendment_integration_test.go:117](../../internal/doctor/store/eval_baseline_amendment_integration_test.go) | TestIntegrationEvalBaselineRejectStarted 连没有关联诊断的占位槽位也拒绝重启，不能挑一次新故障重做 |
| TestIntegrationEvalBaselineAmendment | [internal/doctor/store/eval_baseline_amendment_integration_test.go:59](../../internal/doctor/store/eval_baseline_amendment_integration_test.go) | TestIntegrationEvalBaselineAmendment 同一批准只能重置一次，新注入的开始时间不能被第二次命令清空 |
| TestIntegrationEvalTokenBudget | [internal/doctor/store/eval_budget_integration_test.go:18](../../internal/doctor/store/eval_budget_integration_test.go) | TestIntegrationEvalTokenBudget 让费用余额充足而 token 不足，单独验证评测这一道限制 |
| TestIntegrationEvalClockAmendment | [internal/doctor/store/eval_clock_amendment_integration_test.go:17](../../internal/doctor/store/eval_clock_amendment_integration_test.go) | TestIntegrationEvalClockAmendment 不访问模型，只在测试库模拟时钟取消和人工批准 |
| TestIntegrationEvalExecution | [internal/doctor/store/eval_execution_integration_test.go:17](../../internal/doctor/store/eval_execution_integration_test.go) | TestIntegrationEvalExecution 在真实事务边界验证绑定，不只检查配置解析 |
| TestIntegrationPairedSlots | [internal/doctor/store/eval_execution_integration_test.go:59](../../internal/doctor/store/eval_execution_integration_test.go) | TestIntegrationPairedSlots 保持场景轮次、组别和故障时间相同，只改变模型执行配置 |
| TestIntegrationEvalLock | [internal/doctor/store/eval_judge_integration_test.go:114](../../internal/doctor/store/eval_judge_integration_test.go) | TestIntegrationEvalLock 第一执行者离开前第二个必须失败，离开后可以续跑 |
| TestIntegrationEvalJudge | [internal/doctor/store/eval_judge_integration_test.go:19](../../internal/doctor/store/eval_judge_integration_test.go) | TestIntegrationEvalJudge 用一份已失败的医生结果创建评分审计，模拟响应丢失后的恢复 |
| TestInfrastructureFailure | [internal/doctor/store/eval_reliability_test.go:11](../../internal/doctor/store/eval_reliability_test.go) | TestInfrastructureFailure 不依赖错误文案，也不会连接真实模型 |
| TestIntegrationEvalResults | [internal/doctor/store/eval_results_integration_test.go:19](../../internal/doctor/store/eval_results_integration_test.go) | TestIntegrationEvalResults 按“先保存槽位、后建任务、最后保存结果”的真实顺序逐处重试 |
| TestIntegrationEvalMakeup | [internal/doctor/store/eval_results_integration_test.go:77](../../internal/doctor/store/eval_results_integration_test.go) | TestIntegrationEvalMakeup 用失败审计模拟断网；重复申请也只有一个新槽位，不发送 HTTP |
| TestTrialTransition | [internal/doctor/store/eval_trials_integration_test.go:15](../../internal/doctor/store/eval_trials_integration_test.go) | TestTrialTransition 完成和未知状态没有后继，错误不会变成“已完成” |
| TestIntegrationEvalTrial | [internal/doctor/store/eval_trials_integration_test.go:36](../../internal/doctor/store/eval_trials_integration_test.go) | TestIntegrationEvalTrial 在每一步重新读取，模拟执行器退出后重建进程 |
| TestIntegrationEvaluationApproval | [internal/doctor/store/evaluation_integration_test.go:102](../../internal/doctor/store/evaluation_integration_test.go) | TestIntegrationEvaluationApproval 即使有人误改审批状态，执行器也要根据诊断来源拒绝领取 |
| TestIntegrationEvaluation | [internal/doctor/store/evaluation_integration_test.go:38](../../internal/doctor/store/evaluation_integration_test.go) | TestIntegrationEvaluation 同一配置重试只拿回旧 ID；切换模型不能把这份实验交给别的客户端 |
| TestIntegrationAlertRollback | [internal/doctor/store/store_integration_test.go:151](../../internal/doctor/store/store_integration_test.go) | TestIntegrationAlertRollback 直接调用仓储制造子表失败，证明任务不会独自留在库里 |
| TestIntegrationUserBootstrap | [internal/doctor/store/store_integration_test.go:165](../../internal/doctor/store/store_integration_test.go) | TestIntegrationUserBootstrap 第二次初始化不能悄悄改掉用户已经在用的密码摘要 |
| TestIntegrationAlertDeduplication | [internal/doctor/store/store_integration_test.go:92](../../internal/doctor/store/store_integration_test.go) | TestIntegrationAlertDeduplication 先按要求顺序推三次，再加并发推送检查竞态 |
| TestIntegrationDiagnosisTiming | [internal/doctor/store/timing_integration_test.go:19](../../internal/doctor/store/timing_integration_test.go) | TestIntegrationDiagnosisTiming 单段执行有独立耗时，跨进程接手则明确缺少完整测量 |
| TestRunbookSearch | [internal/doctor/tools/runbooks_test.go:16](../../internal/doctor/tools/runbooks_test.go) | TestRunbookSearch 用无关文本增加篇幅，证明 BM25 不会单纯偏爱较长的文章 |
| TestRunbookBounds | [internal/doctor/tools/runbooks_test.go:60](../../internal/doctor/tools/runbooks_test.go) | TestRunbookBounds 文件入口和工具入口分别校验，不能用一个测试代替另一道边界 |
| TestDiagnosisWindow | [internal/doctor/tools/window_test.go:15](../../internal/doctor/tools/window_test.go) | TestDiagnosisWindow 比较传给后端的实际参数，不只检查公开菜单中的说明 |
| TestDiagnosisMenu | [internal/doctor/tools/window_test.go:45](../../internal/doctor/tools/window_test.go) | TestDiagnosisMenu 证明 E0 的空菜单不只是展示限制，基础注册表的默认值也不会被改掉 |
| TestCheck | [internal/eval/judge/grade_test.go:22](../../internal/eval/judge/grade_test.go) | TestCheck 对每种伪造分别给出失败，模型打分错误不能静默算成零分 |
| TestEvaluate | [internal/eval/judge/grade_test.go:66](../../internal/eval/judge/grade_test.go) | TestEvaluate 验证初始告警可作 E0 证据；模型自己的文字不会混入观察列表 |
| TestRuleOverride | [internal/eval/judge/protocol_test.go:102](../../internal/eval/judge/protocol_test.go) | TestRuleOverride 重现评委把规则项重复返回的失败，不能为让检查通过而忽略它 |
| TestPendingProtocol | [internal/eval/judge/protocol_test.go:19](../../internal/eval/judge/protocol_test.go) | TestPendingProtocol 同时检查发出的列表和供应商看到的参数范围，避免只验其中一层 |
| TestInvalidRules | [internal/eval/judge/protocol_test.go:89](../../internal/eval/judge/protocol_test.go) | TestInvalidRules 内部规则损坏不能靠模型补救，更不能为此发付费请求 |
| TestCalibrationSelection | [internal/eval/report/calibration_test.go:19](../../internal/eval/report/calibration_test.go) | TestCalibrationSelection 用完整的三轮矩阵证明抽样不只落在一个场景或模型 |
| TestBlindAndAgreement | [internal/eval/report/calibration_test.go:54](../../internal/eval/report/calibration_test.go) | TestBlindAndAgreement 检查原样保留恶意文本但不能让它跳出代码块，人的分数保持空白 |
| TestStatistics | [internal/eval/report/statistics_test.go:17](../../internal/eval/report/statistics_test.go) | TestStatistics 不用混合模型或可靠性失败稀释质量分母，也不隐藏它们花的钱 |
| TestSummarize | [internal/eval/report/summary_test.go:23](../../internal/eval/report/summary_test.go) | TestSummarize 覆盖正常报告、混合模型、只有新模型步骤、拼错组件和没有报告 |
| TestMissingDuration | [internal/eval/report/summary_test.go:56](../../internal/eval/report/summary_test.go) | TestMissingDuration 不用老记录的起止时间推算未实际测量过的耗时 |
| TestMarkdown | [internal/eval/report/summary_test.go:66](../../internal/eval/report/summary_test.go) | TestMarkdown 未完成和失败各占一行；币种跟着费用显示，不能汇成无单位的总数 |
| TestMarkdownUnknown | [internal/eval/report/summary_test.go:77](../../internal/eval/report/summary_test.go) | TestMarkdownUnknown 不把未收到账单的零用量、倒序时间和默认提交标志包装成正常测量 |
| TestReliabilityRows | [internal/eval/report/summary_test.go:91](../../internal/eval/report/summary_test.go) | TestReliabilityRows 将原失败和补跑都导出，不能用成功那行代替原来花掉的请求 |
| TestAmendmentHash | [internal/eval/runner/amendment_test.go:15](../../internal/eval/runner/amendment_test.go) | TestAmendmentHash 核对恢复白名单快照后，其他一字节变化仍然可见 |
| TestMatchRecord | [internal/eval/runner/automatic_test.go:23](../../internal/eval/runner/automatic_test.go) | TestMatchRecord 同一个 S01 的另一次注入也必须拒绝，不能仅凭场景名续跑 |
| TestNestedClockGuard | [internal/eval/runner/automatic_test.go:54](../../internal/eval/runner/automatic_test.go) | TestNestedClockGuard 子流程结束不能把外层守卫停掉，外层仍持续保护下一组 |
| TestBaselineAmendmentScope | [internal/eval/runner/baseline_amendment_test.go:16](../../internal/eval/runner/baseline_amendment_test.go) | TestBaselineAmendmentScope 把一个允许变化的文件还原后，逐项加入不允许变化的文件 |
| TestBaselineBehaviorVersions | [internal/eval/runner/baseline_amendment_test.go:64](../../internal/eval/runner/baseline_amendment_test.go) | TestBaselineBehaviorVersions 专门保护源码目录以外的手册哈希，代码版本变动不能顺带换手册 |
| TestClockGuard | [internal/eval/runner/clock_test.go:15](../../internal/eval/runner/clock_test.go) | TestClockGuard 保留时钟错误而不只返回泛泛的取消，同时结束自己的检查 goroutine |
| TestBaseline | [internal/eval/runner/environment_test.go:21](../../internal/eval/runner/environment_test.go) | TestBaseline 区分配置文件里的期望值与控制接口返回的实际值 |
| TestMemorySize | [internal/eval/runner/environment_test.go:51](../../internal/eval/runner/environment_test.go) | TestMemorySize 拒绝 NaN、无穷大和溢出；这些值转成整数可能错误地通过六 GiB 检查 |
| TestCLIValidation | [internal/eval/runner/environment_test.go:71](../../internal/eval/runner/environment_test.go) | TestCLIValidation 不提供任何数据库配置，仍应先得到具体的命令校验错误 |
| TestPartialLoadActions | [internal/eval/runner/load_test.go:115](../../internal/eval/runner/load_test.go) | TestPartialLoadActions 只改QPS时保留热点，显式零则确实取消热点；动作顺序不能颠倒 |
| TestLoadGuard | [internal/eval/runner/load_test.go:124](../../internal/eval/runner/load_test.go) | TestLoadGuard 不对调度耗时作断言，只等待真实的复核周期观察到设置变化 |
| TestWorkloadThroughout | [internal/eval/runner/load_test.go:148](../../internal/eval/runner/load_test.go) | TestWorkloadThroughout 过渡中的流量由动作控制，但另两项仍要严格匹配 |
| TestScenarioLoadFiles | [internal/eval/runner/load_test.go:25](../../internal/eval/runner/load_test.go) | TestScenarioLoadFiles 直接读仓库里的全部文件，新增流量动作后也自动进入检查范围 |
| TestLoadExact | [internal/eval/runner/load_test.go:84](../../internal/eval/runner/load_test.go) | TestLoadExact 故障期也必须完全相等，而不是允许任意高于正常基线的值 |
| TestPairedPlanBoundary | [internal/eval/runner/paired_test.go:18](../../internal/eval/runner/paired_test.go) | TestPairedPlanBoundary 同一组 E1 在两个模型下必须各有一个身份，不能被去重成一份 |
| TestGraderMock | [internal/eval/runner/paired_test.go:61](../../internal/eval/runner/paired_test.go) | TestGraderMock 即使 --allow-paid 为真，医生的 Mock 总开关仍不允许读取收费配置 |
| TestEvidence | [internal/eval/scoring/evidence_test.go:18](../../internal/eval/scoring/evidence_test.go) | TestEvidence 对相同表格逐项改变可信条件，防止只看一段包含 ALL 的文字就给分 |
| TestComponentAndCoverage | [internal/eval/scoring/evidence_test.go:46](../../internal/eval/scoring/evidence_test.go) | TestComponentAndCoverage 确认名字相近不能得分，未取得的证据仍计入分母 |
| TestPolicy | [internal/platform/clockcheck/policy_test.go:18](../../internal/platform/clockcheck/policy_test.go) | TestPolicy 同时验证是否暂停和是否留档；允许继续不等于忽略发生过的修正 |
| TestMonitorAcrossSegments | [internal/platform/clockcheck/policy_test.go:54](../../internal/platform/clockcheck/policy_test.go) | TestMonitorAcrossSegments 模拟四段独立采样；每段 elapsed 从零开始，但计数不能重置 |
| TestRollingWindow | [internal/platform/clockcheck/policy_test.go:75](../../internal/platform/clockcheck/policy_test.go) | TestRollingWindow 在不同分钟放入四次跳变；日期乱跳也不能改变历史的淘汰时刻 |
| TestCheck | [internal/platform/clockcheck/sample_test.go:19](../../internal/platform/clockcheck/sample_test.go) | TestCheck 在第 53 次放入变化，复现原实测位置，仍要求全部六十次采样 |
| TestSample | [internal/platform/clockcheck/sample_test.go:71](../../internal/platform/clockcheck/sample_test.go) | TestSample 用 Go 虚拟时间走完一分钟，CI 不真的等待，也不访问 Docker |
| TestClientAgent | [internal/platform/httpx/user_agent_test.go:21](../../internal/platform/httpx/user_agent_test.go) | TestClientAgent 疑似秘密不落日志，普通中文再长也保持有界和完整编码 |
| TestAgentLogData | [internal/platform/httpx/user_agent_test.go:34](../../internal/platform/httpx/user_agent_test.go) | TestAgentLogData 伪造 JSON 字段和命令的文字只能留在 user_agent 字符串中 |
| TestDoctorMounts | [tools/archcheck/doctor_test.go:18](../../tools/archcheck/doctor_test.go) | TestDoctorMounts 直接调用 archcheck 使用的检查函数，不另写一份较宽松的测试判断 |
| TestOpsInternal | [tools/archcheck/doctor_test.go:56](../../tools/archcheck/doctor_test.go) | TestOpsInternal 运维端口即使只映射到 localhost 也不符合本项目的“仅内部网络”约定 |
