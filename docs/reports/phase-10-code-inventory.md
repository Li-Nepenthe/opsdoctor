# 阶段 10 文件与符号清单

消费者修复代码为 `364a889`，最终运行及日志隔离代码为 `c246734`；目录、文档及证据随后归档。本表覆盖本阶段相对 main/f851460 的文件变化；用户保留的两处修改不属于本阶段。最终以 `git diff --stat main...phase-10-showcase` 和本清单互相核对。

## 文件

| 文件 | 当前行数 / 字节 | 作用 |
|---|---:|---|
| [.env.example](../../.env.example) | 175 行 | 补充Compose内置项目名的演示说明，所有凭据仍为假值 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 421 行 | 固定runner完整验收；首次计时、五独立新卷及消费和同trace三服务日志、失败日志、仅stop清理 |
| [README.md](../../README.md) | 216 行 | 五分钟展示入口、双shell快速开始、三分钟Mock脚本、冻结成绩与权限限制 |
| [deploy/alloy/README.md](../../deploy/alloy/README.md) | 13 行 | 说明实际项目名注入及三服务日志验收 |
| [deploy/alloy/config.alloy](../../deploy/alloy/config.alloy) | 47 行 | 按实际Compose项目名过滤Docker日志，避免采集其他项目 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 522 行 | 给Alloy注入最终项目名，并修正过期的阶段数量注释 |
| [docs/README.md](../../docs/README.md) | 25 行 | 文档导航，新增架构、运维、排查、面试和简历入口 |
| [docs/architecture.md](../../docs/architecture.md) | 184 行 | 分层、依赖、两条核心流程和已实现边界 |
| [docs/decisions.md](../../docs/decisions.md) | 662 行 | 先10后9的用户决定、补做清单与美元口径 |
| [docs/interview.md](../../docs/interview.md) | 849 行 | 阶段0–8、10全部必答题及项目问题、要点与出处 |
| [docs/learn/README.md](../../docs/learn/README.md) | 35 行 | 阶段学习入口 |
| [docs/learn/phase-05-doctor-tools.md](../../docs/learn/phase-05-doctor-tools.md) | 478 行 | 把迁移后的运行章节链接改到operations，学习正文不变 |
| [docs/learn/phase-08-eval.md](../../docs/learn/phase-08-eval.md) | 472 行 | 把迁移后的评测操作章节链接改到operations，冻结材料不变 |
| [docs/learn/phase-10-showcase.md](../../docs/learn/phase-10-showcase.md) | 311 行 | 初学者学习路线与真实开发过程回放 |
| [docs/notes.md](../../docs/notes.md) | 1744 行 | 命令、失败、计时、版本、数据保护与调试过程的原始记录 |
| [docs/operations.md](../../docs/operations.md) | 593 行 | 整体迁移原README的587行运行说明及其32个命令块 |
| [docs/reports/README.md](../../docs/reports/README.md) | 82 行 | 阶段报告与历史材料导航 |
| [docs/reports/phase-07-reliability.md](../../docs/reports/phase-07-reliability.md) | 211 行 | 追加用户最新美元账本口径，历史结果不改 |
| [docs/reports/phase-08-eval.md](../../docs/reports/phase-08-eval.md) | 153 行 | 追加用户最新美元账本口径，冻结评测不改 |
| [docs/reports/phase-08-formal-results.md](../../docs/reports/phase-08-formal-results.md) | 482 行 | 只改美元说明，195份报告及原评分保留 |
| [docs/reports/phase-10-checks.json](../../docs/reports/phase-10-checks.json) | 57 行 | 最终修复本地质量检查、竞态计数与源文件哈希 |
| [docs/reports/phase-10-ci-first.json](../../docs/reports/phase-10-ci-first.json) | 202 行 | 5806a7e首轮CI与五次原版成功的精确计时 |
| [docs/reports/phase-10-ci-fixed.json](../../docs/reports/phase-10-ci-fixed.json) | 502 行 | 修复与日志增强CI的完整步骤、首启及五轮数据 |
| [docs/reports/phase-10-code-inventory.md](../../docs/reports/phase-10-code-inventory.md) | 85 行 | 本阶段全部文件、行数及关键符号清单 |
| [docs/reports/phase-10-docker-preservation.json](../../docs/reports/phase-10-docker-preservation.json) | 398 行 | 首次演示前后原容器/卷对照与全局镜像标签显示变化 |
| [docs/reports/phase-10-eval-summary.json](../../docs/reports/phase-10-eval-summary.json) | 57 行 | 五组计数、算法说明与导出来源哈希 |
| [docs/reports/phase-10-eval-summary.py](../../docs/reports/phase-10-eval-summary.py) | 69 行 | 标准库只读汇总冻结导出，不运行模型或重评 |
| [docs/reports/phase-10-fresh-demo.json](../../docs/reports/phase-10-fresh-demo.json) | 48 行 | 首次远端新克隆与两次页面走查的实际计时和接口证据 |
| [docs/reports/phase-10-fresh-final.json](../../docs/reports/phase-10-fresh-final.json) | 71 行 | 最终c246734新克隆、Mock演示、SSE与日志链路实测 |
| [docs/reports/phase-10-log-isolation.json](../../docs/reports/phase-10-log-isolation.json) | 17 行 | 固定Alloy验证和独立项目同trace跨三服务日志证据 |
| [docs/reports/phase-10-mock-demo-final.png](../../docs/reports/phase-10-mock-demo-final.png) | 102357 字节（二进制） | 最终修复代码的真实Mock页面截图 |
| [docs/reports/phase-10-mock-demo.png](../../docs/reports/phase-10-mock-demo.png) | 55700 字节（二进制） | 真实本机Mock页面截图，不是生成效果图 |
| [docs/reports/phase-10-release-audit.json](../../docs/reports/phase-10-release-audit.json) | 37 行 | 最终链接、凭据、源码冻结与用户改动保护的归档检查 |
| [docs/reports/phase-10-resource-final.json](../../docs/reports/phase-10-resource-final.json) | 327 行 | 阶段结束全部测试项目和卷精确清单、原资源对照 |
| [docs/reports/phase-10-showcase.md](../../docs/reports/phase-10-showcase.md) | 168 行 | 阶段十节工程交付与验收报告 |
| [docs/reports/phase-10-startup-fix-validation.json](../../docs/reports/phase-10-startup-fix-validation.json) | 679 行 | 修复前后同条件对照、最终镜像清单及五次默认新卷消费验证 |
| [docs/reports/phase-10-startup-reproduction.json](../../docs/reports/phase-10-startup-reproduction.json) | 443 行 | 原版十轮复现参数、成功/停机失败与日志归属和哈希 |
| [docs/reports/phase-10-startup-root-cause.json](../../docs/reports/phase-10-startup-root-cause.json) | 54 行 | 受控放大下原版错误16退出的同窗日志与状态 |
| [docs/resume-draft.md](../../docs/resume-draft.md) | 48 行 | 不含个人信息的已验证项目文案，每个实测数字附台账 |
| [docs/troubleshooting.md](../../docs/troubleshooting.md) | 143 行 | 项目自身排查、冷启动证据、测试项目人工清理方法 |
| [docs/代码地图.md](../../docs/代码地图.md) | 497 行 | 重新生成426个非生成Go文件的说明与链接 |
| [docs/数据台账.md](../../docs/数据台账.md) | 753 行 | 新增P10真实数字，历史美元条目备注追加用户最新口径 |
| [internal/eval/report/markdown.go](../../internal/eval/report/markdown.go) | 98 行 | 只更新导出抬头的美元费用说明，不改评分统计 |
| [internal/platform/alertsink/doc.go](../../internal/platform/alertsink/doc.go) | 21 行 | 说明历史接收器保留供阶段3学习 |
| [internal/platform/alertsink/handler.go](../../internal/platform/alertsink/handler.go) | 81 行 | 去掉已过期的阶段5未来时态 |
| [internal/shop/app/inventoryservice/config.go](../../internal/shop/app/inventoryservice/config.go) | 39 行 | 配置职责说明与现有遥测/运维组装保持一致 |
| [internal/shop/app/shopctl/doc.go](../../internal/shop/app/shopctl/doc.go) | 24 行 | 阅读顺序包含已经实现的销量核对 |
| [internal/shop/events/consumer.go](../../internal/shop/events/consumer.go) | 200 行 | 只在Fetch边界有界恢复协调器错误14/15/16，处理与提交失败仍致命 |
| [internal/shop/events/consumer_fetch_test.go](../../internal/shop/events/consumer_fetch_test.go) | 220 行 | 八项新增回归覆盖恢复、永久错、预算、重置、空闲、取消、消息后边界 |
| [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | 215 行 | 测试装配面向MessageReader，供新的脚本读取器复用 |
| [internal/shop/events/doc.go](../../internal/shop/events/doc.go) | 33 行 | 新增测试文件导航 |
| [进度.md](../../进度.md) | 171 行 | 交付勾选、跳过阶段9的顺序说明和工程/学习验收分离 |
| [项目计划书.md](../../项目计划书.md) | 2164 行 | 保留原需求，追加阶段9/10/8.2的顺序决定及美元新口径 |

## 类型与函数

由 `commentcheck -list` 生成后筛选本阶段新增及关键修改，位置以验收源码为准；没有新增业务导出类型或公共接口。测试函数属于导出符号，一并列出。

| 名称 | 位置 | 作用 | 被谁调用 |
|---|---|---|---|
| `*Consumer.fetchMessage` | [internal/shop/events/consumer.go:108](../../internal/shop/events/consumer.go#L108) | fetchMessage 的重试只发生在尚未取得消息时，不会越过处理失败或尚未确认位点的消息 | Consumer.Run |
| `recoverableCoordinatorError` | [internal/shop/events/consumer.go:131](../../internal/shop/events/consumer.go#L131) | recoverableCoordinatorError 只放行真实冷启动中同类的协调器状态，不把鉴权或协议配置错误当成抖动 | fetchMessage |
| `waitFetchRetry` | [internal/shop/events/consumer.go:136](../../internal/shop/events/consumer.go#L136) | waitFetchRetry 使用可取消的定时器；直接 time.Sleep 会让退出信号白等剩余退避时间 | fetchMessage |
| `*Consumer.Run` | [internal/shop/events/consumer.go:77](../../internal/shop/events/consumer.go#L77) | Run 串行处理消息 | orderworker.serve |
| `TestConsumerCoordinatorBudgetResets` | [internal/shop/events/consumer_fetch_test.go:108](../../internal/shop/events/consumer_fetch_test.go#L108) | TestConsumerCoordinatorBudgetResets 保证两次独立恢复不会错误地共用一个进程终身次数 | go test |
| `TestConsumerCoordinatorRecoveryIdleContext` | [internal/shop/events/consumer_fetch_test.go:131](../../internal/shop/events/consumer_fetch_test.go#L131) | TestConsumerCoordinatorRecoveryIdleContext 防止恢复后安静的主题因人为新增短 deadline 被错杀 | go test |
| `TestConsumerCoordinatorErrorAfterFetch` | [internal/shop/events/consumer_fetch_test.go:162](../../internal/shop/events/consumer_fetch_test.go#L162) | TestConsumerCoordinatorErrorAfterFetch 保证错误码相同也不能把业务处理或提交失败当成尚未取到消息 | go test |
| `TestConsumerCancelCoordinatorRetry` | [internal/shop/events/consumer_fetch_test.go:185](../../internal/shop/events/consumer_fetch_test.go#L185) | TestConsumerCancelCoordinatorRetry 验证停止信号不会触发第二次读取，更不会处理消息 | go test |
| `TestFetchRetryWaitCancellation` | [internal/shop/events/consumer_fetch_test.go:206](../../internal/shop/events/consumer_fetch_test.go#L206) | TestFetchRetryWaitCancellation 用很长的待等待值证明取消会打断计时器，测试本身不会等待该时长 | go test |
| `fetchErrorReader` | [internal/shop/events/consumer_fetch_test.go:32](../../internal/shop/events/consumer_fetch_test.go#L32) | fetchErrorReader 的脚本中，nil 表示交给原内存队列取一条消息，非 nil 则只返回错误 | events 包回归测试 |
| `*fetchErrorReader.FetchMessage` | [internal/shop/events/consumer_fetch_test.go:41](../../internal/shop/events/consumer_fetch_test.go#L41) | FetchMessage 先返回脚本中的错误；错误不推进消息队列的位置 | events 包回归测试 |
| `TestConsumerRetriesCoordinatorErrors` | [internal/shop/events/consumer_fetch_test.go:60](../../internal/shop/events/consumer_fetch_test.go#L60) | TestConsumerRetriesCoordinatorErrors 覆盖同类初始化状态，尤其是实测出现的错误 16 | go test |
| `TestConsumerPermanentFetchError` | [internal/shop/events/consumer_fetch_test.go:81](../../internal/shop/events/consumer_fetch_test.go#L81) | TestConsumerPermanentFetchError 防止扩大成所有错误都重试，掩盖账号或协议配置问题 | go test |
| `TestConsumerCoordinatorRetryBudget` | [internal/shop/events/consumer_fetch_test.go:92](../../internal/shop/events/consumer_fetch_test.go#L92) | TestConsumerCoordinatorRetryBudget 验证持续失败只能重取三次，不能无限等一个坏配置 | go test |
| `testConsumer` | [internal/shop/events/consumer_test.go:113](../../internal/shop/events/consumer_test.go#L113) | testConsumer 把日志送到 Discard，让预期失败不会混进测试输出 | events 包回归测试 |

数据库表、迁移、HTTP/gRPC契约和端口均未变更。医生提示、工具、手册v1.1、评分与场景保持冻结。
