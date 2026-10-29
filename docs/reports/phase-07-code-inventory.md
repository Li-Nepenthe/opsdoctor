# 阶段 7 代码清单

本文件是[汇报书](phase-07-reliability.md)第三、四节的明细。范围是阶段分支相对 main 的提交内容，不包含用户保留的两处修改。函数导航由 commentcheck -list 生成后补调用关系；包含修改文件中复验的既有函数。

## 文件清单

| 文件 | 最终行数 | 增加/删除 | 作用 |
|---|---:|---:|---|
| [.env.example](../../.env.example) | 153 | +26/−9 | 全部密码都是本机演示假值，不可用于线上；复制为 .env，真实值只存在忽略的文件或环境变量中。 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 299 | +3/−2 | 固定部署、接入或 CI 配置 |
| [AGENTS.md](../../AGENTS.md) | 116 | +1/−1 | AGENTS.md |
| [README.md](../../README.md) | 496 | +32/−14 | OpsDoctor |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 508 | +27/−9 | 固定部署、接入或 CI 配置 |
| [deploy/doctor/targets/README.md](../../deploy/doctor/targets/README.md) | 13 | +2/−0 | 系统接入目录 |
| [deploy/doctor/targets/shop.yaml](../../deploy/doctor/targets/shop.yaml) | 32 | +16/−0 | 固定部署、接入或 CI 配置 |
| [deploy/mysql/init/006_doctor_readonly.sh](../../deploy/mysql/init/006_doctor_readonly.sh) | 42 | +2/−0 | 医生使用独立账号。即使工具代码出错，这个账号也不能写表、建索引或管理数据库。 |
| [deploy/mysql/init/007_doctor_index.sh](../../deploy/mysql/init/007_doctor_index.sh) | 30 | +30/−0 | 医生的写账号只有五张架构表的 INDEX 权限，不授予 ALTER、业务数据读写或整库权限。 |
| [deploy/mysql/init/README.md](../../deploy/mysql/init/README.md) | 27 | +2/−0 | deploy/mysql/init：首次启动脚本 |
| [docs/README.md](../../docs/README.md) | 19 | +1/−0 | docs：项目文档 |
| [docs/decisions.md](../../docs/decisions.md) | 456 | +67/−0 | 设计决定 |
| [docs/learn/README.md](../../docs/learn/README.md) | 32 | +2/−0 | docs/learn：逐阶段学习材料 |
| [docs/learn/phase-07-reliability.md](../../docs/learn/phase-07-reliability.md) | 362 | +362/−0 | 阶段 7 学习材料：审批、断点恢复与安全 |
| [docs/mcp.md](../../docs/mcp.md) | 43 | +43/−0 | 从外部客户端使用医生的只读工具 |
| [docs/notes.md](../../docs/notes.md) | 1386 | +84/−0 | 实测记录 |
| [docs/reports/README.md](../../docs/reports/README.md) | 18 | +1/−0 | docs/reports：阶段汇报书 |
| [docs/reports/phase-07-approval.png](../../docs/reports/phase-07-approval.png) | 二进制 | +-/−- | 真实验收页面截图 |
| [docs/reports/phase-07-code-inventory.md](../../docs/reports/phase-07-code-inventory.md) | 514 | +514/−0 | 阶段 7 代码清单 |
| [docs/reports/phase-07-final-mock.png](../../docs/reports/phase-07-final-mock.png) | 二进制 | +-/−- | 真实验收页面截图 |
| [docs/reports/phase-07-reliability.md](../../docs/reports/phase-07-reliability.md) | 208 | +208/−0 | 阶段 7 汇报书：审批、断点恢复与安全 |
| [docs/代码地图.md](../../docs/代码地图.md) | 407 | +48/−4 | OpsDoctor 代码地图 |
| [docs/数据台账.md](../../docs/数据台账.md) | 352 | +29/−0 | 数据台账 |
| [go.mod](../../go.mod) | 86 | +6/−0 | 固定模块与直接、间接依赖版本 |
| [go.sum](../../go.sum) | 213 | +14/−0 | 依赖内容校验值 |
| [internal/chaos/app/app.go](../../internal/chaos/app/app.go) | 221 | +2/−1 | 本文件：把 chaos 命令行接到场景、执行器和自动验收流程。 |
| [internal/chaos/verify/baseline.go](../../internal/chaos/verify/baseline.go) | 86 | +17/−5 | 本文件：在等待健康指标之前，核对压测器实际使用的实验参数。 |
| [internal/chaos/verify/doc.go](../../internal/chaos/verify/doc.go) | 22 | +2/−0 | Package verify 回答两个问题：问题真的出现了吗，现在真的恢复了吗？ |
| [internal/chaos/verify/repairs.go](../../internal/chaos/verify/repairs.go) | 80 | +80/−0 | 本文件：阻止把医生修过的环境误当成下一场实验的原始基线。 |
| [internal/chaos/verify/repairs_test.go](../../internal/chaos/verify/repairs_test.go) | 51 | +51/−0 | 本文件：验证实验基线能发现医生留下的修改，而不会替人自动撤销。 |
| [internal/doctor/README.md](../../internal/doctor/README.md) | 30 | +4/−0 | doctor：从告警到有证据的诊断 |
| [internal/doctor/agent/approval_test.go](../../internal/doctor/agent/approval_test.go) | 123 | +123/−0 | 本文件：验证 Agent 暂停审批后仍能接回原来的工具调用。 |
| [internal/doctor/agent/budget.go](../../internal/doctor/agent/budget.go) | 36 | +36/−0 | 本文件：把单次上下文、整场调查用量和费用分开检查。 |
| [internal/doctor/agent/budget_test.go](../../internal/doctor/agent/budget_test.go) | 117 | +117/−0 | 本文件：验证新预算不会把重复发送的历史过早算成调查结束。 |
| [internal/doctor/agent/calls.go](../../internal/doctor/agent/calls.go) | 200 | +62/−9 | 本文件：执行一轮工具请求，并把结果按模型给出的 ID 放回对话。 |
| [internal/doctor/agent/doc.go](../../internal/doctor/agent/doc.go) | 24 | +6/−1 | Package agent 把“问模型、查证据、再问模型”组织成一个有边界的循环。 |
| [internal/doctor/agent/loop.go](../../internal/doctor/agent/loop.go) | 247 | +88/−22 | 本文件：控制诊断从第一次询问模型到提交报告的整个过程。 |
| [internal/doctor/agent/loop_test.go](../../internal/doctor/agent/loop_test.go) | 311 | +15/−3 | 本文件：用脚本模型和内存记录器验证完整诊断循环。 |
| [internal/doctor/agent/model.go](../../internal/doctor/agent/model.go) | 101 | +52/−30 | 本文件：定义诊断循环需要的输入、步骤记录和依赖接口。 |
| [internal/doctor/agent/prompt.go](../../internal/doctor/agent/prompt.go) | 51 | +4/−1 | 本文件：固定系统提示与架构说明的前缀，供每一轮模型调用复用。 |
| [internal/doctor/agent/prompts/system.md](../../internal/doctor/agent/prompts/system.md) | 19 | +7/−5 | OpsDoctor 诊断规则 v7.2 |
| [internal/doctor/agent/replay.go](../../internal/doctor/agent/replay.go) | 112 | +112/−0 | 本文件：从已保存的模型请求和工具结果，重建中断前的对话。 |
| [internal/doctor/agent/replay_test.go](../../internal/doctor/agent/replay_test.go) | 122 | +122/−0 | 本文件：用旧步骤模拟中断，验证恢复、换模型和工具数据边界。 |
| [internal/doctor/alerts/doc.go](../../internal/doctor/alerts/doc.go) | 12 | +1/−0 | Package alerts 把重复收到的告警交给仓储，创建等待处理的诊断任务。 |
| [internal/doctor/alerts/service.go](../../internal/doctor/alerts/service.go) | 139 | +15/−8 | 本文件：检查 Alertmanager 通知，并把合法的分组交给仓储保存。 |
| [internal/doctor/alerts/service_test.go](../../internal/doctor/alerts/service_test.go) | 45 | +45/−0 | 本文件：验证列表标题在系统级告警分组下仍能说明发生了什么。 |
| [internal/doctor/app/agent_config.go](../../internal/doctor/app/agent_config.go) | 158 | +51/−14 | 本文件：读取模型、调度和页面安全配置，默认只运行不花钱的 Mock。 |
| [internal/doctor/app/agent_config_test.go](../../internal/doctor/app/agent_config_test.go) | 64 | +1/−1 | 本文件：验证新环境不会因为填了 Key 就自动花钱，以及所有预算配置的边界。 |
| [internal/doctor/app/agent_wiring.go](../../internal/doctor/app/agent_wiring.go) | 90 | +12/−6 | 本文件：把仓储、模型、Agent、调度和实时通知接成一条执行路径。 |
| [internal/doctor/app/app.go](../../internal/doctor/app/app.go) | 118 | +13/−1 | 本文件：按顺序准备医生自己的数据库、接入系统和 HTTP 服务。 |
| [internal/doctor/app/doc.go](../../internal/doctor/app/doc.go) | 20 | +1/−1 | Package app 把医生各部分按启动顺序连接起来。 |
| [internal/doctor/app/metrics.go](../../internal/doctor/app/metrics.go) | 141 | +17/−9 | 本文件：记录医生做了多少工作，以及为什么不能继续启动真实诊断。 |
| [internal/doctor/approval/backend.go](../../internal/doctor/approval/backend.go) | 158 | +158/−0 | 本文件：为一个系统绑定索引账号和有限的运维地址。 |
| [internal/doctor/approval/definitions.go](../../internal/doctor/approval/definitions.go) | 54 | +54/−0 | 本文件：描述模型可以申请的三种写操作。 |
| [internal/doctor/approval/doc.go](../../internal/doctor/approval/doc.go) | 20 | +20/−0 | Package approval 把模型的建议变成人能检查、批准或拒绝的具体操作。 |
| [internal/doctor/approval/execute.go](../../internal/doctor/approval/execute.go) | 131 | +131/−0 | 本文件：执行已经领取的审批，并在恢复时先核对目标系统的真实状态。 |
| [internal/doctor/approval/model.go](../../internal/doctor/approval/model.go) | 85 | +85/−0 | 本文件：定义一张审批单保存什么，以及执行器需要哪些能力。 |
| [internal/doctor/approval/permissions_integration_test.go](../../internal/doctor/approval/permissions_integration_test.go) | 114 | +114/−0 | 本文件：用真实索引账号验证最后一道数据库权限边界。 |
| [internal/doctor/approval/prepare.go](../../internal/doctor/approval/prepare.go) | 183 | +183/−0 | 本文件：把三个固定写工具的参数转换成可审阅、可撤销的计划。 |
| [internal/doctor/approval/prepare_test.go](../../internal/doctor/approval/prepare_test.go) | 108 | +108/−0 | 本文件：用虚构架构和元数据检查写工具的参数边界。 |
| [internal/doctor/approval/service.go](../../internal/doctor/approval/service.go) | 116 | +116/−0 | 本文件：把一次模型写工具调用接到持久化审批流程。 |
| [internal/doctor/approval/service_test.go](../../internal/doctor/approval/service_test.go) | 143 | +143/−0 | 本文件：验证审批保存以后，模型不能换掉执行参数。 |
| [internal/doctor/httpapi/approvals.go](../../internal/doctor/httpapi/approvals.go) | 78 | +78/−0 | 本文件：给已登录的人提供审批单查看、批准和拒绝入口。 |
| [internal/doctor/httpapi/approvals_test.go](../../internal/doctor/httpapi/approvals_test.go) | 79 | +79/−0 | 本文件：证明批准接口只能由人使用，并且无法夹带新的执行参数。 |
| [internal/doctor/httpapi/doc.go](../../internal/doctor/httpapi/doc.go) | 22 | +3/−1 | Package httpapi 把告警、人工取证和实时页面接到同一个医生。 |
| [internal/doctor/httpapi/handlers.go](../../internal/doctor/httpapi/handlers.go) | 201 | +1/−1 | 本文件：实现登录、告警接收、就绪检查和任务查询的 HTTP 细节。 |
| [internal/doctor/httpapi/router.go](../../internal/doctor/httpapi/router.go) | 189 | +16/−1 | 本文件：把登录、系统工具和告警入口接到同一个 Gin 服务。 |
| [internal/doctor/llm/boundary_test.go](../../internal/doctor/llm/boundary_test.go) | 80 | +20/−0 | 本文件：验证发往模型的内容边界与整数金额计算。 |
| [internal/doctor/llm/cost.go](../../internal/doctor/llm/cost.go) | 110 | +36/−13 | 本文件：用整数表示费用，并规定发送前预留、发送后结算的账本接口。 |
| [internal/doctor/llm/doc.go](../../internal/doctor/llm/doc.go) | 19 | +1/−0 | Package llm 约定医生怎样和模型交换消息，以及怎样记录这次交换的费用。 |
| [internal/doctor/llm/lease.go](../../internal/doctor/llm/lease.go) | 25 | +25/−0 | 本文件：把执行权编号沿着 context 传到保存步骤和预留费用的位置。 |
| [internal/doctor/llm/model.go](../../internal/doctor/llm/model.go) | 92 | +19/−13 | 本文件：描述一轮模型对话的请求、回复与工具调用关联。 |
| [internal/doctor/llm/openai/client.go](../../internal/doctor/llm/openai/client.go) | 197 | +33/−10 | 本文件：把一轮模型请求发送出去，并在发送前后留下费用和内容审计。 |
| [internal/doctor/llm/openai/client_test.go](../../internal/doctor/llm/openai/client_test.go) | 198 | +2/−2 | 本文件：用内存 HTTP 传输检查模型协议、重试、费用预留与取消。 |
| [internal/doctor/llm/openai/doc.go](../../internal/doctor/llm/openai/doc.go) | 17 | +2/−0 | Package openai 用普通 HTTP 实现兼容 OpenAI 的非流式工具调用协议。 |
| [internal/doctor/llm/openai/protocol.go](../../internal/doctor/llm/openai/protocol.go) | 86 | +4/−19 | 本文件：把模型 HTTP 回复转换为对话消息，限定可重试的错误。 |
| [internal/doctor/llm/openai/responses.go](../../internal/doctor/llm/openai/responses.go) | 165 | +165/−0 | 本文件：在项目内部的对话格式与 OpenAI Responses 协议之间转换。 |
| [internal/doctor/llm/openai/responses_test.go](../../internal/doctor/llm/openai/responses_test.go) | 95 | +95/−0 | 本文件：用内存 HTTP 回复验证 Responses 的函数往返和缓存计费。 |
| [internal/doctor/mcpserver/audit.go](../../internal/doctor/mcpserver/audit.go) | 84 | +84/−0 | 本文件：让来自外部客户端的工具调用也留下可追查记录。 |
| [internal/doctor/mcpserver/doc.go](../../internal/doctor/mcpserver/doc.go) | 14 | +14/−0 | Package mcpserver 让外部编辑器也能使用医生的只读工具。 |
| [internal/doctor/mcpserver/server.go](../../internal/doctor/mcpserver/server.go) | 75 | +75/−0 | 本文件：把已经绑定系统的只读注册表接到 MCP 的 HTTP 入口。 |
| [internal/doctor/mcpserver/server_test.go](../../internal/doctor/mcpserver/server_test.go) | 140 | +140/−0 | 本文件：用官方 MCP 客户端验证身份、工具边界和审计确实贯通。 |
| [internal/doctor/run/doc.go](../../internal/doctor/run/doc.go) | 14 | +1/−1 | Package run 决定哪个诊断现在可以执行，并管理取消和结束状态。 |
| [internal/doctor/run/scheduler.go](../../internal/doctor/run/scheduler.go) | 389 | +102/−29 | 本文件：为排队诊断分配名额，运行结束后回收名额，并处理取消。 |
| [internal/doctor/run/scheduler_test.go](../../internal/doctor/run/scheduler_test.go) | 339 | +63/−11 | 本文件：用可控时间和阻塞模型验证调度名额、过时任务、费用拦截与取消。 |
| [internal/doctor/run/state.go](../../internal/doctor/run/state.go) | 33 | +9/−8 | 本文件：列出本阶段允许的诊断状态变化。 |
| [internal/doctor/source/mysqlinspect/doc.go](../../internal/doctor/source/mysqlinspect/doc.go) | 18 | +1/−0 | Package mysqlinspect 让医生查看 MySQL 为什么变慢，而不是查看业务数据内容。 |
| [internal/doctor/source/mysqlinspect/write_metadata.go](../../internal/doctor/source/mysqlinspect/write_metadata.go) | 58 | +58/−0 | 本文件：为审批准备和恢复提供有限的只读元数据检查。 |
| [internal/doctor/store/agent_integration_test.go](../../internal/doctor/store/agent_integration_test.go) | 267 | +60/−3 | 本文件：在专用数据库验证故障开放期、任务清理、步骤编号和累计费用。 |
| [internal/doctor/store/approval_integration_test.go](../../internal/doctor/store/approval_integration_test.go) | 171 | +171/−0 | 本文件：在专用医生测试库验证审批的事务边界和等待计时。 |
| [internal/doctor/store/approvals.go](../../internal/doctor/store/approvals.go) | 321 | +321/−0 | 本文件：保存人工审批，并用条件更新保证同一张单只能批准或拒绝一次。 |
| [internal/doctor/store/budget.go](../../internal/doctor/store/budget.go) | 191 | +51/−9 | 本文件：把真实模型的费用预留与结算保存到医生数据库。 |
| [internal/doctor/store/diagnoses.go](../../internal/doctor/store/diagnoses.go) | 119 | +8/−5 | 本文件：读取已经创建的诊断任务，供验收和后续页面使用。 |
| [internal/doctor/store/doc.go](../../internal/doctor/store/doc.go) | 29 | +7/−0 | Package store 保存医生自己的用户、告警、诊断步骤和费用账本。 |
| [internal/doctor/store/lease_integration_test.go](../../internal/doctor/store/lease_integration_test.go) | 169 | +169/−0 | 本文件：在专用数据库模拟两个执行者争抢租约，以及模型回复保存一半时崩溃。 |
| [internal/doctor/store/leases.go](../../internal/doctor/store/leases.go) | 216 | +216/−0 | 本文件：在数据库里分配和续租诊断的执行权。 |
| [internal/doctor/store/mcp.go](../../internal/doctor/store/mcp.go) | 38 | +38/−0 | 本文件：保存 MCP 客户端调用工具的审计，不把外部调用伪装成医生诊断。 |
| [internal/doctor/store/mcp_integration_test.go](../../internal/doctor/store/mcp_integration_test.go) | 46 | +46/−0 | 本文件：在医生专用测试库核对 MCP 审计不是只写在日志里。 |
| [internal/doctor/store/migrations/0013_pricing_and_report.sql](../../internal/doctor/store/migrations/0013_pricing_and_report.sql) | 7 | +7/−0 | 费用按请求当时的三档单价复算。旧记录保持 NULL，不能把阶段 6 的保守旧账伪装成实价。 |
| [internal/doctor/store/migrations/0014_report_reason.sql](../../internal/doctor/store/migrations/0014_report_reason.sql) | 4 | +4/−0 | 直接保存报告触发方式，验收时不用从措辞猜测是不是被预算迫使结束。 |
| [internal/doctor/store/migrations/0015_mcp_audit.sql](../../internal/doctor/store/migrations/0015_mcp_audit.sql) | 12 | +12/−0 | 外部 MCP 调用不依赖某个诊断；开始先落库，崩溃也保留 running 记录供人工核对。 |
| [internal/doctor/store/migrations/0016_lease_clock.sql](../../internal/doctor/store/migrations/0016_lease_clock.sql) | 5 | +5/−0 | 运行时间只累加真正持有租约的区间。等待审批不占调查时间。 |
| [internal/doctor/store/migrations/0017_step_identity.sql](../../internal/doctor/store/migrations/0017_step_identity.sql) | 7 | +7/−0 | 同一逻辑结果只保存一次；NULL 允许历史步骤和普通状态事件没有幂等编号。 |
| [internal/doctor/store/migrations/0018_attempt_call.sql](../../internal/doctor/store/migrations/0018_attempt_call.sql) | 3 | +3/−0 | 多次 HTTP 重试归属于同一个发送意图，恢复时可找回结算完成但尚未写步骤的回复。 |
| [internal/doctor/store/migrations/0019_scheduler_lock.sql](../../internal/doctor/store/migrations/0019_scheduler_lock.sql) | 2 | +2/−0 | 一行协调锁只用于分配租约的短事务，不会在模型请求期间持有。 |
| [internal/doctor/store/migrations/0020_scheduler_guard.sql](../../internal/doctor/store/migrations/0020_scheduler_guard.sql) | 2 | +2/−0 | 多进程启动也只保留同一个名额分配入口。 |
| [internal/doctor/store/migrations/0021_budget_currency.sql](../../internal/doctor/store/migrations/0021_budget_currency.sql) | 2 | +2/−0 | 旧账户一直按人民币记账，新增字段不能重解释历史金额。 |
| [internal/doctor/store/migrations/0022_diagnosis_currency.sql](../../internal/doctor/store/migrations/0022_diagnosis_currency.sql) | 2 | +2/−0 | 一次诊断只使用一种币种，换供应商恢复时不能直接相加。 |
| [internal/doctor/store/migrations/0023_step_currency.sql](../../internal/doctor/store/migrations/0023_step_currency.sql) | 2 | +2/−0 | 步骤自己带币种，页面显示旧诊断时不能使用当前账户的币种。 |
| [internal/doctor/store/migrations/0024_attempt_cache_write.sql](../../internal/doctor/store/migrations/0024_attempt_cache_write.sql) | 2 | +2/−0 | 同时保存缓存写入量和发送时的写入价，费用能按旧价重算。 |
| [internal/doctor/store/migrations/0025_approvals.sql](../../internal/doctor/store/migrations/0025_approvals.sql) | 26 | +26/−0 | 审批保存的是具体计划；executing 和连接编号用于崩溃后核对真实状态。 |
| [internal/doctor/store/migrations/README.md](../../internal/doctor/store/migrations/README.md) | 37 | +13/−0 | 医生自己的表 |
| [internal/doctor/store/model_recovery.go](../../internal/doctor/store/model_recovery.go) | 105 | +105/−0 | 本文件：补上模型结算和步骤保存之间的崩溃空窗。 |
| [internal/doctor/store/recovery_integration_test.go](../../internal/doctor/store/recovery_integration_test.go) | 139 | +2/−1 | 本文件：用真实事务验证恢复观察期既防止重复诊断，也不会永远吞掉下一场故障。 |
| [internal/doctor/store/steps.go](../../internal/doctor/store/steps.go) | 125 | +39/−6 | 本文件：给步骤分配编号，并为页面断线后的补发提供有序读取。 |
| [internal/doctor/store/store.go](../../internal/doctor/store/store.go) | 91 | +14/−0 | 本文件：建立医生仓储并应用医生自己的迁移。 |
| [internal/doctor/store/tasks.go](../../internal/doctor/store/tasks.go) | 195 | +38/−6 | 本文件：保存调度状态变化，隔离启动清理、排队和结束操作。 |
| [internal/doctor/target/actions.go](../../internal/doctor/target/actions.go) | 68 | +68/−0 | 本文件：把系统说明书、索引账号和运维入口绑定到审批后端。 |
| [internal/doctor/target/catalog.go](../../internal/doctor/target/catalog.go) | 281 | +30/−11 | 本文件：为每个系统分别创建数据源和工具注册表。 |
| [internal/doctor/target/catalog_test.go](../../internal/doctor/target/catalog_test.go) | 211 | +3/−0 | 本文件：证明两个系统的工具和告警令牌不会串用。 |
| [internal/doctor/target/config.go](../../internal/doctor/target/config.go) | 140 | +6/−2 | 本文件：读取一个目录里的全部系统配置，并检查系统之间有没有共用凭据。 |
| [internal/doctor/target/doc.go](../../internal/doctor/target/doc.go) | 16 | +1/−0 | Package target 把“要诊断哪套系统”变成启动时就确定的连接与工具。 |
| [internal/doctor/web/doc.go](../../internal/doctor/web/doc.go) | 11 | +1/−1 | Package web 把页面文件一起编译进医生，不需要额外启动前端服务器。 |
| [internal/doctor/web/static/app.js](../../internal/doctor/web/static/app.js) | 267 | +45/−11 | 诊断页面、审批按钮与安全文本展示 |
| [internal/doctor/web/static/index.html](../../internal/doctor/web/static/index.html) | 29 | +1/−1 | 原生诊断工作台页面结构 |
| [internal/shop/README.md](../../internal/shop/README.md) | 29 | +2/−0 | internal/shop：靶场（电商业务） |
| [internal/shop/app/doc.go](../../internal/shop/app/doc.go) | 20 | +2/−1 | Package app 存放靶场几个程序共用的数据库配置和内部运维入口。 |
| [internal/shop/app/inventoryservice/app.go](../../internal/shop/app/inventoryservice/app.go) | 125 | +6/−0 | 本文件：库存进程的资源组装与退出流程。 |
| [internal/shop/app/ops.go](../../internal/shop/app/ops.go) | 80 | +80/−0 | 本文件：为三个业务进程装配独立的运维监听器。 |
| [internal/shop/app/orderservice/app.go](../../internal/shop/app/orderservice/app.go) | 187 | +10/−0 | 本文件：装配订单服务的依赖、路由和退出流程。 |
| [internal/shop/app/orderworker/app.go](../../internal/shop/app/orderworker/app.go) | 140 | +5/−0 | 本文件：启动订单消息消费者，并管理它的停机顺序。 |
| [internal/shop/ops/doc.go](../../internal/shop/ops/doc.go) | 13 | +13/−0 | Package ops 给已经获批的操作提供一个很窄的业务侧入口。 |
| [internal/shop/ops/handler.go](../../internal/shop/ops/handler.go) | 232 | +232/−0 | 本文件：只接收调整连接池和预热缓存这两类运维请求。 |
| [internal/shop/ops/handler_test.go](../../internal/shop/ops/handler_test.go) | 137 | +137/−0 | 本文件：验证业务端运维接口自己的限制，不依赖医生端先过滤参数。 |
| [tools/README.md](../../tools/README.md) | 23 | +4/−2 | tools：代码检查与恢复验收 |
| [tools/archcheck/doctor.go](../../tools/archcheck/doctor.go) | 90 | +33/−1 | 本文件：检查医生容器能从宿主机拿到什么。 |
| [tools/archcheck/doctor_test.go](../../tools/archcheck/doctor_test.go) | 72 | +24/−0 | 本文件：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。 |
| [tools/recoverycheck/crash_integration_test.go](../../tools/recoverycheck/crash_integration_test.go) | 405 | +405/−0 | 本文件：真的终止一个子进程，再用同一个测试库恢复诊断。 |
| [tools/recoverycheck/doc.go](../../tools/recoverycheck/doc.go) | 13 | +13/−0 | Package recoverycheck 验证医生进程真的消失以后，数据库中的断点是否足够恢复。 |
| [tools/recoverycheck/fixture_test.go](../../tools/recoverycheck/fixture_test.go) | 68 | +68/−0 | 本文件：给进程崩溃验收准备独立的医生测试库记录。 |
| [进度.md](../../进度.md) | 163 | +8/−8 | OpsDoctor 进度看板 |
| [项目计划书.md](../../项目计划书.md) | 2128 | +34/−12 | OpsDoctor 项目计划书：Go 故障诊断 Agent + 可复现故障靶场 |

