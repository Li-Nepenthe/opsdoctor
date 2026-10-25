# 阶段 4 汇报书：故障注入与恢复

> 2026-10-25 完成。本机连续两轮 selftest 共 22/22 通过；验收源码为 0cb75b3，其后只修正说明文字。阶段 4 完成后等待 Claude 验收和用户学习。

## 一、总览

本阶段把手工破坏环境变成可记录、可恢复、能查询实际症状的命令流程。

chaos 在宿主机读取场景，先保存原配置，再改索引、网络、服务配置或流量。业务服务自己产生变慢、排队或积压，验收器根据 Prometheus 数据判断结果。标准答案留在 eval/scenarios，运行镜像不携带它。阶段 3 的两个遗留项也做了实测：Jaeger 的起始 trace 在二十分钟后仍可查，独立发送预算下的二十分钟健康压测没有消息未确认告警。

| 计划要求 | 当前情况 |
|---|---|
| 发布配置与另一构建版本 | 已完成；正常与另一版本镜像构建、发布与原镜像恢复均通过两轮实测 |
| 三条 Toxiproxy 业务链路 | 已完成，18 容器通过健康检查 |
| list/inject/restore/restore-all/verify/healthy/selftest | 已完成；单元、集成、竞态与两轮完整实测通过 |
| S01–S10、S13 场景与证据 | 已完成；十一份定义、标准答案、证据里程碑和两轮自动验收通过 |
| 逐场景学习与人工排障讲解 | 已完成；逐场景解释、真实开发回放、六组可运行 Go 示例和最终实验结果 |

## 二、新引入的技术与依赖

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| Toxiproxy | ghcr.io/shopify/toxiproxy:2.13.0 | 三条真实 TCP 链路 | 计划书指定，可以改变真实网络传输 | 不在业务代码里 Sleep 或伪造错误 |
| goccy/go-yaml | v1.19.2 | 严格读取场景文件 | 已在依赖树中，提升为直接依赖；拒绝拼错字段 | 不自行编写 YAML 解析器 |
| Go 构建标签 | Go 1.27.1 | 编译 S09 的另一份实现 | 运行时没有业务故障开关 | 不根据秘密环境变量切换代码 |

Jaeger 保留用户批准的 2.20.0 例外，其他基础设施沿用阶段 3 固定版本。没有引入计划外组件或付费服务。

## 三、代码明细

### 1. 包的分工

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| internal/chaos/scenario | 新增 | 加载场景、限制资源和参数 | 场景结构、Load、Decode、动作与配置校验 |
| internal/chaos/action | 新增 | 执行真正的 Docker、SQL、Redis、代理动作 | Runner、基线快照、Execute、Restore |
| internal/chaos/verify | 新增 | 读取实际指标和告警，持续观察健康 | Client、Query、Alerts、Symptom、Healthy |
| internal/chaos/app | 新增 | 装配命令、保存恢复记录、运行整套实验 | Run、Engine、Record、Trial、Selftest |
| internal/shop/app/shopctl | 修改 | 显式按商品范围补库存 | stock restock 子命令 |
| internal/shop/inventoryclient | 修改 | 让只读重试有独立单次预算 | Config.AttemptTimeout，扣减仍不重试 |
| internal/shop/app/orderservice | 修改 | 读入新配置、注册另一版本实现 | 启动配置日志与编译时选中的中间件 |
| internal/shop/order | 修改 | 订单提交后独立等待消息发送 | Create 的收尾预算 |
| internal/shop/events | 修改 | 保留发送错误分类与 writer 诊断 | NewPublisher 的日志依赖 |
| internal/loadgen | 修改 | 复用固定工作者发送有限商品查询突发 | POST /burst、GET /control 的真实批次进度 |
| internal/platform/telemetry | 修改 | 集中登记重叠查库计数 | cache_concurrent_loads_total，仅用于诊断 |

### 2. 文件清单

以下清单来自 main 到当前阶段分支的 Git 差异，已按最终工作区重新生成行数。

