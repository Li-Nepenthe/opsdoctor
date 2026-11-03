# 阶段 9：Collector 起点锚点查询修复

2026-11-03。付费试跑继续暂停。本次只修改宿主评测器的日志起点查询，完成 Mock 检查和真实 Collector 的只读回归；没有创建诊断、切换故障、调用模型或重新运行试跑。重新开跑需要用户批准新的批次。

## 1. 中断与证据边界

原批次 `phase09-pilot-20261103-terra` 在任何模型调用前，因 `Collector起点锚点时间无法验证` 停止，未创建诊断，生成调用与两币种费用均为 0。故障开关随后恢复并通过健康检查，医生回到 Mock。原始状态见[中断记录](phase-09-pilot-interrupted.md)和[恢复证明](phase-09-pilot-recovery-proof.json)。

原失败调用在保存起点文件之前返回，其 Docker 输出没有留存。本报告保存的是同一 Collector 上重新验证的查询机制证据，不能当作恢复出的原失败输出。

## 2. 可重复的查询问题

旧代码用 `docker logs --timestamps --until <起点> --tail 1`。本机实际行为是先选择末行，再应用时间过滤。当末行晚于起点时，命令成功却返回空字节；即使容器仍保留起点之前的日志，也可能被这个组合排除。旧代码再将空字符串作为一行解析，于是报出时间无法验证。

只读回归把起点固定为观测时刻之前 60 秒：`2026-11-03T08:10:58.3492546Z`。同一保留日志中，最新一行时间为 `08:11:57.835300528Z`，确实晚于起点。

| 查询 | 原始字节 | 可解析行 | 结果 |
|---|---:|---:|---|
| 原 `tail 1 + until` | 0 | 0 | 命令成功，无法取得锚点 |
| 新 `tail all + until` | 4,588,422 | 13,146 | 找到起点之前最近的保留行 |

新查询选中 `08:10:57.033325246Z`。全部返回行的 Docker 时间均可解析，晚于起点的行数为 0。本次证据没有支持“标准输出与错误输出拼接损坏时间戳”的解释。查询原件分别为[最新行](phase-09-collector-anchor-fix-latest.log)、[旧查询空输出](phase-09-collector-anchor-fix-legacy-tail1.log)和[新查询全部保留行](phase-09-collector-anchor-fix-anchor-all.log)。

## 3. 最小修复与保持的检查

[生产代码](../../internal/eval/runner/generalization_collection.go)只把起点命令的 `--tail 1` 改为 `--tail all`，并为真正空或只有空白的输出返回“Collector起点前无保留日志，无法证明窗口覆盖”。没有增加重试或改变诊断窗口。

64 MiB 输出硬上限、固定容器身份、进程启动时间与重启次数、时钟余量、逐行时间解析、未来行拒绝、最近合法锚点选择，以及结束时的锚点保留与日志轮转检查均保留。现有日志配置的只读快照是 `json-file`、`max-size=5m`、`max-file=2`；超出命令硬上限仍然失败，不截断后冒充完整证据。

[Mock 回归](../../internal/eval/runner/generalization_collection_test.go)显式模拟旧查询空成功、新查询仍有旧行，并覆盖返回顺序不按时间排列时选取最近锚点、真正空输出、空白输出、未来行和超过 64 MiB。原有重启、轮转、命令失败、异常日志及评分前暂停测试继续通过。

## 4. 真实只读回归

显式设置 `TEST_PHASE09_COLLECTOR_DIR` 后运行 `TestIntegrationCollectorWindow`，测试只读现有 Collector，不访问数据库、模型或故障开关。没有设置该变量时跳过，CI 仍只跑 Mock。

本次观测时刻为 `08:11:58.349254600Z`，读取过去 60 秒的起点，再等待 2 秒结束；实际审计窗口为 `08:10:58.3492546Z` 至 `08:12:00.6681492Z`，共 62.3188946 秒。Go 测试通过，审计状态为 `ok`、质量为 `normal`，容器身份和起点锚点保留校验均通过。窗口原始日志 59,274 字节、169 行，报告最终丢弃事件 0、OpenSearch 日志 `dropped_items` 报告和 0、所有信号最终丢弃报告和 0、mapper 错误行与出现次数均为 0。

这里的 `normal` 表示这份有限窗口证据通过了既定检查，没有报告最终丢弃或 mapper 错误；它不等于端到端零丢失，也不表示 Collector 从未出现其他错误。

原件为[起点证明](phase-09-collector-anchor-fix-start.json)、[窗口日志](phase-09-collector-anchor-fix-window.log)、[审计 JSON](phase-09-collector-anchor-fix-audit.json)及[测试日志](phase-09-collector-anchor-fix-integration.log)。窗口日志 SHA256 为 `b8a45ac981e7d92384d73971c5d4355f9e79e1382698b8758d1677825d08139a`。执行包装在 Go 返回 0 后打印中文时遇到 cp1252 编码错误；保存的 Go 原始日志完整含 `PASS`，测试与证据保存不受影响。

这些文件通过二进制读写复制，未重编码，原 Temp 文件保留。`.gitattributes` 对新日志和原件 JSON 禁止换行转换。每份文件的完整字节数、SHA256、原路径及时间解析统计见[量化 JSON](phase-09-collector-anchor-fix.json)；[环境快照](phase-09-collector-anchor-fix-environment.json)保留同一容器的日志配置。

## 5. 工程检查与下一步

[本轮全仓检查](phase-09-anchor-fix-checks.json)全部通过：gofmt、go vet、staticcheck、commentcheck、archcheck 和 race。顶层测试 PASS 306、SKIP 48、FAIL 0；含子例 PASS 1,033、SKIP 58、FAIL 0；共 74 包，其中 58 通过、16 跳过。commentcheck 覆盖 443 个非生成 Go 文件。

全仓检查清除了 `TEST_*` 和模型 Key，使用 Mock，未调用 Docker。六份旧检查及其原日志保持不变，两处用户未提交文件的哈希保持不变。`doctor/tools`、`doctor/agent`、`source/httpbackend`、`eval/judge`、`eval/scoring` 相对 main 的差异均为空。

本次没有补跑原批次，没有产生新的诊断结果或评分。下一步由用户审核这项修复及免费回归证据，再决定是否批准新批次的付费试跑。