## 类型与函数导航

本表包含修改文件中既有入口，不能把每一行都理解为新函数。测试函数另列在下一节。

| 包 | 名称 | 位置 | 作用 | 被谁调用 |
|---|---|---|---|---|
| internal/chaos/app (app) | `*Engine.general` | [internal/chaos/app/app.go:102](../../internal/chaos/app/app.go) | general 处理无需场景编号的命令 | chaos healthy 的基线核对 |
| internal/chaos/app (app) | `*Engine.operate` | [internal/chaos/app/app.go:126](../../internal/chaos/app/app.go) | operate 用已有记录匹配 verify 和 restore，防止把另一场景的效果误当成当前场景 | chaos healthy 的基线核对 |
| internal/chaos/app (app) | `overrides` | [internal/chaos/app/app.go:172](../../internal/chaos/app/app.go) | overrides 只覆盖对应动作已有的参数；没有这种动作的场景会拒绝无效的覆盖选项 | chaos healthy 的基线核对 |
| internal/chaos/app (app) | `Run` | [internal/chaos/app/app.go:33](../../internal/chaos/app/app.go) | Run 不隐式启动流量 | chaos healthy 的基线核对 |
| internal/chaos/app (app) | `newEngine` | [internal/chaos/app/app.go:77](../../internal/chaos/app/app.go) | newEngine 只读取密码，不输出它 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `Baseline` | [internal/chaos/verify/baseline.go:24](../../internal/chaos/verify/baseline.go) | Baseline 在原有指标观察前加参数核对 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `WithBaseline` | [internal/chaos/verify/baseline.go:33](../../internal/chaos/verify/baseline.go) | WithBaseline 由 app 传入校验过的配置，单元测试仍可单独验证原来的指标观察器 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `*Baseline.Healthy` | [internal/chaos/verify/baseline.go:38](../../internal/chaos/verify/baseline.go) | Healthy 遇到配置不匹配直接返回，不等三分钟，也不开始任何注入动作 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `*Baseline.check` | [internal/chaos/verify/baseline.go:57](../../internal/chaos/verify/baseline.go) | check 只读取小型配置响应；老版本缺字段时得到零值，也必须报错，不能默认放行 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `*Baseline.WithRepairs` | [internal/chaos/verify/repairs.go:21](../../internal/chaos/verify/repairs.go) | WithRepairs 与旧参数检查串联；生产 healthy 和 selftest 都必须装配这条检查 | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `repairIndexes` | [internal/chaos/verify/repairs.go:27](../../internal/chaos/verify/repairs.go) | repairIndexes 先确认能看见五张表，权限缺失时不能把“不知道”说成“没有残留索引” | chaos healthy 的基线核对 |
| internal/chaos/verify (verify) | `*Baseline.checkRepairs` | [internal/chaos/verify/repairs.go:60](../../internal/chaos/verify/repairs.go) | checkRepairs 既要指标值为零，也要三个进程都报告了值，不能把缺采集误当未修改 | chaos healthy 的基线核对 |
| internal/doctor/agent (agent) | `*session.inputBound` | [internal/doctor/agent/budget.go:13](../../internal/doctor/agent/budget.go) | inputBound 利用对话只追加的约束，避免每轮都按整段 UTF-8 字节高估历史 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.budgetReason` | [internal/doctor/agent/budget.go:23](../../internal/doctor/agent/budget.go) | budgetReason 的先后顺序只影响展示原因；任何一个上限耗尽，都不能继续查新证据 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.saveTool` | [internal/doctor/agent/calls.go:132](../../internal/doctor/agent/calls.go) | saveTool 把实际步骤号包在 tool 回复里，模型无需猜测并行完成的顺序 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `toolKey` | [internal/doctor/agent/calls.go:156](../../internal/doctor/agent/calls.go) | toolKey 加入模型步骤号；供应商在不同轮里复用同一个 ID 时仍然是两次不同调用 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `toolMessage` | [internal/doctor/agent/calls.go:160](../../internal/doctor/agent/calls.go) | toolMessage 把工具正文编码成 JSON 字符串 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `invalidReport` | [internal/doctor/agent/calls.go:169](../../internal/doctor/agent/calls.go) | invalidReport 与普通工具错误一样交回模型，只说明校验失败，不结束诊断 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.writeTool` | [internal/doctor/agent/calls.go:174](../../internal/doctor/agent/calls.go) | writeTool 等待时不伪造工具完成结果 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.calls` | [internal/doctor/agent/calls.go:23](../../internal/doctor/agent/calls.go) | calls 要求报告独占一轮；混合时执行查看工具，明确拒绝报告，让模型看完新结果再提交 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `validateCalls` | [internal/doctor/agent/calls.go:63](../../internal/doctor/agent/calls.go) | validateCalls 只检查协议关联，工具名是否存在和参数是否正确仍交给统一注册表 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.readTools` | [internal/doctor/agent/calls.go:78](../../internal/doctor/agent/calls.go) | readTools 仅审计失败才中止整轮；参数错误、超时和数据源错误都作为普通工具结果 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.ask` | [internal/doctor/agent/loop.go:113](../../internal/doctor/agent/loop.go) | ask 把实际脱敏后的请求写进步骤；真实 HTTP 尝试另由 llm 账本在发送前保存 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.finish` | [internal/doctor/agent/loop.go:191](../../internal/doctor/agent/loop.go) | finish 先要求整理；如果确实提交了报告但校验失败，再给一次修正机会 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `record` | [internal/doctor/agent/loop.go:229](../../internal/doctor/agent/loop.go) | record 取消之后只留半秒保存最后状态；真实查询仍使用已取消的原 context | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `markMock` | [internal/doctor/agent/loop.go:237](../../internal/doctor/agent/loop.go) | markMock 在程序端加醒目标记，不能靠脚本模型自己记住声明 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Agent` | [internal/doctor/agent/loop.go:23](../../internal/doctor/agent/loop.go) | Agent 构造后只读；每次 Run 的消息和计数都放在独立 session 中 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*Agent.isMock` | [internal/doctor/agent/loop.go:247](../../internal/doctor/agent/loop.go) | isMock 只检查程序选定的模型名，供应商回复不能把真实请求变成免费请求 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `session` | [internal/doctor/agent/loop.go:31](../../internal/doctor/agent/loop.go) | session 保存一次诊断的状态，不把历史挂在共享模型客户端上 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `New` | [internal/doctor/agent/loop.go:45](../../internal/doctor/agent/loop.go) | New 不联网，参数错误在启动时暴露，单元测试可使用较小预算 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*Agent.Run` | [internal/doctor/agent/loop.go:60](../../internal/doctor/agent/loop.go) | Run 为普通查证和最后整理分别设截止时间，但二者加起来仍只有一份总时限 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.execute` | [internal/doctor/agent/loop.go:82](../../internal/doctor/agent/loop.go) | execute 在每次回答之后检查累计用量，最后报告机会仍受 whole 的总时限约束 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Step` | [internal/doctor/agent/model.go:21](../../internal/doctor/agent/model.go) | Step 按诊断内部的 Seq 排序；并行工具按保存完成的次序得到编号 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Recorder` | [internal/doctor/agent/model.go:42](../../internal/doctor/agent/model.go) | Recorder 必须支持并行工具保存，成功返回的步骤才可被报告引用 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Toolbox` | [internal/doctor/agent/model.go:47](../../internal/doctor/agent/model.go) | Toolbox 只提供当前系统的查看工具；模型无法改变它绑定的数据源 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `ApprovalFlow` | [internal/doctor/agent/model.go:53](../../internal/doctor/agent/model.go) | ApprovalFlow 只有申请和读取结果的能力，没有批准按钮 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Input` | [internal/doctor/agent/model.go:60](../../internal/doctor/agent/model.go) | Input 在开始诊断时固定，后续告警仍入库，但不会改变已经发送的固定消息前缀 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Limits` | [internal/doctor/agent/model.go:75](../../internal/doctor/agent/model.go) | Limits 的默认值来自计划书，测试可调小以快速检查预算边界 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `DefaultLimits` | [internal/doctor/agent/model.go:86](../../internal/doctor/agent/model.go) | DefaultLimits 给正常查证预留四分钟，最后一分钟只整理已有证据，不再查新数据 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `Result` | [internal/doctor/agent/model.go:91](../../internal/doctor/agent/model.go) | Result 失败也带用量，费用不能因最终没有报告而消失 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `messages` | [internal/doctor/agent/prompt.go:28](../../internal/doctor/agent/prompt.go) | messages 的前两条不会随轮次变化，动态证据始终追加在末尾 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `definitions` | [internal/doctor/agent/prompt.go:41](../../internal/doctor/agent/prompt.go) | definitions 从同一个注册表生成工具菜单，Agent 不能偷偷增加另一个执行入口 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `finalRequest` | [internal/doctor/agent/replay.go:110](../../internal/doctor/agent/replay.go) | finalRequest 由程序保存的工具菜单识别整理阶段，不信任模型自称的预算状态 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.restore` | [internal/doctor/agent/replay.go:21](../../internal/doctor/agent/replay.go) | restore 的计数来自全部历史，进程重启不能重新获得十五轮或一份新费用预算 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/agent (agent) | `*session.repairFinal` | [internal/doctor/agent/replay.go:95](../../internal/doctor/agent/replay.go) | repairFinal 只补一次最终报告，仍沿用原任务剩下的 token、费用与活动时间 | Scheduler.execute → Agent.Run；回放与模型工具分派 |
| internal/doctor/alerts (alerts) | `validLabels` | [internal/doctor/alerts/service.go:107](../../internal/doctor/alerts/service.go) | validLabels 限制单字段和数量，整体正文另由 HTTP 的一 MiB 上限限制 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Title` | [internal/doctor/alerts/service.go:120](../../internal/doctor/alerts/service.go) | Title 只用于列表展示，数据库身份始终是系统名与分组键，不用可变的标题去重 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Alert` | [internal/doctor/alerts/service.go:24](../../internal/doctor/alerts/service.go) | Alert 保存证据快照；Labels、Annotations 是外部文字，以后也不能把它们当指令 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Notification` | [internal/doctor/alerts/service.go:34](../../internal/doctor/alerts/service.go) | Notification 的 GroupKey 是推送方的分组键，仓储还会加上系统名形成去重身份 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Receipt` | [internal/doctor/alerts/service.go:42](../../internal/doctor/alerts/service.go) | Receipt 说明通知已保存或被忽略，不表示诊断已经执行 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Receiver` | [internal/doctor/alerts/service.go:50](../../internal/doctor/alerts/service.go) | Receiver 定义在通知处理这一使用方，MySQL 仓储只需实现它，不需要显式 implements | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Service` | [internal/doctor/alerts/service.go:55](../../internal/doctor/alerts/service.go) | Service 不保留某个“默认系统”，每次身份都由经过认证的接口传入 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `New` | [internal/doctor/alerts/service.go:58](../../internal/doctor/alerts/service.go) | New 让测试可以替换仓储，同时生产路径仍只有同一个通知校验入口 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `*Service.Receive` | [internal/doctor/alerts/service.go:61](../../internal/doctor/alerts/service.go) | Receive 校验错误与存储错误分开，HTTP 层才能分别返回 400 和可重试的 503 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Invalid` | [internal/doctor/alerts/service.go:72](../../internal/doctor/alerts/service.go) | Invalid 表示通知格式错误；错误文字由代码生成，不包含请求正文 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `Invalid.Error` | [internal/doctor/alerts/service.go:75](../../internal/doctor/alerts/service.go) | Error 让 Invalid 可作为标准 error 返回，接口层通过 errors.As 判断分类 | 告警接收与诊断创建 |
| internal/doctor/alerts (alerts) | `validate` | [internal/doctor/alerts/service.go:78](../../internal/doctor/alerts/service.go) | validate 不信任推送内容的条数和字段长度，认证通过也不意味着可以无限占用数据库 | 告警接收与诊断创建 |
| internal/doctor/app (app) | `knownSecrets` | [internal/doctor/app/agent_config.go:129](../../internal/doctor/app/agent_config.go) | knownSecrets 只收集本项目明确使用的凭据，在内存中替换，不输出也不另存清单 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `agentSettings` | [internal/doctor/app/agent_config.go:26](../../internal/doctor/app/agent_config.go) | agentSettings 含 API Key，不能作为整个对象写进日志或错误 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `loadAgentConfig` | [internal/doctor/app/agent_config.go:50](../../internal/doctor/app/agent_config.go) | loadAgentConfig 的空值是配置错误，不把误拼的真实模型模式静默退回 Mock | cmd/doctor → app.Run |
| internal/doctor/app (app) | `assembleAgent` | [internal/doctor/app/agent_wiring.go:32](../../internal/doctor/app/agent_wiring.go) | assembleAgent 只装配，不启动 goroutine；全部成功后 Run 才启动调度和监听端口 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `createModel` | [internal/doctor/app/agent_wiring.go:85](../../internal/doctor/app/agent_wiring.go) | createModel 不做探测调用；真实配置不完整时返回错误，不回退成误导用户的脚本报告 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `serve` | [internal/doctor/app/app.go:102](../../internal/doctor/app/app.go) | serve 停止接新请求后等待在途请求；工具最多十秒，停机预算再多留两秒做响应收尾 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `Run` | [internal/doctor/app/app.go:38](../../internal/doctor/app/app.go) | Run 只有装配完全成功才监听端口，启动失败也会关闭已经创建的资源 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `*doctorMetrics.changed` | [internal/doctor/app/metrics.go:103](../../internal/doctor/app/metrics.go) | changed 统计状态进入次数，只有开始过执行的终态才计入耗时分布 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `*doctorMetrics.budget` | [internal/doctor/app/metrics.go:111](../../internal/doctor/app/metrics.go) | budget 与页面共享调度器读数，额度不足不会只在日志中留下难发现的一句话 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `moneyCurrency` | [internal/doctor/app/metrics.go:136](../../internal/doctor/app/metrics.go) | moneyCurrency 限定两个标签；测试旧步骤没带字段时仍按旧人民币口径处理 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `doctorMetrics` | [internal/doctor/app/metrics.go:22](../../internal/doctor/app/metrics.go) | doctorMetrics 的计数器由 Prometheus 库处理并发，mu 只保护有限模型标签集合 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `newDoctorMetrics` | [internal/doctor/app/metrics.go:40](../../internal/doctor/app/metrics.go) | newDoctorMetrics 延用平台注册表，诊断耗时另用覆盖五分钟的桶 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `*doctorMetrics.model` | [internal/doctor/app/metrics.go:63](../../internal/doctor/app/metrics.go) | model 将供应商的任意文本限制为八个已见值加 other，实际原文仍完整保存在步骤中 | cmd/doctor → app.Run |
| internal/doctor/app (app) | `*doctorMetrics.step` | [internal/doctor/app/metrics.go:77](../../internal/doctor/app/metrics.go) | step 在保存后调用，未知工具和结果都归类，不把模型随手编的名称变成新指标 | cmd/doctor → app.Run |
| internal/doctor/approval (approval) | `*Backend.Close` | [internal/doctor/approval/backend.go:104](../../internal/doctor/approval/backend.go) | Close 由接入目录统一调用，单次写工具不能拆掉共享后端 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.opsRequest` | [internal/doctor/approval/backend.go:113](../../internal/doctor/approval/backend.go) | opsRequest 的路径由代码固定，返回体只接受小型运维结构，不读取商品内容 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `databaseProblem` | [internal/doctor/approval/backend.go:152](../../internal/doctor/approval/backend.go) | databaseProblem 只保留错误号；DDL 的原始错误可能夹带库名、账号或连接信息 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `OpsConfig` | [internal/doctor/approval/backend.go:28](../../internal/doctor/approval/backend.go) | OpsConfig 来自接入配置，不接受模型的同名字段覆盖 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Backend` | [internal/doctor/approval/backend.go:37](../../internal/doctor/approval/backend.go) | Backend 只保存本系统的资源，不包含目录或其他系统的连接 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `NewBackend` | [internal/doctor/approval/backend.go:47](../../internal/doctor/approval/backend.go) | NewBackend 不立即连接数据库；启动配置检查失败时关闭已经创建的池 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.definitions` | [internal/doctor/approval/definitions.go:19](../../internal/doctor/approval/definitions.go) | definitions 只在有对应资源时列出工具；MCP 使用另一份只读菜单，拿不到这些申请入口 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `writeDefinition` | [internal/doctor/approval/definitions.go:45](../../internal/doctor/approval/definitions.go) | writeDefinition 把证据约束说明在菜单里，但真正的步骤存在性检查仍在 prepare 中 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.setPool` | [internal/doctor/approval/execute.go:113](../../internal/doctor/approval/execute.go) | setPool 先看当前状态 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.execute` | [internal/doctor/approval/execute.go:23](../../internal/doctor/approval/execute.go) | execute 的状态检查不能靠提示词代替；直接交来 pending 单也不会执行 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.createIndex` | [internal/doctor/approval/execute.go:55](../../internal/doctor/approval/execute.go) | createIndex 只在没有目标索引、且旧执行连接已消失后才重新发送；形状不符时留给人处理 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `PoolState` | [internal/doctor/approval/model.go:24](../../internal/doctor/approval/model.go) | PoolState 与运维协议对应；医生不导入任何 shop 代码，只通过接口认识这个结构 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Plan` | [internal/doctor/approval/model.go:33](../../internal/doctor/approval/model.go) | Plan 只由 prepare 生成；HTTP 批准请求不接受这些字段 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Record` | [internal/doctor/approval/model.go:48](../../internal/doctor/approval/model.go) | Record 的 Args 保留脱敏后的原始申请，Plan 是人审阅与执行的同一份确定内容 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Repository` | [internal/doctor/approval/model.go:72](../../internal/doctor/approval/model.go) | Repository 的每个写入都检查诊断租约；进程丢失执行权后不能继续操作同一张审批 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Metadata` | [internal/doctor/approval/model.go:81](../../internal/doctor/approval/model.go) | Metadata 复用原来的 diag_ro 三连接池，只读取列、索引与线程是否存在 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.indexPlan` | [internal/doctor/approval/prepare.go:150](../../internal/doctor/approval/prepare.go) | indexPlan 先核对表、列，再引用标识符；只允许常规名字，不让字符串变成 SQL 片段 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `identifier` | [internal/doctor/approval/prepare.go:181](../../internal/doctor/approval/prepare.go) | identifier 把允许的 SQL 标识符范围收窄，不接受点号、引号或转义 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `proposal` | [internal/doctor/approval/prepare.go:23](../../internal/doctor/approval/prepare.go) | proposal 只用于读取模型输入；不接受 URL、主机、系统名或任意 SQL | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `decodeProposal` | [internal/doctor/approval/prepare.go:37](../../internal/doctor/approval/prepare.go) | decodeProposal 逐个读键，拒绝“展示一个值、执行时被第二个同名键覆盖”的输入 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Backend.prepare` | [internal/doctor/approval/prepare.go:92](../../internal/doctor/approval/prepare.go) | prepare 限制申请文字和证据数量 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `result` | [internal/doctor/approval/service.go:114](../../internal/doctor/approval/service.go) | result 只生成固定的小结果，不把数据库错误、后端地址或账号回传给模型 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `Service` | [internal/doctor/approval/service.go:25](../../internal/doctor/approval/service.go) | Service 每次从已绑定系统取后端；调用参数不能换系统或指定主机 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `New` | [internal/doctor/approval/service.go:33](../../internal/doctor/approval/service.go) | New 不联网，后端账号与工具权限都在启动时固定 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Service.List` | [internal/doctor/approval/service.go:38](../../internal/doctor/approval/service.go) | List 只提供当前配置能够实施的写工具，执行仍必须经过数据库审批 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Service.Handles` | [internal/doctor/approval/service.go:41](../../internal/doctor/approval/service.go) | Handles 使用代码中的固定名单，模型不能给普通查看工具加一个“写”属性来改变权限 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/approval (approval) | `*Service.Handle` | [internal/doctor/approval/service.go:46](../../internal/doctor/approval/service.go) | Handle 的 waiting=true 表示已经保存申请，Agent 应让出执行线程而不是轮询人有没有批准 | Agent 审批接口；Service.Handle 经仓储领取后执行 |
| internal/doctor/httpapi (httpapi) | `Approvals` | [internal/doctor/httpapi/approvals.go:22](../../internal/doctor/httpapi/approvals.go) | Approvals 只允许读取和决定，没有修改已保存参数的方法 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.listApprovals` | [internal/doctor/httpapi/approvals.go:28](../../internal/doctor/httpapi/approvals.go) | listApprovals 只读一个诊断的单子，避免页面轮询扫描全部审批 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.decideApproval` | [internal/doctor/httpapi/approvals.go:49](../../internal/doctor/httpapi/approvals.go) | decideApproval 的批准人来自签名后的登录身份，正文不能自行指定 decided_by | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.ready` | [internal/doctor/httpapi/handlers.go:116](../../internal/doctor/httpapi/handlers.go) | ready 只检查医生自己的持久化依赖，靶场正在故障不能让接收告警的医生也变成不可用 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.listDiagnoses` | [internal/doctor/httpapi/handlers.go:127](../../internal/doctor/httpapi/handlers.go) | listDiagnoses 限制页长与偏移量；未知系统不能静默返回全部系统的任务 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.diagnosis` | [internal/doctor/httpapi/handlers.go:162](../../internal/doctor/httpapi/handlers.go) | diagnosis 只接受本服务生成的 ID，查询失败时不泄露 SQL 或数据库连接细节 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.diagnosisAlerts` | [internal/doctor/httpapi/handlers.go:183](../../internal/doctor/httpapi/handlers.go) | diagnosisAlerts 只刷新小份告警快照，让已结束的报告仍能展示后来到达的症状 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.login` | [internal/doctor/httpapi/handlers.go:35](../../internal/doctor/httpapi/handlers.go) | login 对未知用户也执行一次摘要比较，避免用响应快慢直接枚举邮箱 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.receiveAlert` | [internal/doctor/httpapi/handlers.go:82](../../internal/doctor/httpapi/handlers.go) | receiveAlert 的 target 来自路径并通过令牌验证，不接受正文中另一个系统名作为覆盖 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.requireLogin` | [internal/doctor/httpapi/router.go:122](../../internal/doctor/httpapi/router.go) | requireLogin 不把解析失败的细节回显给客户端，令牌原文也不会写入日志 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.listTargets` | [internal/doctor/httpapi/router.go:152](../../internal/doctor/httpapi/router.go) | listTargets 只返回名称和工具说明，不把接入地址、账号变量或令牌发给客户端 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `*api.runTool` | [internal/doctor/httpapi/router.go:175](../../internal/doctor/httpapi/router.go) | runTool 是 doctorctl 与未来其他入口共用的调用路径，不能直接调用适配器 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `Repository` | [internal/doctor/httpapi/router.go:38](../../internal/doctor/httpapi/router.go) | Repository 定义在接口使用方，测试可以观察持久化是否被调用，而不启动 MySQL | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `Runner` | [internal/doctor/httpapi/router.go:50](../../internal/doctor/httpapi/router.go) | Runner 是页面需要的调度操作；没有提供恢复、审批或任意执行入口 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `Options` | [internal/doctor/httpapi/router.go:57](../../internal/doctor/httpapi/router.go) | Options 只接收已经装配的对象，不读取环境变量或创建第二套数据库池 | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `api` | [internal/doctor/httpapi/router.go:75](../../internal/doctor/httpapi/router.go) | api 持有服务范围的登录限流器，不能让猜密码请求无限占用 bcrypt 的 CPU | Gin 路由与网页登录用户 |
| internal/doctor/httpapi (httpapi) | `New` | [internal/doctor/httpapi/router.go:82](../../internal/doctor/httpapi/router.go) | New 把页面、任务与手动工具接到同一身份边界，告警另用系统令牌 | Gin 路由与网页登录用户 |
| internal/doctor/llm (llm) | `Ledger` | [internal/doctor/llm/cost.go:107](../../internal/doctor/llm/cost.go) | Ledger 必须原子地检查总额并预留；两个同时运行的诊断不能各花掉同一份余额 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Error` | [internal/doctor/llm/cost.go:20](../../internal/doctor/llm/cost.go) | Error 用具名字符串实现 error，让错误分类是不能被重新赋值的常量 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Error.Error` | [internal/doctor/llm/cost.go:23](../../internal/doctor/llm/cost.go) | Error 满足标准库 error 接口；errors.Is 仍可直接比较同一个常量 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Prices` | [internal/doctor/llm/cost.go:32](../../internal/doctor/llm/cost.go) | Prices 保存每百万 token 的单价，单位同样是百万分之一账户货币单位 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Money` | [internal/doctor/llm/cost.go:40](../../internal/doctor/llm/cost.go) | Money 将最多六位小数的金额转换为整数，不接受科学计数法或负数 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Prices.Cost` | [internal/doctor/llm/cost.go:66](../../internal/doctor/llm/cost.go) | Cost 分别向上取整，不因微小舍入把未花的钱误认为仍可使用 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Prices.Charge` | [internal/doctor/llm/cost.go:73](../../internal/doctor/llm/cost.go) | Charge 只信任供应商明确返回的命中数，剩余输入全部按未命中计算 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Prices.ReserveCost` | [internal/doctor/llm/cost.go:81](../../internal/doctor/llm/cost.go) | ReserveCost 按最贵的输入类别预留 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Reservation` | [internal/doctor/llm/cost.go:87](../../internal/doctor/llm/cost.go) | Reservation 在网络发送之前落库，原文已经脱敏，不能包含 Authorization 头 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Settlement` | [internal/doctor/llm/cost.go:98](../../internal/doctor/llm/cost.go) | Settlement 保存供应商回复与结算额；失败文本只用代码生成的简短原因 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `leaseKey` | [internal/doctor/llm/lease.go:14](../../internal/doctor/llm/lease.go) | leaseKey 使用私有类型，其他包不能无意中用同名字符串覆盖编号 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `WithLease` | [internal/doctor/llm/lease.go:17](../../internal/doctor/llm/lease.go) | WithLease 每次接手都换新编号；不能只用进程 ID，否则同进程再次接手会混淆旧工作 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Lease` | [internal/doctor/llm/lease.go:22](../../internal/doctor/llm/lease.go) | Lease 返回空串表示没有执行租约，例如不执行诊断的 MCP 查看请求 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Client` | [internal/doctor/llm/model.go:17](../../internal/doctor/llm/model.go) | Client 同时由真实 HTTP 客户端和脚本客户端实现；取消由同一个 ctx 一路传递 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Function` | [internal/doctor/llm/model.go:22](../../internal/doctor/llm/model.go) | Function 是模型想执行的函数；Arguments 是 JSON 字符串，仍需工具自己校验 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `ToolCall` | [internal/doctor/llm/model.go:28](../../internal/doctor/llm/model.go) | ToolCall 的 ID 只用于对话关联，不能当成数据库步骤号 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Message` | [internal/doctor/llm/model.go:35](../../internal/doctor/llm/model.go) | Message 使用非流式对话需要的字段，页面的实时更新由步骤事件提供 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Tool` | [internal/doctor/llm/model.go:43](../../internal/doctor/llm/model.go) | Tool 把注册表的参数说明包在 OpenAI 兼容协议要求的 function 中 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `ToolDefinition` | [internal/doctor/llm/model.go:49](../../internal/doctor/llm/model.go) | ToolDefinition 支持嵌套报告 Schema；普通查看工具仍沿用阶段 5 的 Schema | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Request` | [internal/doctor/llm/model.go:56](../../internal/doctor/llm/model.go) | Request 的诊断 ID 只交给本机审计，不进入外部请求或指标标签 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Thinking` | [internal/doctor/llm/model.go:70](../../internal/doctor/llm/model.go) | Thinking 是 DeepSeek 的模式字段；本项目真实调用固定 disabled | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Usage` | [internal/doctor/llm/model.go:75](../../internal/doctor/llm/model.go) | Usage 保留供应商的计量；命中的输入用较低价格结算，不能重复计入未命中部分 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm (llm) | `Response` | [internal/doctor/llm/model.go:85](../../internal/doctor/llm/model.go) | Response 即使伴随 error 也可能已有费用，调用方必须先记账再判断是否继续 | 模型适配器、Agent 与持久账本 |
| internal/doctor/llm/openai (openai) | `*Client.attempt` | [internal/doctor/llm/openai/client.go:122](../../internal/doctor/llm/openai/client.go) | attempt 在发送前记录预留额 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `*Client.settle` | [internal/doctor/llm/openai/client.go:187](../../internal/doctor/llm/openai/client.go) | settle 保留取消信号中的追踪信息，只给记账半秒；失败时预留额已经在库中，不会丢账 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `Config` | [internal/doctor/llm/openai/client.go:27](../../internal/doctor/llm/openai/client.go) | Config 的 Key 只能来自环境变量，调用者不能把整个配置写入日志 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `Client` | [internal/doctor/llm/openai/client.go:36](../../internal/doctor/llm/openai/client.go) | Client 的依赖在启动时固定，账本与脱敏器由 app 注入 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `New` | [internal/doctor/llm/openai/client.go:53](../../internal/doctor/llm/openai/client.go) | New 允许回环 HTTP 用于 httptest，真实地址必须 HTTPS；不会在构造时联网 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `*Client.Complete` | [internal/doctor/llm/openai/client.go:81](../../internal/doctor/llm/openai/client.go) | Complete 每次重试都重新记账；401、402、400、网络错误和取消不进入重试分支 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `decodeResponse` | [internal/doctor/llm/openai/protocol.go:23](../../internal/doctor/llm/openai/protocol.go) | decodeResponse 限制回复大小，保留接口 model；缺少该字段时才用请求模型 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `httpProblem` | [internal/doctor/llm/openai/protocol.go:47](../../internal/doctor/llm/openai/protocol.go) | httpProblem 的白名单不包含网络错误，避免无法确认是否计费时盲目重复生成 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `retryAfter` | [internal/doctor/llm/openai/protocol.go:65](../../internal/doctor/llm/openai/protocol.go) | retryAfter 支持秒数和 HTTP 日期；未提供时等待一秒，避免立即重试形成突发 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `wait` | [internal/doctor/llm/openai/protocol.go:77](../../internal/doctor/llm/openai/protocol.go) | wait 用 select 同时等待计时器和取消，不使用无法提前结束的 Sleep | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `validateUsage` | [internal/doctor/llm/openai/responses.go:160](../../internal/doctor/llm/openai/responses.go) | validateUsage 既防负数，也防把同一批输入同时当成命中和写入来重复抵扣费用 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `encodeRequest` | [internal/doctor/llm/openai/responses.go:21](../../internal/doctor/llm/openai/responses.go) | encodeRequest 使用非思考模式和标准价格档，不提供网络搜索或供应商托管工具 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `DecodeSavedResponse` | [internal/doctor/llm/openai/responses.go:56](../../internal/doctor/llm/openai/responses.go) | DecodeSavedResponse 同时用于收到 HTTP 回复和恢复已保存的回复，避免两套解析规则产生不同工具调用 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/llm/openai (openai) | `decodeResponses` | [internal/doctor/llm/openai/responses.go:87](../../internal/doctor/llm/openai/responses.go) | decodeResponses 缓存读和缓存写都已经包含在输入总数里；推理量也包含在输出总数里 | Agent 经 llm.Client；仓储恢复读取已保存响应 |
| internal/doctor/mcpserver (mcpserver) | `Entry` | [internal/doctor/mcpserver/audit.go:22](../../internal/doctor/mcpserver/audit.go) | Entry 的系统来自固定入口，客户端只提供工具名和参数，二者都尚未被信任 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `Auditor` | [internal/doctor/mcpserver/audit.go:29](../../internal/doctor/mcpserver/audit.go) | Auditor 把开始与结束分开，进程崩溃也不会完全抹掉这次外部调用 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `auditCalls` | [internal/doctor/mcpserver/audit.go:35](../../internal/doctor/mcpserver/audit.go) | auditCalls 包住工具分发，因此非法参数、未知工具与查询失败也都会进入审计 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `short` | [internal/doctor/mcpserver/audit.go:75](../../internal/doctor/mcpserver/audit.go) | short 按 UTF-8 边界截断，数据库不能收到半个汉字 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `Registry` | [internal/doctor/mcpserver/server.go:26](../../internal/doctor/mcpserver/server.go) | Registry 定义在使用方，只要求已有菜单与同一执行入口，不接触后端连接 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `New` | [internal/doctor/mcpserver/server.go:32](../../internal/doctor/mcpserver/server.go) | New 在启动时固定系统与账号；不会接受客户端提供的 URL、主机或系统参数 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/mcpserver (mcpserver) | `authorize` | [internal/doctor/mcpserver/server.go:64](../../internal/doctor/mcpserver/server.go) | authorize 每次验证固定系统令牌，登录 Cookie 和另一系统的令牌都不能替代它 | doctor 路由与官方 MCP SDK 请求处理 |
| internal/doctor/run (run) | `*Scheduler.Start` | [internal/doctor/run/scheduler.go:106](../../internal/doctor/run/scheduler.go) | Start 先检查过时的断点，再清理过时告警；可恢复的任务会由循环接手 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.Wake` | [internal/doctor/run/scheduler.go:122](../../internal/doctor/run/scheduler.go) | Wake 合并高频通知，后台仍定期轮询，因此丢掉重复唤醒不会丢任务 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.Close` | [internal/doctor/run/scheduler.go:130](../../internal/doctor/run/scheduler.go) | Close 先阻止分配新名额，再取消所有在途任务，最后等待 goroutine 退出 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.loop` | [internal/doctor/run/scheduler.go:144](../../internal/doctor/run/scheduler.go) | loop 每秒检查队列；通知只是加快响应，不承担持久化职责 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.dispatch` | [internal/doctor/run/scheduler.go:165](../../internal/doctor/run/scheduler.go) | dispatch 清理过时任务后再看费用，这样额度用完也不会让旧告警永久卡在 pending | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.claim` | [internal/doctor/run/scheduler.go:204](../../internal/doctor/run/scheduler.go) | claim 在一把锁内同时检查两种名额，避免两条任务都看见“还有一个空位” | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.execute` | [internal/doctor/run/scheduler.go:223](../../internal/doctor/run/scheduler.go) | execute 将取消、预算和普通失败转换为有限状态；结束写入也有独立的短预算 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `Task` | [internal/doctor/run/scheduler.go:27](../../internal/doctor/run/scheduler.go) | Task 只包含执行所需的最小字段，模型不能改变 ID 或 Target | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.Cancel` | [internal/doctor/run/scheduler.go:285](../../internal/doctor/run/scheduler.go) | Cancel 取消正在运行的 context；排队任务直接以条件更新改为 cancelled | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.Status` | [internal/doctor/run/scheduler.go:306](../../internal/doctor/run/scheduler.go) | Status 同时显示已经花掉的额度和单次预留不足的原因，页面不能只看到“没有新任务” | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `taskRecorder` | [internal/doctor/run/scheduler.go:326](../../internal/doctor/run/scheduler.go) | taskRecorder 把 agent 的一步绑定到任务 ID，并在提交后通知订阅者 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `taskRecorder.Record` | [internal/doctor/run/scheduler.go:332](../../internal/doctor/run/scheduler.go) | Record 先保存后发布，SSE 的 Last-Event-ID 才一定能在数据库里找到 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `*Scheduler.renew` | [internal/doctor/run/scheduler.go:341](../../internal/doctor/run/scheduler.go) | renew 定期证明执行者仍在工作；一次明确失去租约就取消本机请求，不能再继续花钱 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `BudgetStatus` | [internal/doctor/run/scheduler.go:35](../../internal/doctor/run/scheduler.go) | BudgetStatus 同时供调度器、页面和指标读取，金额是百万分之一账户货币单位 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `minTime` | [internal/doctor/run/scheduler.go:384](../../internal/doctor/run/scheduler.go) | minTime 选较早的截止时间，续约查询本身也不能越过旧租约的保守期限 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `Repository` | [internal/doctor/run/scheduler.go:45](../../internal/doctor/run/scheduler.go) | Repository 把状态与步骤保存放在同一个边界，假仓储可完整验证调度行为 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `Config` | [internal/doctor/run/scheduler.go:58](../../internal/doctor/run/scheduler.go) | Config 保存进程内调度参数；Resolve 在任务开始时返回已固定系统的工具与说明 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `running` | [internal/doctor/run/scheduler.go:73](../../internal/doctor/run/scheduler.go) | running 保存取消函数和系统名，mu 保护这张表与 stopped、blocked | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `Scheduler` | [internal/doctor/run/scheduler.go:80](../../internal/doctor/run/scheduler.go) | Scheduler 的 worker 都计入 WaitGroup，Close 等它们退出后才允许 app 关数据库 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `New` | [internal/doctor/run/scheduler.go:95](../../internal/doctor/run/scheduler.go) | New 只检查配置，Start 完成数据库清理后才真正接受任务 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/run (run) | `Transition` | [internal/doctor/run/state.go:25](../../internal/doctor/run/state.go) | Transition 的表只允许向前推进，不能把终态重新排队 | app 启动、调度循环和 HTTP 取消 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.Columns` | [internal/doctor/source/mysqlinspect/write_metadata.go:17](../../internal/doctor/source/mysqlinspect/write_metadata.go) | Columns 使用绑定参数查固定库的列名，不能通过表名跨库 | 只读工具与审批元数据查询 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.IndexShape` | [internal/doctor/source/mysqlinspect/write_metadata.go:33](../../internal/doctor/source/mysqlinspect/write_metadata.go) | IndexShape 比较完整列顺序和唯一性，不把碰巧同名的另一种索引当作执行成功 | 只读工具与审批元数据查询 |
| internal/doctor/source/mysqlinspect (mysqlinspect) | `*Client.ConnectionAlive` | [internal/doctor/source/mysqlinspect/write_metadata.go:52](../../internal/doctor/source/mysqlinspect/write_metadata.go) | ConnectionAlive 只返回线程是否存在，不读取当前 SQL 文本或锁里的业务值 | 只读工具与审批元数据查询 |
| internal/doctor/store (store) | `*Store.DecideApproval` | [internal/doctor/store/approvals.go:123](../../internal/doctor/store/approvals.go) | DecideApproval 只有一个请求能把仍有效的 pending 单改掉，HTTP 重试不会二次决定 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.ClaimApproval` | [internal/doctor/store/approvals.go:169](../../internal/doctor/store/approvals.go) | ClaimApproval 把批准与执行分开记录，已在执行的单子只能由当前诊断租约的持有者恢复 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.ApprovalConnection` | [internal/doctor/store/approvals.go:206](../../internal/doctor/store/approvals.go) | ApprovalConnection 在发送 DDL 前保存连接编号，恢复者可以先检查旧操作是否还在运行 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.FinishApproval` | [internal/doctor/store/approvals.go:227](../../internal/doctor/store/approvals.go) | FinishApproval 只有当前租约能保存结果；旧执行者的迟到回复不会盖掉恢复者的核对 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.ExpireApprovals` | [internal/doctor/store/approvals.go:262](../../internal/doctor/store/approvals.go) | ExpireApprovals 不需要人在页面点拒绝；到期后任务会被调度器继续，让模型得到明确的未执行结果 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.FindApproval` | [internal/doctor/store/approvals.go:26](../../internal/doctor/store/approvals.go) | FindApproval 总是同时限定诊断编号，不能拿另一个诊断的审批结果接续当前任务 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.expireApproval` | [internal/doctor/store/approvals.go:292](../../internal/doctor/store/approvals.go) | expireApproval 与人工决定使用相同的锁顺序，过期状态和页面事件一起落库 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `scanner` | [internal/doctor/store/approvals.go:35](../../internal/doctor/store/approvals.go) | scanner 让 QueryRow 和 Rows 使用同一套解码，避免列表与执行读出不同参数 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `scanApproval` | [internal/doctor/store/approvals.go:38](../../internal/doctor/store/approvals.go) | scanApproval 不给损坏的 JSON 补默认值，无法准确恢复参数时必须停下 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Approvals` | [internal/doctor/store/approvals.go:66](../../internal/doctor/store/approvals.go) | Approvals 返回本次诊断的审批单，页面只能通过它展示已经保存的内容 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.CreateApproval` | [internal/doctor/store/approvals.go:84](../../internal/doctor/store/approvals.go) | CreateApproval 在同一事务中保存计划和页面步骤；崩溃时不会只保存其中一半 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Ledger.Settle` | [internal/doctor/store/budget.go:120](../../internal/doctor/store/budget.go) | Settle 可重复调用但只结算一次，避免重试数据库写入时重复退钱 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Ledger.Status` | [internal/doctor/store/budget.go:179](../../internal/doctor/store/budget.go) | Status 读的是整个真实账户，包含其他诊断预留但还没结算的钱 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Ledger` | [internal/doctor/store/budget.go:22](../../internal/doctor/store/budget.go) | Ledger 保存不随诊断变化的累计账户，单位统一为百万分之一元 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.NewLedger` | [internal/doctor/store/budget.go:30](../../internal/doctor/store/budget.go) | NewLedger 创建空账本但绝不清零已有记录，配置降低上限也不会隐藏已花费用 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.NewCurrencyLedger` | [internal/doctor/store/budget.go:35](../../internal/doctor/store/budget.go) | NewCurrencyLedger 拒绝将已有人民币账户重新解释成美元；账户名相同也不能改变金额单位 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Ledger.Reserve` | [internal/doctor/store/budget.go:59](../../internal/doctor/store/budget.go) | Reserve 先锁预算再保存请求，两个动作一起提交后调用方才能联网 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Diagnosis` | [internal/doctor/store/diagnoses.go:26](../../internal/doctor/store/diagnoses.go) | Diagnosis 是列表所需的最小摘要，不包含账号或工具凭据 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Detail` | [internal/doctor/store/diagnoses.go:39](../../internal/doctor/store/diagnoses.go) | Detail 的嵌入字段使摘要与列表保持一致，避免维护两套字段映射 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.List` | [internal/doctor/store/diagnoses.go:62](../../internal/doctor/store/diagnoses.go) | List 只把筛选值绑定到参数；排序和 SQL 结构由代码固定 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Detail` | [internal/doctor/store/diagnoses.go:96](../../internal/doctor/store/diagnoses.go) | Detail 只按固定 ID 查一次任务，再取相关告警，不允许调用者指定表或字段 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Renew` | [internal/doctor/store/leases.go:121](../../internal/doctor/store/leases.go) | Renew 的条件更新不允许续活已经过期的租约；迟到者必须退出，不能自行复活 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.staleLeases` | [internal/doctor/store/leases.go:131](../../internal/doctor/store/leases.go) | staleLeases 和接手共用协调行，查出过时任务后不会误把刚刚获得新租约的任务结束掉 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Resume` | [internal/doctor/store/leases.go:190](../../internal/doctor/store/leases.go) | Resume 先补齐“已经记账但没保存回复步骤”的空窗，再返回完整历史 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `requireLease` | [internal/doctor/store/leases.go:24](../../internal/doctor/store/leases.go) | requireLease 必须在持有诊断行锁的事务中调用，检查和写入之间不能让别人接手 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.claimLease` | [internal/doctor/store/leases.go:37](../../internal/doctor/store/leases.go) | claimLease 用一行短事务锁同时检查全局两个名额、系统一个名额，防止两进程各自超配 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.BeginMCP` | [internal/doctor/store/mcp.go:19](../../internal/doctor/store/mcp.go) | BeginMCP 保存成功后才返回审计 ID，调用方此时才能访问目标系统 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.EndMCP` | [internal/doctor/store/mcp.go:30](../../internal/doctor/store/mcp.go) | EndMCP 只接受有限结果分类，错误正文不能借审计状态字段泄露后端信息 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.recoverModelCalls` | [internal/doctor/store/model_recovery.go:22](../../internal/doctor/store/model_recovery.go) | recoverModelCalls 只查看本次诊断的发送意图，不能用另一个诊断的回答续接对话 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.recoverOneCall` | [internal/doctor/store/model_recovery.go:55](../../internal/doctor/store/model_recovery.go) | recoverOneCall 把 HTTP 重试仍合成一轮模型调用，所有失败尝试的费用都保留 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `nullableKey` | [internal/doctor/store/steps.go:113](../../internal/doctor/store/steps.go) | nullableKey 让旧记录和无须去重的状态事件使用 SQL NULL，避免空串互相冲突 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `readStep` | [internal/doctor/store/steps.go:121](../../internal/doctor/store/steps.go) | readStep 在同一事务内返回已保存的完整结果，重试保存不会分配第二个步骤号 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.RecordStep` | [internal/doctor/store/steps.go:22](../../internal/doctor/store/steps.go) | RecordStep 在数据库提交之后才发布事件，失败时不会产生只有页面看得到的虚假步骤 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.appendStep` | [internal/doctor/store/steps.go:40](../../internal/doctor/store/steps.go) | appendStep 被普通步骤和状态转换共用，使终态事件也有可补发的数据库编号 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.publish` | [internal/doctor/store/steps.go:84](../../internal/doctor/store/steps.go) | publish 不持有数据库锁；回调只能做有界的内存投递，慢浏览器不能拖住事务 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.StepsAfter` | [internal/doctor/store/steps.go:91](../../internal/doctor/store/steps.go) | StepsAfter 每次最多读取一百步，SSE 会继续翻页，不把所有历史一次塞进内存 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Store` | [internal/doctor/store/store.go:35](../../internal/doctor/store/store.go) | Store 不向 HTTP 层暴露 sql.DB，接口不能随意拼接 SQL | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Config` | [internal/doctor/store/store.go:41](../../internal/doctor/store/store.go) | Config 把时间和通知作为依赖注入，测试无需真的等待分组过期 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `New` | [internal/doctor/store/store.go:55](../../internal/doctor/store/store.go) | New 只装配已有池，连接检查和生命周期由 app 统一负责 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `NewConfigured` | [internal/doctor/store/store.go:58](../../internal/doctor/store/store.go) | NewConfigured 保留无网络的构造过程，缺省时间与脱敏器也适用于集成测试 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `Migrate` | [internal/doctor/store/store.go:82](../../internal/doctor/store/store.go) | Migrate 复用命名锁和校验和，两个误启动的进程也不会同时执行同一迁移 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `newID` | [internal/doctor/store/store.go:87](../../internal/doctor/store/store.go) | newID 用随机 16 字节编码为 32 位十六进制；ID 不包含账号或告警内容 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.EndTask` | [internal/doctor/store/tasks.go:103](../../internal/doctor/store/tasks.go) | EndTask 用累计结果覆盖最终计数，失败时也保存已经发生的用量 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.CancelPending` | [internal/doctor/store/tasks.go:121](../../internal/doctor/store/tasks.go) | CancelPending 和启动竞争同一行的 pending 状态，只有先拿到行锁的操作成功 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.changeState` | [internal/doctor/store/tasks.go:139](../../internal/doctor/store/tasks.go) | changeState 仅接受本文件拼出的字段片段，外部内容始终用参数绑定 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Manual` | [internal/doctor/store/tasks.go:24](../../internal/doctor/store/tasks.go) | Manual 只负责保存已由接口确认存在的系统，症状先脱敏再落库 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Pending` | [internal/doctor/store/tasks.go:35](../../internal/doctor/store/tasks.go) | Pending 先选出每个系统最老的一条，避免某系统大量排队挡住另一系统 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Recover` | [internal/doctor/store/tasks.go:53](../../internal/doctor/store/tasks.go) | Recover 只结束中断太久的任务；有效的过期租约由调度器接手并重建对话 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.Expire` | [internal/doctor/store/tasks.go:61](../../internal/doctor/store/tasks.go) | Expire 只清理告警触发的旧 pending，手动任务不会被这条告警时效规则取消 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.endSelected` | [internal/doctor/store/tasks.go:66](../../internal/doctor/store/tasks.go) | endSelected 先关闭结果集再逐条写入，连接池较小时也不占着读连接等待写连接 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/store (store) | `*Store.StartTask` | [internal/doctor/store/tasks.go:98](../../internal/doctor/store/tasks.go) | StartTask 记录本次选用的模型与提示版本，不能沿用上一个进程的隐含配置 | 调度器、HTTP 审批、模型账本或 MCP 审计 |
| internal/doctor/target (target) | `opsConfig` | [internal/doctor/target/actions.go:21](../../internal/doctor/target/actions.go) | opsConfig 只保存部署者给的根地址和变量名，模型没有修改这些字段的入口 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.Actions` | [internal/doctor/target/actions.go:30](../../internal/doctor/target/actions.go) | Actions 不导出账号，只把已经绑定当前系统的后端交给审批服务 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.bindActions` | [internal/doctor/target/actions.go:33](../../internal/doctor/target/actions.go) | bindActions 必须在只读原生来源装配后调用，审批元数据复用原来的三连接池 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `Load` | [internal/doctor/target/catalog.go:100](../../internal/doctor/target/catalog.go) | Load 先检查所有凭据冲突，再创建任何客户端，防止装配一半才发现系统共用身份 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `credentials` | [internal/doctor/target/catalog.go:132](../../internal/doctor/target/catalog.go) | credentials 除了变量名，还检查账号身份；换一个变量名不能把同一个账号重复接进来 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `credentialIdentity` | [internal/doctor/target/catalog.go:159](../../internal/doctor/target/catalog.go) | credentialIdentity 不包含密码；只要主机和用户名一样，就已经是同一个账号 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `build` | [internal/doctor/target/catalog.go:184](../../internal/doctor/target/catalog.go) | build 把具体后端适配器装进接口；未配置的数据源不会生成对应工具 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.bindHTTP` | [internal/doctor/target/catalog.go:215](../../internal/doctor/target/catalog.go) | bindHTTP 装配指标、调用链和日志查询，关闭函数与客户端一起登记 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.bindNative` | [internal/doctor/target/catalog.go:245](../../internal/doctor/target/catalog.go) | bindNative 装配需要独立账号的数据库，以及配置里逐个列出的进程管理端口 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.MCPHandler` | [internal/doctor/target/catalog.go:276](../../internal/doctor/target/catalog.go) | MCPHandler 不向 app 导出令牌，直接构造一个只能访问本系统只读工具的入口 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `System` | [internal/doctor/target/catalog.go:40](../../internal/doctor/target/catalog.go) | System 不导出令牌与后端连接，接口层只能使用固定注册表 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `Catalog` | [internal/doctor/target/catalog.go:55](../../internal/doctor/target/catalog.go) | Catalog 不公开内部 map，调用方无法在运行中替换系统 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.Name` | [internal/doctor/target/catalog.go:58](../../internal/doctor/target/catalog.go) | Name 返回稳定系统名，不把架构说明或凭据混到任务标识里 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.RecoveryWindow` | [internal/doctor/target/catalog.go:61](../../internal/doctor/target/catalog.go) | RecoveryWindow 让接收告警的一侧使用本系统的观察期，不需要了解具体告警规则 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.Tools` | [internal/doctor/target/catalog.go:64](../../internal/doctor/target/catalog.go) | Tools 返回已经固定数据源的注册表；注册表本身没有运行时增删接口 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*System.Authenticate` | [internal/doctor/target/catalog.go:67](../../internal/doctor/target/catalog.go) | Authenticate 对两个摘要作定长比较，错误令牌不会因为碰巧是另一个系统的令牌而通过 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*Catalog.Get` | [internal/doctor/target/catalog.go:73](../../internal/doctor/target/catalog.go) | Get 精确匹配系统名，不把不存在的名字退回默认 shop | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*Catalog.Names` | [internal/doctor/target/catalog.go:79](../../internal/doctor/target/catalog.go) | Names 排序后返回副本，CLI 输出不会随着 map 的遍历顺序变化 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `*Catalog.Close` | [internal/doctor/target/catalog.go:89](../../internal/doctor/target/catalog.go) | Close 即使一条连接关闭失败，也继续关闭其他系统的连接 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `Config.variables` | [internal/doctor/target/config.go:138](../../internal/doctor/target/config.go) | variables 只包含本阶段真正使用的凭据，不提前向医生传递写账号 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `Config` | [internal/doctor/target/config.go:27](../../internal/doctor/target/config.go) | Config 只保存地址、允许范围和变量名；凭据值仅从环境变量读取 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `readConfigs` | [internal/doctor/target/config.go:45](../../internal/doctor/target/config.go) | readConfigs 按文件名顺序读取全部 YAML，目录为空也报错，避免“启动成功但什么都接不上” | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `readFile` | [internal/doctor/target/config.go:74](../../internal/doctor/target/config.go) | readFile 只接受目录里的普通文件，配置不能通过 ../ 或符号链接读到实验答案 | app 装配与每次诊断的 Resolve |
| internal/doctor/target (target) | `validateConfigs` | [internal/doctor/target/config.go:91](../../internal/doctor/target/config.go) | validateConfigs 先查跨系统冲突，再查前缀，错误能直接说明两个系统共用了哪个变量 | app 装配与每次诊断的 Resolve |
| internal/shop/app (app) | `StartOps` | [internal/shop/app/ops.go:29](../../internal/shop/app/ops.go) | StartOps 借用业务连接池，不创建高权限连接；商品缓存只由订单服务提供 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/inventoryservice (inventoryservice) | `normalServeError` | [internal/shop/app/inventoryservice/app.go:124](../../internal/shop/app/inventoryservice/app.go) | normalServeError 只过滤预期的关闭结果 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/inventoryservice (inventoryservice) | `Run` | [internal/shop/app/inventoryservice/app.go:38](../../internal/shop/app/inventoryservice/app.go) | Run 拥有库存进程的完整生命周期 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/inventoryservice (inventoryservice) | `serve` | [internal/shop/app/inventoryservice/app.go:88](../../internal/shop/app/inventoryservice/app.go) | serve 在两个端口都绑定成功后才接收请求 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderservice (orderservice) | `connectInventory` | [internal/shop/app/orderservice/app.go:104](../../internal/shop/app/orderservice/app.go) | connectInventory 把首次 DNS 查询和握手放进启动预算，避免第一位用户替服务承担建连等待 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderservice (orderservice) | `routes` | [internal/shop/app/orderservice/app.go:123](../../internal/shop/app/orderservice/app.go) | routes 最外层建立 span 并计时，连被限流拒绝的请求也能被观测到 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderservice (orderservice) | `serve` | [internal/shop/app/orderservice/app.go:148](../../internal/shop/app/orderservice/app.go) | serve 等两个端口都绑定成功才开始接流量，避免只启动了半个进程 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderservice (orderservice) | `normalHTTPError` | [internal/shop/app/orderservice/app.go:182](../../internal/shop/app/orderservice/app.go) | normalHTTPError 只忽略标准的正常关闭错误，绑定后的监听故障仍交给入口返回失败码 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderservice (orderservice) | `Run` | [internal/shop/app/orderservice/app.go:55](../../internal/shop/app/orderservice/app.go) | Run 的 result 是具名返回值，defer 可以把关闭连接的错误一并交给入口 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderworker (orderworker) | `serve` | [internal/shop/app/orderworker/app.go:105](../../internal/shop/app/orderworker/app.go) | serve 在管理端口可用后启动消费，任意循环失败都会取消另一侧并读齐退出结果 | 对应 cmd 入口与服务生命周期 |
| internal/shop/app/orderworker (orderworker) | `Run` | [internal/shop/app/orderworker/app.go:43](../../internal/shop/app/orderworker/app.go) | Run 先停消费者，再关闭 Kafka 和数据库，不能在当前事务尚未完成时拆掉连接 | 对应 cmd 入口与服务生命周期 |
| internal/shop/ops (ops) | `*Handler.warm` | [internal/shop/ops/handler.go:134](../../internal/shop/ops/handler.go) | warm 只回源缺失项；崩溃后重试不会改业务数据，已经存在的缓存也不被刷新 TTL | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `decode` | [internal/shop/ops/handler.go:182](../../internal/shop/ops/handler.go) | decode 拒绝重复字段和未知字段，不能让审批展示的参数与执行器理解的不一样 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `respond` | [internal/shop/ops/handler.go:222](../../internal/shop/ops/handler.go) | respond 的内容都由服务生成，接口不会返回缓存里的商品资料 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `*Handler.Modified` | [internal/shop/ops/handler.go:228](../../internal/shop/ops/handler.go) | Modified 记录是否曾在本次进程生命周期里改过池；即使手工改回同一个值，也要求重启恢复基线 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `Pool` | [internal/shop/ops/handler.go:32](../../internal/shop/ops/handler.go) | Pool 只暴露当前上限与修改方法，测试可以用小型替身而不连接 MySQL | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `PoolState` | [internal/shop/ops/handler.go:38](../../internal/shop/ops/handler.go) | PoolState 把进程身份与旧值一起交给审批，重启后的同一个数值不代表同一份状态 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `Handler` | [internal/shop/ops/handler.go:47](../../internal/shop/ops/handler.go) | Handler 的锁只保护池状态；预热按最多一百个 ID 顺序处理，不无限开 goroutine | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `New` | [internal/shop/ops/handler.go:57](../../internal/shop/ops/handler.go) | New 不给短令牌补默认值；只有显式配置了身份才开放运维入口 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `*Handler.ServeHTTP` | [internal/shop/ops/handler.go:68](../../internal/shop/ops/handler.go) | ServeHTTP 先鉴权再解析，错误响应不带令牌、数据库错误或商品内容 | StartOps 挂载的内部 HTTP 路由 |
| internal/shop/ops (ops) | `*Handler.pool` | [internal/shop/ops/handler.go:94](../../internal/shop/ops/handler.go) | pool 的比较与修改在同一把锁中，两个审批不会同时依据同一个旧值执行 | StartOps 挂载的内部 HTTP 路由 |
| tools/archcheck (main) | `doctorMountProblems` | [tools/archcheck/doctor.go:22](../../tools/archcheck/doctor.go) | doctorMountProblems 使用完整 YAML 解析，长格式挂载也不能绕过只接受固定字符串的规则 | 命令检查入口与对应测试 |
| tools/archcheck (main) | `opsPortProblems` | [tools/archcheck/doctor.go:62](../../tools/archcheck/doctor.go) | opsPortProblems 同时识别 Compose 两种端口格式，不能只靠“没有写 localhost”猜是否公开 | 命令检查入口与对应测试 |