| 文件 | 当前行数 | 作用 |
|---|---:|---|
| [.dockerignore](../../.dockerignore) | 11 | 避免答案与恢复记录进入构建上下文 |
| [.env.example](../../.env.example) | 104 | 列出演示配置，不放本机口令 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 226 | 增加另一构建版本和真实代理恢复冒烟 |
| [.gitignore](../../.gitignore) | 15 | 忽略本机恢复记录 |
| [README.md](../../README.md) | 383 | 说明目录职责、运行方法或学习顺序 |
| [cmd/README.md](../../cmd/README.md) | 23 | 说明目录职责、运行方法或学习顺序 |
| [cmd/chaos/main.go](../../cmd/chaos/main.go) | 22 | 把 Ctrl+C 转为取消信号，再调用 chaos 命令装配层 |
| [deploy/README.md](../../deploy/README.md) | 23 | 说明目录职责、运行方法或学习顺序 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 429 | 配置代理链路、内存上限与可发布参数 |
| [deploy/docker/README.md](../../deploy/docker/README.md) | 14 | 说明目录职责、运行方法或学习顺序 |
| [deploy/docker/go.Dockerfile](../../deploy/docker/go.Dockerfile) | 18 | 支持编译指定构建标签和版本号 |
| [deploy/jaeger/README.md](../../deploy/jaeger/README.md) | 14 | 说明目录职责、运行方法或学习顺序 |
| [deploy/jaeger/jaeger.yml](../../deploy/jaeger/jaeger.yml) | 44 | 增加调用链容量及相应内存预算 |
| [deploy/mysql/init/005_chaos_user.sh](../../deploy/mysql/init/005_chaos_user.sh) | 17 | 建立只供实验使用的结构变更账号 |
| [deploy/mysql/init/README.md](../../deploy/mysql/init/README.md) | 20 | 说明目录职责、运行方法或学习顺序 |
| [deploy/prometheus/rules/shop.yml](../../deploy/prometheus/rules/shop.yml) | 76 | 根据可见症状报警，不预先给出故障原因 |
| [deploy/toxiproxy/README.md](../../deploy/toxiproxy/README.md) | 9 | 说明目录职责、运行方法或学习顺序 |
| [deploy/toxiproxy/proxies.json](../../deploy/toxiproxy/proxies.json) | 5 | 定义三条业务代理链路 |
| [docs/decisions.md](../../docs/decisions.md) | 256 | 记录经用户批准的边界和工程取舍 |
| [docs/learn/phase-04-chaos.md](../../docs/learn/phase-04-chaos.md) | 710 | 讲解开发过程、Go 写法和逐场景排障 |
| [docs/notes.md](../../docs/notes.md) | 774 | 保存真实开发和实验过程 |
| [docs/reports/phase-04-chaos.md](../../docs/reports/phase-04-chaos.md) | 384 | 记录本阶段的实现或实测证据 |
| [docs/代码地图.md](../../docs/代码地图.md) | 248 | 记录本阶段的实现或实测证据 |
| [docs/数据台账.md](../../docs/数据台账.md) | 212 | 记录本阶段的实现或实测证据 |
| [eval/README.md](../../eval/README.md) | 9 | 说明目录职责、运行方法或学习顺序 |
| [eval/scenarios/README.md](../../eval/scenarios/README.md) | 23 | 说明目录职责、运行方法或学习顺序 |
| [eval/scenarios/S01.yaml](../../eval/scenarios/S01.yaml) | 20 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S02.yaml](../../eval/scenarios/S02.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S03.yaml](../../eval/scenarios/S03.yaml) | 21 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S04.yaml](../../eval/scenarios/S04.yaml) | 30 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S05.yaml](../../eval/scenarios/S05.yaml) | 22 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S06.yaml](../../eval/scenarios/S06.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S07.yaml](../../eval/scenarios/S07.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S08.yaml](../../eval/scenarios/S08.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S09.yaml](../../eval/scenarios/S09.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S10.yaml](../../eval/scenarios/S10.yaml) | 19 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [eval/scenarios/S13.yaml](../../eval/scenarios/S13.yaml) | 29 | 记录这一场景的原因、动作、症状、恢复与调查证据 |
| [go.mod](../../go.mod) | 80 | 声明场景 YAML 解析直接依赖 |
| [internal/README.md](../../internal/README.md) | 27 | 说明目录职责、运行方法或学习顺序 |
| [internal/chaos/README.md](../../internal/chaos/README.md) | 14 | 说明目录职责、运行方法或学习顺序 |
| [internal/chaos/action/burst.go](../../internal/chaos/action/burst.go) | 99 | 让流量工具发送有限批次的商品查询，并确认真实完成情况 |
| [internal/chaos/action/burst_test.go](../../internal/chaos/action/burst_test.go) | 78 | 检查商品突发动作只接受自己这一批的真实完成结果 |
| [internal/chaos/action/deploy.go](../../internal/chaos/action/deploy.go) | 160 | 保存发布前的配置，并通过 Compose 发布与恢复服务 |
| [internal/chaos/action/doc.go](../../internal/chaos/action/doc.go) | 21 | 负责真正改变实验环境 |
| [internal/chaos/action/http.go](../../internal/chaos/action/http.go) | 82 | 通过本机控制接口改变网络延迟和请求流量 |
| [internal/chaos/action/runner.go](../../internal/chaos/action/runner.go) | 96 | 把已检查的场景动作交给真正的外部系统执行 |
| [internal/chaos/action/runner_test.go](../../internal/chaos/action/runner_test.go) | 141 | 用假的命令执行器与本地 HTTP 服务检查注入和恢复操作 |
| [internal/chaos/action/storage.go](../../internal/chaos/action/storage.go) | 122 | 改动明确允许的索引，以及商品缓存的有效时间 |
| [internal/chaos/app/app.go](../../internal/chaos/app/app.go) | 216 | 把 chaos 命令行接到场景、执行器和自动验收流程 |
| [internal/chaos/app/doc.go](../../internal/chaos/app/doc.go) | 18 | 把一次实验从头到尾组织起来 |
| [internal/chaos/app/engine.go](../../internal/chaos/app/engine.go) | 108 | 组织一次实验，并把尚未恢复的状态留在宿主机 |
| [internal/chaos/app/engine_test.go](../../internal/chaos/app/engine_test.go) | 180 | 用假的外部执行器验证实验失败时的恢复顺序 |
| [internal/chaos/app/selftest.go](../../internal/chaos/app/selftest.go) | 121 | 按健康、注入、观察、恢复、恢复健康的顺序运行整套场景 |
| [internal/chaos/scenario/doc.go](../../internal/chaos/scenario/doc.go) | 19 | 读取“这次实验准备怎样做”的说明文件 |
| [internal/chaos/scenario/load.go](../../internal/chaos/scenario/load.go) | 73 | 加载并检查场景文件，所有检查通过后才允许开始操作 |
| [internal/chaos/scenario/load_test.go](../../internal/chaos/scenario/load_test.go) | 103 | 检查场景定义在执行前能否挡住错误输入 |
| [internal/chaos/scenario/model.go](../../internal/chaos/scenario/model.go) | 57 | 定义一个可重复实验需要保存的输入与判断条件 |
| [internal/chaos/scenario/validate.go](../../internal/chaos/scenario/validate.go) | 134 | 把场景操作限制在计划书规定的服务和资源内 |
| [internal/chaos/verify/doc.go](../../internal/chaos/verify/doc.go) | 18 | 回答两个问题：问题真的出现了吗，现在真的恢复了吗？ |
| [internal/chaos/verify/prometheus.go](../../internal/chaos/verify/prometheus.go) | 118 | 读取 Prometheus 的查询结果和正在触发的告警 |
| [internal/chaos/verify/verify_test.go](../../internal/chaos/verify/verify_test.go) | 106 | 检查监控查询失败、空数据和告警状态的解释 |
| [internal/chaos/verify/wait.go](../../internal/chaos/verify/wait.go) | 132 | 等待症状和告警出现，或等待系统连续一分钟保持健康 |
| [internal/loadgen/app.go](../../internal/loadgen/app.go) | 119 | 启动压测进程，管理控制端口、统计输出和退出 |
| [internal/loadgen/burst.go](../../internal/loadgen/burst.go) | 101 | 把一小批商品查询同时交给已有的工作者 |
| [internal/loadgen/burst_test.go](../../internal/loadgen/burst_test.go) | 126 | 验证商品突发只查询商品，并且始终受固定工作者上限约束 |
| [internal/loadgen/client.go](../../internal/loadgen/client.go) | 102 | 发送真实 HTTP 请求，并按约定重试超时下单 |
| [internal/loadgen/control.go](../../internal/loadgen/control.go) | 113 | 通过本机控制接口调整速率和热点商品 |
| [internal/loadgen/doc.go](../../internal/loadgen/doc.go) | 35 | 是压测工具：按设定的速率持续给订单服务发送真实的 HTTP 请求 |
| [internal/loadgen/generator.go](../../internal/loadgen/generator.go) | 173 | 按指定速率选择请求类型和商品，并交给固定工作者 |
| [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | 201 | 验证压测请求分布、重试身份和真实统计口径 |
| [internal/platform/telemetry/metrics.go](../../internal/platform/telemetry/metrics.go) | 92 | 集中定义本阶段的指标名称、标签和注册方式 |
| [internal/shop/app/orderservice/app.go](../../internal/shop/app/orderservice/app.go) | 177 | 装配订单服务的依赖、路由和退出流程 |
| [internal/shop/app/orderservice/config.go](../../internal/shop/app/orderservice/config.go) | 128 | 读取订单进程的配置，并检查各层时间预算 |
| [internal/shop/app/orderservice/config_test.go](../../internal/shop/app/orderservice/config_test.go) | 89 | 验证订单服务的治理参数和时间预算关系 |
| [internal/shop/app/orderservice/dependencies.go](../../internal/shop/app/orderservice/dependencies.go) | 154 | 准备订单进程需要的连接，并明确由谁关闭它们 |
| [internal/shop/app/orderservice/doc.go](../../internal/shop/app/orderservice/doc.go) | 28 | 负责把订单服务组装起来并运行，从启动到退出的全过程都在这里 |
| [internal/shop/app/orderservice/observations.go](../../internal/shop/app/orderservice/observations.go) | 108 | 在订单服务的接口外面加计数，不把 Prometheus 类型传进业务流程 |
| [internal/shop/app/orderservice/observations_test.go](../../internal/shop/app/orderservice/observations_test.go) | 78 | 检查同键并发回源只记录真实重叠的数据库查询 |
| [internal/shop/app/orderservice/response_async.go](../../internal/shop/app/orderservice/response_async.go) | 30 | 将响应结果交给后台任务整理 |
| [internal/shop/app/orderservice/response_direct.go](../../internal/shop/app/orderservice/response_direct.go) | 20 | 定义当前版本的响应收尾方式 |
| [internal/shop/app/orderservice/servers.go](../../internal/shop/app/orderservice/servers.go) | 72 | 用已经准备好的依赖装配业务，并创建两个 HTTP 入口 |
| [internal/shop/app/shopctl/app.go](../../internal/shop/app/shopctl/app.go) | 87 | 分派宿主机调试命令并组织造数流程 |
| [internal/shop/app/shopctl/doc.go](../../internal/shop/app/shopctl/doc.go) | 23 | 是 shopctl 命令行工具的实现，用来在本机手动操作靶场 |
| [internal/shop/app/shopctl/restock_test.go](../../internal/shop/app/shopctl/restock_test.go) | 67 | 验证批量补库存遇到结果不确定时会立即停止 |
| [internal/shop/app/shopctl/stock_client.go](../../internal/shop/app/shopctl/stock_client.go) | 118 | 解析库存命令并通过正式 gRPC 契约调用服务 |
| [internal/shop/app/shopctl/stock_client_test.go](../../internal/shop/app/shopctl/stock_client_test.go) | 44 | 验证命令行参数不会被误解或截断 |
| [internal/shop/events/doc.go](../../internal/shop/events/doc.go) | 32 | 负责“订单已创建”这条 Kafka 消息：消息长什么样、怎么发送、怎么消费 |
| [internal/shop/events/publisher.go](../../internal/shop/events/publisher.go) | 87 | 把订单创建消息发送到 Kafka |
| [internal/shop/events/publisher_test.go](../../internal/shop/events/publisher_test.go) | 108 | 复现“调用者结束等待，Kafka 批次随后才确认”的时间顺序 |
| [internal/shop/inventoryclient/attempt_test.go](../../internal/shop/inventoryclient/attempt_test.go) | 118 | 验证单次查询时限与整次调用时限各自限制什么 |
| [internal/shop/inventoryclient/doc.go](../../internal/shop/inventoryclient/doc.go) | 29 | 是订单服务调用库存服务的客户端，在 gRPC 调用外面加了一层保护 |
| [internal/shop/inventoryclient/inventory_client.go](../../internal/shop/inventoryclient/inventory_client.go) | 199 | 给库存 gRPC 调用加上时间限制和故障保护 |
| [internal/shop/order/service.go](../../internal/shop/order/service.go) | 224 | 按顺序完成下单，并在失败时判断能否安全重试 |
| [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | 356 | 逐个模拟下单步骤失败，核对库存补偿与幂等记录 |
| [进度.md](../../进度.md) | 163 | 记录本阶段的实现或实测证据 |
| [项目计划书.md](../../项目计划书.md) | 2062 | 记录本阶段的实现或实测证据 |

### 3. 类型与关键函数

清单由 commentcheck -list 生成后逐项补充调用方，包含新增的导出类型、函数及关键内部流程。测试单独列在第四节。

| 名称 | 位置 | 作用 | 被谁调用 |
|---|---|---|---|
| main | cmd/chaos/main.go:15:1 | main 将 Ctrl+C 变成取消信号，selftest 收到后先恢复当前场景再退出 | Go 程序入口 |
| *Runner.burst | internal/chaos/action/burst.go:25:1 | burst 让每批内部同时出发，再按批次大小留出间距，使平均速率不超过 320 次/秒 | Runner.Execute |
| *Runner.waitBurst | internal/chaos/action/burst.go:53:1 | waitBurst 不会自动重发失败批次，否则一次缓存失效可能被重复流量改变成另一种实验 | Runner.burst |
| pause | internal/chaos/action/burst.go:87:1 | pause 使用可取消的定时器，让停止实验不必等到整个批次间隔结束 | Runner.burst、waitBurst |
| Baseline | internal/chaos/action/deploy.go:26:6 | Baseline 是本机恢复记录的一部分，不含凭据 | Engine 与 Runner 的动作流程 |
| Deployment | internal/chaos/action/deploy.go:33:6 | Deployment 用镜像内容 ID 和公开环境重建原服务 | Engine 与 Runner 的动作流程 |
| *Runner.Capture | internal/chaos/action/deploy.go:40:1 | Capture 在注入之前执行 | Engine.Inject |
| *Runner.deploy | internal/chaos/action/deploy.go:103:1 | deploy 用真实构建版本区分配置发布 | Runner.Execute、Restore |
| *Runner.Restore | internal/chaos/action/deploy.go:144:1 | Restore 对发布和流量使用注入前快照，其余动作仍按场景声明执行 | Engine.Restore |
| *Runner.request | internal/chaos/action/http.go:28:1 | request 不自动重试写控制请求；超时后交给恢复路径处理结果不确定的情况 | Runner.Capture、latency、load |
| *Runner.latency | internal/chaos/action/http.go:63:1 | latency 使用固定名称便于重复恢复 | Runner.Execute |
| *Runner.load | internal/chaos/action/http.go:73:1 | load 只发送明确指定的字段，省略字段不会把现有设置重置为零 | Runner.Execute |
| Runner | internal/chaos/action/runner.go:31:6 | Runner 只保存配置，不长时间占用数据库连接 | Engine 与 Runner 的动作流程 |
| *Runner.Execute | internal/chaos/action/runner.go:42:1 | Execute 再次检查参数，防止调用方绕过 YAML 加载器后执行任意操作 | Engine.Inject |
| *Runner.Command | internal/chaos/action/runner.go:71:1 | Command 使用参数数组，YAML 里的值不会被 PowerShell 或 bash 当成代码 | Runner.RunCommand |
| *Runner.compose | internal/chaos/action/runner.go:93:1 | compose 固定配置文件与项目目录，只接受内部代码构造的子命令 | Runner.Capture、Execute、deploy |
| *Runner.index | internal/chaos/action/storage.go:28:1 | index 不接受任意 SQL | Runner.Execute |
| expectedIndex | internal/chaos/action/storage.go:63:1 | expectedIndex 不只检查名字，还核对列顺序和唯一性，避免把同名但结构不同的索引误删 | Runner.index |
| *Runner.cache | internal/chaos/action/storage.go:86:1 | cache 使用 pipeline 一次发送多个独立命令，减少宿主机逐次往返造成的到期时间差 | Runner.Execute |
| Run | internal/chaos/app/app.go:33:1 | Run 不隐式启动流量 | cmd/chaos.main |
| newEngine | internal/chaos/app/app.go:77:1 | newEngine 只读取密码，不输出它 | app.Run |
| *Engine.general | internal/chaos/app/app.go:97:1 | general 处理无需场景编号的命令 | app.Run |
| *Engine.operate | internal/chaos/app/app.go:121:1 | operate 用已有记录匹配 verify 和 restore，防止把另一场景的效果误当成当前场景 | app.Run |
| overrides | internal/chaos/app/app.go:167:1 | overrides 只覆盖对应动作已有的参数；没有这种动作的场景会拒绝无效的覆盖选项 | app.operate |
| Executor | internal/chaos/app/engine.go:29:6 | Executor 隔离真正的 Docker、数据库和网络操作，流程测试无需启动容器 | app.Run、Engine 流程与单元测试 |
| Observer | internal/chaos/app/engine.go:36:6 | Observer 只看监控结果，不借用注入器内部状态判断故障是否出现 | app.Run、Engine 流程与单元测试 |
| Record | internal/chaos/app/engine.go:42:6 | Record 保留当时的完整场景，恢复时不依赖后来可能已编辑过的 YAML | app.Run、Engine 流程与单元测试 |
| Engine | internal/chaos/app/engine.go:49:6 | Engine 用一个独占状态文件防止两个场景叠加，否则难以知道该恢复哪份基线 | app.Run、Engine 流程与单元测试 |
| *Engine.Inject | internal/chaos/app/engine.go:56:1 | Inject 用 O_EXCL 独占创建记录 | app.operate、Engine.Trial |
| *Engine.Read | internal/chaos/app/engine.go:83:1 | Read 在恢复或单独 verify 时使用原记录，避免错把另一场景的状态拿来验收 | app.operate、app.general、Engine.Trial |
| *Engine.Restore | internal/chaos/app/engine.go:96:1 | Restore 尝试全部恢复动作；一个动作失败不能阻止另一条网络链路恢复 | app.operate、app.general、Engine.Trial |
| Row | internal/chaos/app/selftest.go:31:6 | Row 中的时间都来自实际时钟，尚未执行的步骤保持零值并在 Problem 中说明 | app.Run、Engine 流程与单元测试 |
| *Engine.Trial | internal/chaos/app/selftest.go:43:1 | Trial 先确认健康，再开始计时；原本已经有故障的系统不能用来判断本次注入的效果 | Engine.Selftest |
| *Engine.Selftest | internal/chaos/app/selftest.go:93:1 | Selftest 不在场景间自动补库存，操作者须在实验前显式准备，避免把数据变化藏进流程 | app.general |
| Load | internal/chaos/scenario/load.go:26:1 | Load 只读取当前阶段明确支持的文件，不把目录中新放入的任意文件自动变成可执行场景 | app.Run |
| Decode | internal/chaos/scenario/load.go:47:1 | Decode 先检查内容，再逐个检查动作 | scenario.Load |
| Scenario | internal/chaos/scenario/model.go:16:6 | Scenario 把实验操作与验收条件放在一起，避免两者各自修改后不再对应 | 场景加载、执行器和 app 流程 |
| Step | internal/chaos/scenario/model.go:27:6 | Step 只允许已经实现的动作 | 场景加载、执行器和 app 流程 |
| Symptom | internal/chaos/scenario/model.go:46:6 | Symptom 的表达式采用 PromQL，即 Prometheus 用来筛选和计算指标的查询语言 | Engine.Trial、app.operate |
| Truth | internal/chaos/scenario/model.go:53:6 | Truth 保存解释，不参与业务行为或告警规则 | 场景加载、执行器和 app 流程 |
| ValidateStep | internal/chaos/scenario/validate.go:21:1 | ValidateStep 先限制动作类型，再检查必要参数 | scenario.Decode、Runner.Execute、Restore、app.overrides |
| ValidateEnvironment | internal/chaos/scenario/validate.go:81:1 | ValidateEnvironment 不接受 DSN、口令或任意环境变量，只允许实验所需的公开配置 | ValidateStep、Runner.Capture |
| validateInteger | internal/chaos/scenario/validate.go:116:1 | validateInteger 提前使用服务端同样的单项边界，避免明知无效还去重建容器 | ValidateEnvironment |
| Client | internal/chaos/verify/prometheus.go:29:6 | Client 的间隔和持续时间集中保存，单元测试可缩短时间，不必真的等一分钟 | app.newEngine 与验收流程 |
| New | internal/chaos/verify/prometheus.go:37:1 | New 使用五秒轮询，能在十秒采集周期内及时看到新样本，又不会不断空转 | app.newEngine 与验收流程 |
| *Client.get | internal/chaos/verify/prometheus.go:42:1 | get 限制响应大小，并要求 HTTP 成功；各调用方再检查 Prometheus JSON 中的业务状态 | Client.Query、Alerts |
| *Client.Query | internal/chaos/verify/prometheus.go:59:1 | Query 用普通比较后的样本判断条件；没有样本表示条件尚未出现，不是后端错误 | Client.Symptom、healthSample |
| *Client.Alerts | internal/chaos/verify/prometheus.go:94:1 | Alerts 排除 pending，它表示条件还在观察中，尚未真的触发告警 | Client.Symptom、healthSample |
| Result | internal/chaos/verify/wait.go:25:6 | Result 即使失败也保留等待时间，不能只在成功时记录实验数据 | app.newEngine 与验收流程 |
| *Client.Symptom | internal/chaos/verify/wait.go:33:1 | Symptom 的 ctx 由注入开始时间计算截止，部署耗时也计入三分钟，不另送三分钟 | Engine.Trial、app.operate |
| healthyQueries | internal/chaos/verify/wait.go:63:1 | healthyQueries 不把无流量当健康；三个业务端点和三个依赖采集器都必须可用 | Client.healthSample |
| *Client.healthSample | internal/chaos/verify/wait.go:77:1 | healthSample 任一查询失败都会使本次采样失败，空数据不会掩盖监控断开 | Client.Healthy |
| *Client.Healthy | internal/chaos/verify/wait.go:99:1 | Healthy 每次坏样本都清空连续计时；不能把分散的健康时间累加成一分钟 | Engine.Trial、app.general |
| wait | internal/chaos/verify/wait.go:123:1 | wait 允许 Ctrl+C 及时结束等待，之后 app 仍可用独立预算做恢复 | Client.Symptom、Healthy |
| Run | internal/loadgen/app.go:42:1 | Run 把启动准备排除在统计之外，实际完成数从工作者开始运行时累计 | cmd/loadgen.main |
| burstReport | internal/loadgen/burst.go:23:6 | burstReport 把接收、完成和未发出分开，不能把 202 Accepted 当成业务成功 | 控制接口与完成统计 |
| burstBatch | internal/loadgen/burst.go:34:6 | burstBatch 只装有限的任务信息 | 调度者与固定工作者 |
| burstState | internal/loadgen/burst.go:42:6 | burstState 用一把锁保护进度；队列最多容纳一批，运行中的批次不能叠加 | generator、controlRouter |
| *burstState.submit | internal/loadgen/burst.go:51:1 | submit 先检查边界，再占用唯一的批次位置，避免并发控制请求突破上限 | POST /burst |
| *burstState.finish | internal/loadgen/burst.go:66:1 | finish 在请求结束或调度被取消时更新进度 | generator.run、dispatchBurst |
| *burstState.snapshot | internal/loadgen/burst.go:84:1 | snapshot 返回副本，HTTP 编码时不占用工作者更新统计所需的锁 | GET /control |
| *generator.dispatchBurst | internal/loadgen/burst.go:91:1 | dispatchBurst 暂停普通调度，复用同一组工作者，因而突发与普通流量相加也不会超过总上限 | generator.schedule |
| *generator.execute | internal/loadgen/client.go:29:1 | execute 把一次逻辑下单限制在两次 HTTP 尝试，第二次沿用同一个 job 的 key | generator.run 的固定工作者 |
| controlRouter | internal/loadgen/control.go:71:1 | controlRouter 只在专用管理端口安装，Compose 把它映射到宿主机回环地址 | loadgen.Run |
| newGenerator | internal/loadgen/generator.go:73:1 | newGenerator 用 v=100 分散最热的一小组商品，仍让它们远比长尾商品常见 | loadgen.Run |
| *generator.run | internal/loadgen/generator.go:100:1 | run 停止时先停调度，再让正在发送的请求结束 | loadgen.Run |
| *generator.schedule | internal/loadgen/generator.go:137:1 | schedule 每五毫秒积累“应发送多少次”的额度，实际完成 QPS 由 statistics 独立计算 | generator.run |
| NewMetrics | internal/platform/telemetry/metrics.go:45:1 | NewMetrics 的名称都由本包固定，重复注册说明代码有错，所以用 MustRegister 尽早暴露 | 各服务装配层与单元测试 |
| loadConfig | internal/shop/app/orderservice/config.go:56:1 | loadConfig 用局部读取函数收集错误，用户一次就能看到多个配置问题 | 对应 app.Run |
| processingBudget | internal/shop/app/orderservice/config.go:111:1 | processingBudget 给幂等占位和优雅停机使用同一个上界，避免发送还在执行时就把请求判为待核对 | orderservice.loadConfig、Run |
| observedProducts | internal/shop/app/orderservice/observations.go:48:6 | observedProducts 放在 singleflight 后面的仓储接口上，合并后的查询只计一次回源 | orderservice.services |
| *observedProducts.Get | internal/shop/app/orderservice/observations.go:57:1 | Get 在调用仓储前记录一次回源尝试，包括后面的借连接和 SQL 执行 | product.Service.load |
| responseRecorder | internal/shop/app/orderservice/response_async.go:19:1 | responseRecorder 等业务处理完成后再取状态，避免把默认的 200 当成最终结果 | orderservice.Run |
| deliverResponse | internal/shop/app/orderservice/response_async.go:28:1 | deliverResponse 使用无缓冲通道交接结果；发送方要等接收方准备好才能结束 | responseRecorder 后台任务 |
| responseRecorder | internal/shop/app/orderservice/response_direct.go:18:1 | responseRecorder 把请求交给后续处理器，响应内容和状态由业务决定 | orderservice.Run |
| parseStock | internal/shop/app/shopctl/stock_client.go:39:1 | parseStock 在建立客户端前拒绝错误参数，防止“敲错命令却已经改了数据” | shopctl.runStock |
| runStock | internal/shop/app/shopctl/stock_client.go:65:1 | runStock 创建一个短生命周期客户端 | shopctl.Run |
| restock | internal/shop/app/shopctl/stock_client.go:105:1 | restock 在实验前逐个补库存，始终通过库存服务的校验和原子更新，不直接操作表 | shopctl.runStock |
| NewPublisher | internal/shop/events/publisher.go:44:1 | NewPublisher 禁用自动建主题 | orderservice.Run |
| *Client.GetStock | internal/shop/inventoryclient/inventory_client.go:90:1 | GetStock 的所有尝试共享一个截止时间，不能每次重试都重新获得完整预算 | product.Service.Get、客户端测试 |
| *Client.queryAttempt | internal/shop/inventoryclient/inventory_client.go:117:1 | queryAttempt 给一次查询分配时间 | inventoryclient.Client.GetStock |
| NewService | internal/shop/order/service.go:72:1 | NewService 由 app 统一传入依赖，cleanupTimeout 必须大于库存调用超时 | 对应 app 的 Run 与测试 |
| *Service.Create | internal/shop/order/service.go:77:1 | Create 遇到结果不明就停止自动处理，不能把“没有收到成功答复”当成“什么都没发生” | order.HTTPHandler 的下单处理 |
| publishErrorKind | internal/shop/order/service.go:216:1 | publishErrorKind 记录可用于排查的错误分类，而不把第三方错误中的连接地址或凭据直接写进日志 | order.Service.Create |

### 4. 数据库变更

没有新增业务表或修改数据模型。初始化脚本 005_chaos_user.sh 新建实验账号，只授予 shop.stock、shop.orders 的 SELECT、ALTER、INDEX。它只能由宿主机实验工具使用，不交给业务服务和以后的医生。索引实验删除后按原定义恢复；补库存通过已有 Release RPC，未直接修改库存表。

### 5. 接口变更

没有新增业务 HTTP 或 gRPC 方法。新增宿主机 chaos 命令和 shopctl stock restock 子命令。Toxiproxy 管理端口 8474 只绑定 127.0.0.1；三个代理端口仅在 Compose 网络中使用。

loadgen 的本机控制接口新增 `POST /burst`，接收商品编号和有限请求数。返回 202 只表示接收成功，`GET /control` 的 burst 字段另行报告完成、失败、未发出数量。它与普通流量共用固定工作者，不另外扩充并发。新增 `cache_concurrent_loads_total` 记录同键数据库查询重叠，只作诊断证据，不进入告警规则。

### 6. 配置变更

| 变量 | 含义 | 默认值 |
|---|---|---|
| ORDER_PUBLISH_TIMEOUT | 已提交订单等待发送确认的独立预算 | 5s，允许 1ms–10s |
| INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT | GetStock 每次尝试最多等多久 | 0s，沿用总预算；不能超过总预算 |
| INVENTORY_GETSTOCK_MAX_RETRIES | 只读查询最多重试几次 | 仍为 2，上限扩大到 10 |
| CHAOS_MYSQL_PASSWORD | 初始化实验账号的密码 | 无真实默认值，example 只有假值 |
| CHAOS_MYSQL_DSN | 宿主机实验工具连接串 | 索引动作必须提供 |
| CHAOS_REDIS_ADDR | 实验工具访问商品缓存 | 127.0.0.1:6379 |
| TOXIPROXY_URL | 本机代理控制地址 | http://127.0.0.1:8474 |
| LOADGEN_CONTROL_URL | 本机流量控制地址 | http://127.0.0.1:6070 |
| PROMETHEUS_URL | 实验查询指标与告警 | http://127.0.0.1:9090 |
| ORDER_IMAGE / INVENTORY_IMAGE | 本次 Compose 发布的镜像 | 对应服务的 20261024.3 标签 |
| ORDER_GO_TAGS | 构建另一份订单实现 | 空；显式构建 S09 时为 leakbug |

### 7. 部署变更

共 18 个容器。新增 Toxiproxy，三条链路为订单到库存、库存到 MySQL、订单到 Redis。Jaeger 容量改为 30000 条，内存上限 1024 MiB。Toxiproxy 上限 128 MiB，MySQL 仍为 1536 MiB。Redis 长实验达到原写入上限后，调整为 512 MiB 写入预算、768 MiB 容器上限，保留 noeviction、AOF 和原幂等期限；重启前后都保留 299587 个幂等键。所有宿主机端口继续绑定本机。增加 7 条只描述症状的告警，共 12 条。

## 四、测试

### 1. 新增或补充的测试

| 测试 | 位置 | 验证内容 |
|---|---|---|
| TestBurstResult | internal/chaos/action/burst_test.go:24:1 | TestBurstResult 避免把上一批成功，或这一批的部分成功，误写成整个注入动作已完成 |
| TestBurstRequest | internal/chaos/action/burst_test.go:50:1 | TestBurstRequest 使用一次真实本机 HTTP 往返，检查控制契约和接收、完成两个步骤 |
| TestDeploymentRestore | internal/chaos/action/runner_test.go:28:1 | TestDeploymentRestore 捕获命令参数，不真的重建容器 |
| TestCapturePinsOriginalImage | internal/chaos/action/runner_test.go:51:1 | TestCapturePinsOriginalImage 模拟标签稍后会被覆盖，检查快照阶段已保留原内容 |
| TestRestoreRejectsChangedImage | internal/chaos/action/runner_test.go:81:1 | TestRestoreRejectsChangedImage 不允许标签同名就认定程序相同 |
| TestToxicLifecycle | internal/chaos/action/runner_test.go:95:1 | TestToxicLifecycle 真实走 JSON 编解码，检查规则作用方向和删除路径 |
| TestInvalidActionNeverRuns | internal/chaos/action/runner_test.go:133:1 | TestInvalidActionNeverRuns 验证运行器本身也做校验，而不只相信 YAML 加载器 |
| TestBurstOverridesRejected | internal/chaos/app/engine_test.go:35:1 | TestBurstOverridesRejected 保护场景的因果边界，不能通过命令行让暂停动作失效或把下单改到热点 |
| TestSelftestKeepsUnrunRows | internal/chaos/app/engine_test.go:57:1 | TestSelftestKeepsUnrunRows 防止缩短的结果表把后续没有执行的场景藏起来 |
| TestTrialUsesScenarioBudget | internal/chaos/app/engine_test.go:114:1 | TestTrialUsesScenarioBudget 用父预算兜底，避免错误实现让测试真的等三分钟 |
| TestTrialRestoresAfterCancellation | internal/chaos/app/engine_test.go:129:1 | TestTrialRestoresAfterCancellation 确认用户中断不会让已经生效的外部动作无人恢复 |
| TestRestoreContinuesAndRetainsRecord | internal/chaos/app/engine_test.go:144:1 | TestRestoreContinuesAndRetainsRecord 一个恢复失败不能挡住后续动作，也不能删除重试依据 |
| TestInjectDoesNotOverwrite | internal/chaos/app/engine_test.go:166:1 | TestInjectDoesNotOverwrite 第二次注入必须失败，原场景和起始时间都不能改变 |
| TestRepositoryScenarios | internal/chaos/scenario/load_test.go:23:1 | TestRepositoryScenarios 直接检查交付文件，避免测试样本正确而真实 YAML 已经写错 |
| TestDecodeRejectsInvalid | internal/chaos/scenario/load_test.go:34:1 | TestDecodeRejectsInvalid 逐项修改一份有效场景，确认坏输入在外部操作之前被拒绝 |
| TestStepBoundary | internal/chaos/scenario/load_test.go:59:1 | TestStepBoundary 即使直接调用执行器，也只能使用这些受限动作 |
| TestBurstBoundary | internal/chaos/scenario/load_test.go:81:1 | TestBurstBoundary 同时限制总批次和每批请求数，不允许用合法商品编号绕过资源上限 |
| TestQueryPresence | internal/chaos/verify/verify_test.go:27:1 | TestQueryPresence 不用网络时钟假设，逐行给出后端可能返回的内容 |
| TestSymptomRequiresRelatedFiringAlert | internal/chaos/verify/verify_test.go:51:1 | TestSymptomRequiresRelatedFiringAlert 先返回 pending，再返回不相关告警，最后才允许成功 |
| TestHealthyRequiresContinuousSamples | internal/chaos/verify/verify_test.go:81:1 | TestHealthyRequiresContinuousSamples 在第一次健康后制造一个坏样本，确认不能立即通过 |
| TestProductBurst | internal/loadgen/burst_test.go:26:1 | TestProductBurst 保留普通工作者的总上限 |
| TestBurstValidation | internal/loadgen/burst_test.go:79:1 | TestBurstValidation 把非法输入挡在进入队列之前，避免靠工作者内部再丢掉超出的请求 |
| TestBurstCancellation | internal/loadgen/burst_test.go:101:1 | TestBurstCancellation 只让一个工作者接到任务，然后取消；其余任务必须标成未发出，不能伪装成成功 |
| TestConfigBudgets | internal/shop/app/orderservice/config_test.go:21:1 | TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动 |
| TestInventoryIdleConfig | internal/shop/app/orderservice/config_test.go:62:1 | TestInventoryIdleConfig 保留正常的 30 分钟默认值，只在实验时允许缩短，不把修复变成关闭空闲回收 |
| TestConcurrentProductLoads | internal/shop/app/orderservice/observations_test.go:34:1 | TestConcurrentProductLoads 也让一个查询返回错误，证明失败不会遗留“仍在途”的商品 |
| TestRestockStopsOnUncertainty | internal/shop/app/shopctl/restock_test.go:47:1 | TestRestockStopsOnUncertainty 保证操作者能够知道哪部分已确认，哪一件还需要查询 |
| TestRestockRange | internal/shop/app/shopctl/restock_test.go:60:1 | TestRestockRange 证明批量参数没有让命令越过操作者指定的最后一个商品 |
| TestParseStock | internal/shop/app/shopctl/stock_client_test.go:17:1 | TestParseStock 在无网络环境检查 CLI 边界，尤其防止 int64 到 int32 的静默截断 |
| TestPublisherCancellationDoesNotCancelBatch | internal/shop/events/publisher_test.go:59:1 | TestPublisherCancellationDoesNotCancelBatch 证明未收到确认时仍需告警，不能因为消息以后可能送达就直接记成功 |
| TestAttemptTimeout | internal/shop/inventoryclient/attempt_test.go:45:1 | TestAttemptTimeout 用真实计时器走取消路径，同时检查每次截止都没有越过父预算 |
| TestAttemptConfiguration | internal/shop/inventoryclient/attempt_test.go:76:1 | TestAttemptConfiguration 直接调用构造函数，防止绕过 app 后带着坏配置启动 |
| TestAttemptBudgetNeverRetriesDeduct | internal/shop/inventoryclient/attempt_test.go:102:1 | TestAttemptBudgetNeverRetriesDeduct 覆盖全部允许的重试次数，确认写操作没有进入查询重试器 |
| TestPublishSurvivesCancellation | internal/shop/order/service_test.go:110:1 | TestPublishSurvivesCancellation 模拟数据库刚提交，客户端就断开连接的时刻 |
| TestCreateFailureBoundaries | internal/shop/order/service_test.go:216:1 | TestCreateFailureBoundaries 检查失败之后系统留下的状态，而不只检查函数返回值 |
| TestConcurrentIdempotency | internal/shop/order/service_test.go:270:1 | TestConcurrentIdempotency 二十个相同请求只能创建一笔订单，其他请求得到回放或处理中 |
| TestCompensationSurvivesCancellation | internal/shop/order/service_test.go:298:1 | TestCompensationSurvivesCancellation 请求取消不应阻止已经扣下的库存被归还 |
| TestValidateCreate | internal/shop/order/service_test.go:320:1 | TestValidateCreate 检查整数上界和键格式，所有拒绝都发生在扣库存之前 |
| TestPublishHasIndependentBudget | internal/shop/order/service_test.go:343:1 | TestPublishHasIndependentBudget 保证调整补偿预算不会顺带缩短 Kafka 的确认等待 |

### 2. 已完成的检查

两轮 selftest 使用 0cb75b3，其后六处纯注释修正也已重新通过 gofmt、go vet、staticcheck、commentcheck、archcheck。commentcheck 检查 179 份手写 Go 文件。配置六个专用测试库及 Redis 测试地址后，完整 `go test -race ./... -count=1 -timeout=180s` 通过：125 个顶层测试，其中 16 个 Integration，0 失败、0 跳过。两个构建版本均已从同一提交成功构建。Prometheus 配置与 12 条规则通过 promtool。第一次完整测试脚本误填两个专用库名，被测试在连接前拒绝；修正后通过，原错误输出保留在 notes 所列文件中。

## 五、工程验收结果

| 验收项 | 当前结论 | 证据 |
|---|---|---|
| selftest 连续两遍全部通过 | 通过 | 11/11 + 11/11；两轮完整表保留在 [notes](../notes.md#phase-04) 和台账 P4-18、P4-19；中间未改配置、补库存或清键 |
| 每场景三分钟内有症状与告警 | 通过 | 两轮最长 111.032 秒，包含注入动作；需要症状与相关 firing 告警同时成立 |
| 每场景恢复后三分钟内健康 | 通过 | 两轮最长 141.182 秒，包含恢复动作和连续一分钟健康观察 |

已追加数据台账 **P4-1 至 P4-24**，包含三轮未通过的完整表、S04 两种并发规模的对照、正式容器对照、基线配置错误、Redis 容量问题、连续两轮成功、S09 堆栈、S13 实际放大、内存快照和最终本机检查。两轮 S04 只触发 CacheLoadsHigh，没有 RowLockWaiting；正式开启/关闭对照中两组行锁增量均为零。

2026-10-25 两轮验收结束后，在暂停流量前于 2026-10-25T09:10:35.809618+00:00 记录 docker stats。按显示值换算相加约 3750.340 MiB（3.66 GiB），18 个容器配置上限共 6976 MiB（6.8125 GiB）。健康基线目标为 200 QPS，此时 Prometheus 一分钟 HTTP 速率为 220.361/s；配置目标和观测值分别记录。这是单次快照，不是峰值，也不包含整个 WSL/Windows 的开销。MySQL 约 1.165/1.5 GiB、Jaeger 380.4/1024 MiB、Redis 238.2/768 MiB。

| 容器 | 实际内存 | 上限 |
|---|---:|---:|
| alertmanager | 30.67MiB | 64MiB |
| alertsink | 19.59MiB | 64MiB |
| alloy | 169.9MiB | 256MiB |
| grafana | 146.8MiB | 256MiB |
| inventory-service | 15.47MiB | 128MiB |
| jaeger | 380.4MiB | 1GiB |
| kafka | 827.9MiB | 1GiB |
| kafka-exporter | 23MiB | 64MiB |
| loadgen | 17.33MiB | 128MiB |
| loki | 442MiB | 512MiB |
| mysqld-exporter | 16.28MiB | 64MiB |
| order-service | 19.84MiB | 256MiB |
| order-worker | 16.47MiB | 128MiB |
| prometheus | 154.5MiB | 512MiB |
| redis | 238.2MiB | 768MiB |
| redis-exporter | 17.89MiB | 64MiB |
| shop-mysql | 1.165GiB | 1.5GiB |
| toxiproxy | 21.14MiB | 128MiB |

快照时所有容器正常，三条代理均无延迟规则，消费积压为 0、firing 告警为空。取完快照后已暂停压测，保留容器与原数据卷供学习。

## 六、偏离计划之处

用户批准为 S13 增加只读调用的单次预算，重试上限为 10，总预算不放宽，Deduct 仍只调用一次。用户批准显式补库存命令，只在实验前使用。S04 原热点流量产生了行锁干扰，经用户批准改为有并发上限的商品查询突发，同键重叠计数只作诊断证据，告警仍按实际回源速率。决定及理由见 [S13 单次预算](../decisions.md#phase-04-attempt-timeout)、[S04 商品突发](../decisions.md#phase-04-s04) 和[显式补库存](../decisions.md#phase-04-foundation)。三分钟验收时限和连续两轮要求没有放宽。

## 七、已知问题与遗留事项

1. 历史 53 条消息未确认告警缺少底层错误，不能逐条还原最初原因。新日志会保留错误分类和等待耗时；五秒预算不是可靠投递保证。
2. 库存超时可能已经扣减。待核对占位继续保留。根治需要 Deduct 携带请求 ID 去重，本阶段不实现。
3. Docker 引擎曾在长实验中途退出。中断轮保留，原因未定。恢复记录保存在宿主机；引擎不可用时，程序也不能保证立刻恢复容器。
4. MySQL exporter 的锁等待指标是全实例统计，专用测试库也会影响它。故障实验期间不并行跑数据库集成测试。
5. 首轮 S03 暴露了只记录镜像 ID 的恢复缺陷，已保留原镜像标签并核对内容 ID；回归测试与最终两轮恢复均通过。原失败表保留。
6. 所有性能和保留时长数字来自本机 Docker，不代表生产容量。
7. 当前缓存告警按 200 QPS、10000 个活跃商品、64 个工作者校准。误恢复默认五十万活跃商品时已出现正常回源触发告警，失败保留在 P4-16；扩大工作集或请求速率必须重新校准，不能宣称这些静态门槛适用于所有负载。
8. 商品回源只是一次主键查询。正式对照虽多出大量重复查询，P99 仍落在 100ms 以下的桶；回源更昂贵时的影响不能用本机数字代替。
9. Redis 在约三十万个未过期幂等键时耗尽了原 128 MiB 写入预算。已扩容并保留原记录，但有限容量不等于能持续承载一天的压测；需要按订单速率和保存时长规划容量，不能靠淘汰凭据维持下单。

## 八、注释自查结论

三层说明已逐项自查：分组目录有 README，doc.go 列全包内文件；文件说明写明用途、主要内容与阅读建议；函数和分支解释判断理由。commentcheck 对 179 份手写 Go 文件检查通过，文件地图已重新生成。

人工复查修正了几处容易误导初学者的解释：恢复镜像要先保留引用再核对内容；HTTP 状态与业务状态分开；SQL span 从取得连接后开始；Kafka 调用方停止等待后，后台批次仍可能继续；仓储调用重叠可能包含借连接等待，不一定已经同时执行 SQL。参数字段和测试用例也写清了边界。六组 Go 教学示例已实际执行并通过 gofmt，涵盖预算、通道、独占文件、命令参数、构建标签与固定工作者一起出发。

## 九、学习指引

阅读 [阶段 4 学习材料](../learn/phase-04-chaos.md)。先理解实验怎样开始和结束，再看场景文件、动作执行器、监控判断，最后逐个练习人工排障。建议约 12 小时，可拆成多次学习。完整结果和已知限制均已写入材料，可按阅读顺序逐行学习。

## 十、下一阶段预告

阶段 5 才实现医生的只读数据源与受限工具。现在没有开始医生开发，也不需要大模型 API Key。阶段 4 完成后等待 Claude 验收和用户学习，再由用户决定下一步。
