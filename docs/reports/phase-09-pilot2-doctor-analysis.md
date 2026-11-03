# pilot2 Terra 原报告事实复核

本次只读复核 `phase09-pilot2-20261103-terra` 的诊断 `d6fa4da91a42bb7c184478f9c0fdcc05`。医生以 `report_mode=voluntary` 主动提交，原组件字段是 `ad`，同时保留“具体触发原因尚未确定”。DeepSeek 评分 JSON 校验失败，用户要求的“试跑评分正常后再进入正式”门槛尚未满足。本报告不补机制分、证据分或覆盖率，也不重评。原报告、参数、工具返回和步骤元信息见[公开诊断快照](phase-09-pilot2-doctor-analysis.json)，停止位置见[原运行日志](phase-09-pilot2-run.log)。

## 原报告与完成方式

| 原字段 | 本次保存值 |
|---|---|
| 状态 / 提交方式 | `succeeded` / `voluntary` |
| 预算收尾原因 | 空字符串 |
| 模型 / 实际步骤模型 | `gpt-6.1-terra` / 仅 `gpt-6.1-terra`，原摘要 `mixed_model=false` |
| 调查轮数 | 5 |
| 查证工具调用 | 23；另有 1 次 `submit_report`，不计入该字段 |
| 原记录耗时 | 31070 ms，采用保存的单调时钟测量，不相减数据库时间戳 |
| 输入 / 输出 token | 50506 / 1323，输入包含缓存命中 |
| 医生美元账本 | USD 0.027179，按项目用量账本计 |
| 写提议 / 写执行 | 0 / 0 |

美元 Key 与其他用途共用，平台账单无法单独核对。医生 USD 与评委 CNY 分开，不能相加；评分失败的原费用也保留在快照中。以上只复述[原字段](phase-09-pilot2-doctor-analysis.json)，运行成功不等于机制或证据已通过评分。

原摘要：

> 本次广告查询出现少量间歇性 UNAVAILABLE；已定位到 ad 的 GetAds 处理，但具体触发原因尚未确定。

原 `root_cause.mechanism`：

> ad 在处理 GetAds 时对部分请求返回 gRPC UNAVAILABLE，失败传播至 frontend 的广告调用；其余请求仍可成功。日志未提供错误描述或底层异常，不能确定故障注入、依赖或代码等具体原因。

原 `unknowns`：

> 具体触发机制、失败请求的完整调用链、精确错误率及真实用户受影响范围未确认；不能据正常样本推断失败时的 HTTP 响应，也不能推断其他业务受损。查询窗口包含故障前五分钟，但所引三条失败日志均在故障开始之后。

这里保持模型原文，不依据已知场景答案补全它没有确认的触发原因。

## 原证据引用与实际工具链

报告引用顺序为 `[33, 30, 29, 32, 23]`，这些编号都对应已保存的工具步骤。下面只指出原件位置和内容，不判定里程碑是否达成。

| 原引用步骤 | 原工具与保存内容 |
|---|---|
| 33 | `search_logs`，ad、关键词 GetAds Failed、显式近期窗口；原返回包含 UNAVAILABLE 日志，description/cause 为空 |
| 30 | `query_metrics`，GetAds 的 ad SERVER 原计数器；分别保留 ERROR 与 UNSET 序列的范围 |
| 29 | `query_metrics`，按 GetAds 分开的 ad 与 frontend span 指标；increase 是估计增量，不当精确整数请求数，两侧不相加 |
| 32 | `query_metrics`，GetAds RPC 状态码分别包含 0 与 14，原数值保留在快照 |
| 23 | `get_trace`；原报告将它说明为正常路径对照，并明确它不是失败链路 |

调查先查询告警、指标名、慢调用链、变更与日志，再补不限制最小耗时的 trace 搜索，获取一条实际 trace，最后聚焦 GetAds 指标与失败日志。步骤 21 使用 `traces_spanmetrics_calls_total` 得到空序列；步骤 22 发现实际名称 `traces_span_metrics_calls_total`，后续查询使用该名称。这是保存轨迹中可见的查询修正，不据此给推理过程打分。

步骤 16、17、18 的原返回标为 `truncated=true`，不能视为完整日志或完整搜索结果。步骤 36 的 `submit_report` 返回“报告已通过校验，建议操作仅记录”；步骤 37 为 succeeded。格式校验通过不等于机制或证据评分通过。各次查证的参数与返回完整保留在[公开快照](phase-09-pilot2-doctor-analysis.json)；非工具步骤中重复的整段请求 args 不再公开复制，保留其 canonical SHA256 和其他元信息，数据库原记录没有修改。

## EventStream 是否改变了结论

可见工具轨迹里，EventStream 字样出现在步骤 24 的 ad RPC client 指标结果，该次返回的 EventStream 序列 last/min/max 均为 0。这只描述这一条返回序列，不能推广成控制面从未出错。

原报告组件是 `ad`，摘要、机制和证据文字未提 EventStream 或 flagd；可确认它没有被写成这份报告的根因。最终证据引用集中在 GetAds 指标、日志与正常路径对照。是否影响了模型内部假设或调查效率，无法从一次外显轨迹量化，不能写成“完全不受干扰”。这属于原文和步骤事实核对，不是机制评分。

## 采集与停止门槛

原摘要记 `data_quality=normal`、Collector audit `status=ok`；该窗口原始日志的本地字节数与 SHA256 再次核对相符。保存的最终丢弃与 mapper 计数均为零，具体边界和计数见[快照](phase-09-pilot2-doctor-analysis.json)的 `eval_result.data.collector_audit`。这个窗口结论不等于端到端没有静默缺口。

原 `judgement.status` 为 `failed`，错误为 `评分 JSON 格式不合法`。空机制字段与默认 `coverage=0` 都不是有效评分，不能据此统计机制零分或覆盖率零分。[运行日志](phase-09-pilot2-run.log)保留了医生成功、采集审计正常和评分异常后停止的次序；[恢复记录](phase-09-pilot2-emergency-restore.log)由根代理核对。医生报告成功没有满足“评分正常后继续正式”的完整条件。

本复核仅以只读事务读取新医生库的指定行，并核对已保存原件；没有调用医生工具、模型或故障开关，没有改报告、评分、数据库、源码或冻结配置。机制和证据评分继续保持缺失，后续处理由用户决定。