## 测试导航

列出本阶段涉及测试文件中的全部 Test 入口，既有用例保留作为回归验证；具体新增覆盖见各文件说明。

| 测试 | 文件 | 验证什么 |
|---|---|---|
| `TestRepairBaseline` | [internal/chaos/verify/repairs_test.go:21](../../internal/chaos/verify/repairs_test.go) | TestRepairBaseline 每个用例分别控制索引与池证据，不靠告警恰好是否正在 firing。 |
| `TestApprovalReplay` | [internal/doctor/agent/approval_test.go:49](../../internal/doctor/agent/approval_test.go) | TestApprovalReplay 批准、拒绝和过期都只产生一个原调用的结果，等待本身不消耗模型轮数。 |
| `TestMixedWrite` | [internal/doctor/agent/approval_test.go:104](../../internal/doctor/agent/approval_test.go) | TestMixedWrite 写请求和报告同轮出现时，没有任何申请或执行机会。 |
| `TestBudgetRevision` | [internal/doctor/agent/budget_test.go:23](../../internal/doctor/agent/budget_test.go) | TestBudgetRevision 第九轮仍能继续取证，不会再被旧的八万 token 触发线打断。 |
| `TestFinalCorrection` | [internal/doctor/agent/budget_test.go:42](../../internal/doctor/agent/budget_test.go) | TestFinalCorrection 第一次交报告故意编造步骤；只允许再修一次，不能反复占用额度。 |
| `TestInputBound` | [internal/doctor/agent/budget_test.go:74](../../internal/doctor/agent/budget_test.go) | TestInputBound 重发的历史按上次真实计量，只有新增部分按字节作最坏估计。 |
| `TestHardBudget` | [internal/doctor/agent/budget_test.go:85](../../internal/doctor/agent/budget_test.go) | TestHardBudget 初始请求就超过上下文或累计量时，不允许为了最后报告再绕过上限。 |
| `TestLoopCorrection` | [internal/doctor/agent/loop_test.go:89](../../internal/doctor/agent/loop_test.go) | TestLoopCorrection 先调用不存在的工具，再查证据，编造步骤被退回后模型修正报告。 |
| `TestMixedReport` | [internal/doctor/agent/loop_test.go:153](../../internal/doctor/agent/loop_test.go) | TestMixedReport 模型不能一边索要新证据，一边抢先用旧证据结束诊断。 |
| `TestBudget` | [internal/doctor/agent/loop_test.go:176](../../internal/doctor/agent/loop_test.go) | TestBudget 每种预算触发后只给一次交报告机会，最后仍请求查询也不会真的执行。 |
| `TestParallelCancel` | [internal/doctor/agent/loop_test.go:210](../../internal/doctor/agent/loop_test.go) | TestParallelCancel 四个查看同时进入等待；取消后所有查询退出，synctest 会发现残留 goroutine。 |
| `TestReportBoundaries` | [internal/doctor/agent/loop_test.go:245](../../internal/doctor/agent/loop_test.go) | TestReportBoundaries 报告字段和建议工具都由代码限制，不依赖提示词是否被模型遵守。 |
| `TestTimeBudget` | [internal/doctor/agent/loop_test.go:270](../../internal/doctor/agent/loop_test.go) | TestTimeBudget 使用虚拟时钟走完普通阶段和最后机会，证明总时限不会因整理报告而延长。 |
| `TestArgumentCorrection` | [internal/doctor/agent/loop_test.go:292](../../internal/doctor/agent/loop_test.go) | TestArgumentCorrection 使用真实注册表，证明多传主机参数只会得到错误，不会中止循环。 |
| `TestReplayPartialTools` | [internal/doctor/agent/replay_test.go:23](../../internal/doctor/agent/replay_test.go) | TestReplayPartialTools 复现两项并行查询只保存一项时退出，旧结果不能被重复查询覆盖。 |
| `TestUntrustedFrame` | [internal/doctor/agent/replay_test.go:74](../../internal/doctor/agent/replay_test.go) | TestUntrustedFrame 尝试闭合 JSON 和 XML 标记，解码后仍必须完整落在唯一的数据字符串里。 |
| `TestReplayFinalInterrupted` | [internal/doctor/agent/replay_test.go:91](../../internal/doctor/agent/replay_test.go) | TestReplayFinalInterrupted 区分最后报告中断一次与两次，恢复也必须遵守只修正一次的上限。 |
| `TestTitle` | [internal/doctor/alerts/service_test.go:17](../../internal/doctor/alerts/service_test.go) | TestTitle 不需要分组标签，也能从第一批通知生成具体标题。 |
| `TestTitleBound` | [internal/doctor/alerts/service_test.go:40](../../internal/doctor/alerts/service_test.go) | TestTitleBound 给数量后缀留空间，长告警名称也不能导致保存通知失败。 |
| `TestKnownSecrets` | [internal/doctor/app/agent_config_test.go:21](../../internal/doctor/app/agent_config_test.go) | TestKnownSecrets 模拟容器只收到 DSN 和 URL，证明两种连接串里的密码都被收集。 |
| `TestAgentDefaults` | [internal/doctor/app/agent_config_test.go:32](../../internal/doctor/app/agent_config_test.go) | TestAgentDefaults 明确证明 Mock 配置不读取模型 Key；测试用读取器遇到它就失败。 |
| `TestAgentConfigRejects` | [internal/doctor/app/agent_config_test.go:45](../../internal/doctor/app/agent_config_test.go) | TestAgentConfigRejects 每个用例只改一个边界，断言启动拒绝而不是退回默认值。 |
| `TestIntegrationIndexPermissions` | [internal/doctor/approval/permissions_integration_test.go:24](../../internal/doctor/approval/permissions_integration_test.go) | TestIntegrationIndexPermissions 账号误获写权限时也不修改业务行，但测试会立即失败。 |
| `TestWriteArguments` | [internal/doctor/approval/prepare_test.go:43](../../internal/doctor/approval/prepare_test.go) | TestWriteArguments 每个拒绝理由都是独立边界，不能用缩小允许范围来掩盖缺失校验。 |
| `TestIndexRecovery` | [internal/doctor/approval/prepare_test.go:91](../../internal/doctor/approval/prepare_test.go) | TestIndexRecovery 崩溃后的真实索引已经存在时，不再发送 DDL；不同形状也不会当成功。 |
| `TestSavedArguments` | [internal/doctor/approval/service_test.go:64](../../internal/doctor/approval/service_test.go) | TestSavedArguments 用已批准的缓存预热证明模型后来的参数不会进入执行器。 |
| `TestPoolRecovery` | [internal/doctor/approval/service_test.go:115](../../internal/doctor/approval/service_test.go) | TestPoolRecovery 已执行、外部修改和进程重启都只能读取确认，不能盲目再次 POST。 |
| `TestApprovalHTTP` | [internal/doctor/httpapi/approvals_test.go:42](../../internal/doctor/httpapi/approvals_test.go) | TestApprovalHTTP 参数来自原单；人点击按钮只改变决定状态。 |
| `TestRedaction` | [internal/doctor/llm/boundary_test.go:16](../../internal/doctor/llm/boundary_test.go) | TestRedaction 覆盖密钥、Authorization、JWT、赋值语法和嵌套日志，不依赖模型自觉保密。 |
| `TestMoney` | [internal/doctor/llm/boundary_test.go:44](../../internal/doctor/llm/boundary_test.go) | TestMoney 不接受模糊的计价格式，并且很小的费用也向上记到账本里。 |
| `TestCacheCharge` | [internal/doctor/llm/boundary_test.go:63](../../internal/doctor/llm/boundary_test.go) | TestCacheCharge 结算只给明确命中的部分优惠，缺命中字段时全部按未命中计费。 |
| `TestHTTPPolicy` | [internal/doctor/llm/openai/client_test.go:85](../../internal/doctor/llm/openai/client_test.go) | TestHTTPPolicy 用虚拟时钟检查确切等待时间，计算机忙不会改变结果。 |
| `TestWireAudit` | [internal/doctor/llm/openai/client_test.go:109](../../internal/doctor/llm/openai/client_test.go) | TestWireAudit 核对真实发送字节就是落库字节，模型名和缓存 token 不会丢失。 |
| `TestCancel` | [internal/doctor/llm/openai/client_test.go:137](../../internal/doctor/llm/openai/client_test.go) | TestCancel 的 Transport 一直等 context，取消必须传到它，且不会再发第二次请求。 |
| `TestCancelBody` | [internal/doctor/llm/openai/client_test.go:178](../../internal/doctor/llm/openai/client_test.go) | TestCancelBody 防止只有 HTTP.Do 的取消被记录，读正文中断却变成普通解析错误。 |
| `TestResponsesRoundtrip` | [internal/doctor/llm/openai/responses_test.go:24](../../internal/doctor/llm/openai/responses_test.go) | TestResponsesRoundtrip 把模型第一轮的函数调用接到第二轮输入，确认没有丢失 ID 或增加托管工具。 |
| `TestResponsesUsage` | [internal/doctor/llm/openai/responses_test.go:79](../../internal/doctor/llm/openai/responses_test.go) | TestResponsesUsage 计量矛盾不能当零费用；合法但截断的回复仍保留已知用量。 |
| `TestClientBoundary` | [internal/doctor/mcpserver/server_test.go:71](../../internal/doctor/mcpserver/server_test.go) | TestClientBoundary 合法、非法参数与不存在的写工具都要留审计，列表只能看见只读工具。 |
| `TestTokenIsolation` | [internal/doctor/mcpserver/server_test.go:115](../../internal/doctor/mcpserver/server_test.go) | TestTokenIsolation A 的令牌不能访问 B；就算地址正确，也要先被 HTTP 身份边界挡住。 |
| `TestTransitions` | [internal/doctor/run/scheduler_test.go:175](../../internal/doctor/run/scheduler_test.go) | TestTransitions 穷举全部状态对；终态回到 running 等情况必须拒绝。 |
| `TestSchedulerIsolation` | [internal/doctor/run/scheduler_test.go:188](../../internal/doctor/run/scheduler_test.go) | TestSchedulerIsolation 五个 A 只占一个名额，B 同时运行；取消后全部 worker 都退出。 |
| `TestSchedulerBudgetAndExpiry` | [internal/doctor/run/scheduler_test.go:226](../../internal/doctor/run/scheduler_test.go) | TestSchedulerBudgetAndExpiry 额度已满也要清理过时任务，而且不能启动任意付费调用。 |
| `TestCancelBeforeStart` | [internal/doctor/run/scheduler_test.go:246](../../internal/doctor/run/scheduler_test.go) | TestCancelBeforeStart 取消已登记但尚未开始的任务，不能等下次轮询又把它拿起来执行。 |
| `TestBudgetStatusReason` | [internal/doctor/run/scheduler_test.go:268](../../internal/doctor/run/scheduler_test.go) | TestBudgetStatusReason 防止一次预留失败盖掉更需要人工核对的持久停用原因。 |
| `TestRenewUnavailable` | [internal/doctor/run/scheduler_test.go:315](../../internal/doctor/run/scheduler_test.go) | TestRenewUnavailable 用虚拟时钟等到租约边界，避免依赖真实机器的调度速度。 |
| `TestIntegrationFaultPeriod` | [internal/doctor/store/agent_integration_test.go:28](../../internal/doctor/store/agent_integration_test.go) | TestIntegrationFaultPeriod 结束诊断不关闭故障；整组恢复和静默超时才允许新建。 |
| `TestIntegrationTasks` | [internal/doctor/store/agent_integration_test.go:76](../../internal/doctor/store/agent_integration_test.go) | TestIntegrationTasks 证明并发步骤连续、取消不能被完成覆盖、启动清理留下明确原因。 |
| `TestIntegrationBudget` | [internal/doctor/store/agent_integration_test.go:136](../../internal/doctor/store/agent_integration_test.go) | TestIntegrationBudget 并发十次只能预留三次；失败回滚不能把总额冲到上限之外。 |
| `TestIntegrationDiagnosisBudget` | [internal/doctor/store/agent_integration_test.go:217](../../internal/doctor/store/agent_integration_test.go) | TestIntegrationDiagnosisBudget 诊断上限包含未结算尝试；换预算账户也不能绕过它。 |
| `TestIntegrationApproval` | [internal/doctor/store/approval_integration_test.go:44](../../internal/doctor/store/approval_integration_test.go) | TestIntegrationApproval 人不决定时不会重跑；两个请求同时决定也只有一个成功。 |
| `TestIntegrationApprovalExpiry` | [internal/doctor/store/approval_integration_test.go:135](../../internal/doctor/store/approval_integration_test.go) | TestIntegrationApprovalExpiry 过期审批无法批准，取消后的审批也不能重新激活任务。 |
| `TestIntegrationLeaseTakeover` | [internal/doctor/store/lease_integration_test.go:22](../../internal/doctor/store/lease_integration_test.go) | TestIntegrationLeaseTakeover 活租约不能被抢，接手之后旧持有者的步骤和费用预留都被拒绝。 |
| `TestIntegrationModelRecovery` | [internal/doctor/store/lease_integration_test.go:74](../../internal/doctor/store/lease_integration_test.go) | TestIntegrationModelRecovery 分别模拟成功结算后丢失步骤，以及崩溃时还不知道请求是否扣费。 |
| `TestIntegrationCurrency` | [internal/doctor/store/lease_integration_test.go:140](../../internal/doctor/store/lease_integration_test.go) | TestIntegrationCurrency 旧人民币账本不能改成美元，也不能用美元账本替人民币诊断付款。 |
| `TestIntegrationMCPAudit` | [internal/doctor/store/mcp_integration_test.go:21](../../internal/doctor/store/mcp_integration_test.go) | TestIntegrationMCPAudit 保存时先 running，再变 ok；重放失败结果不能覆盖已经完成的审计。 |
| `TestIntegrationRecoveryWindow` | [internal/doctor/store/recovery_integration_test.go:24](../../internal/doctor/store/recovery_integration_test.go) | TestIntegrationRecoveryWindow 先结束报告，再接收恢复与后续症状，证明不会新增付费任务。 |
| `TestIsolation` | [internal/doctor/target/catalog_test.go:29](../../internal/doctor/target/catalog_test.go) | TestIsolation 使用真正的 HTTP 适配器与工具注册表，不用假 Registry 代替接线。 |
| `TestConfigFailures` | [internal/doctor/target/catalog_test.go:85](../../internal/doctor/target/catalog_test.go) | TestConfigFailures 在创建连接之前报错，而且错误不会带出密码或完整连接串。 |
| `TestCredentialReuse` | [internal/doctor/target/catalog_test.go:109](../../internal/doctor/target/catalog_test.go) | TestCredentialReuse 覆盖“变量名不同，但实际值仍共用”的部署错误。 |
| `TestRecoveryWindowConfig` | [internal/doctor/target/catalog_test.go:142](../../internal/doctor/target/catalog_test.go) | TestRecoveryWindowConfig 让部署者必须明确选择时长，不能把拼错或缺项当成立即关闭。 |
| `TestPoolBoundary` | [internal/shop/ops/handler_test.go:33](../../internal/shop/ops/handler_test.go) | TestPoolBoundary 一张审批两次发送也只改一次，另一个过时审批不能覆盖它。 |
| `TestWarmBoundary` | [internal/shop/ops/handler_test.go:104](../../internal/shop/ops/handler_test.go) | TestWarmBoundary 证明最多一百项，并且部分成功后的重试不会重复补已经存在的缓存。 |
| `TestDoctorMounts` | [tools/archcheck/doctor_test.go:17](../../tools/archcheck/doctor_test.go) | TestDoctorMounts 直接调用 archcheck 使用的检查函数，不另写一份较宽松的测试判断。 |
| `TestOpsInternal` | [tools/archcheck/doctor_test.go:52](../../tools/archcheck/doctor_test.go) | TestOpsInternal 运维端口即使只映射到 localhost 也不符合本项目的“仅内部网络”约定。 |
| `TestIntegrationCrashWorker` | [tools/recoverycheck/crash_integration_test.go:97](../../tools/recoverycheck/crash_integration_test.go) | TestIntegrationCrashWorker 只有父测试给出专用标记才工作，普通测试发现它时直接返回。 |
| `TestIntegrationProcessCrash` | [tools/recoverycheck/crash_integration_test.go:196](../../tools/recoverycheck/crash_integration_test.go) | TestIntegrationProcessCrash 每个用例都实际终止进程，恢复时只使用 MySQL 中留下的记录。 |
| `TestIntegrationWriteCrash` | [tools/recoverycheck/crash_integration_test.go:315](../../tools/recoverycheck/crash_integration_test.go) | TestIntegrationWriteCrash 把故障放在“远端成功、本地尚无结果”的间隙，恢复只能先确认。 |

复核差异可运行 `git diff --stat main...phase-07-reliability`。本清单和汇报书自身在收尾时仍会补 CI 链接，最终行数在最后一次文档提交中校准。
