# 阶段9正式评测失败与证据边界分析

本分析只读批次 `phase09-formal-20261103T181204Z-terra` 的[原快照](phase-09-formal-snapshot.json)、[完整诊断 GET 原响应](phase-09-formal-diagnosis-details.json)、[原结果导出](phase-09-formal-results.md)及冻结场景答案。完整原响应逐条 SHA-256 已核验，终报与快照一致；[分析 JSON](phase-09-failure-analysis.json)保留十份原报告、原评分、步骤引用及精确组件字符串。没有重新诊断、重新评分或改写判分。

## 原结果及分母

两组各五份真实 Terra 正常报告，全部主动提交，原 DeepSeek 评分均已成功保存。E1、E2 组件一致各为 3/5；机制正确/部分/错误各为 2/2/1，原 coverage 均值各为 0.75、有效 n=5。它们是五个不同场景的一次运行，不是生产准确率，也不能因为平均值相同就认定两组能力相等。当前双 Mock、原中断空槽及原 pilot2 不进入本表。

| 场景/组 | 报告组件原字符串 | 冻结组件 | 原组件一致 | 原机制 | 原 coverage |
|---|---|---|---|---|---:|
| OT01/E1 | `ad` | `ad` | true | correct | 1.00 |
| OT01/E2 | `ad` | `ad` | true | correct | 1.00 |
| OT02/E1 | `cart->valkey-cart` | `cart` | false | partial | 1.00 |
| OT02/E2 | `cart->valkey-cart` | `cart` | false | partial | 1.00 |
| OT03/E1 | `payment` | `payment` | true | correct | 0.75 |
| OT03/E2 | `payment` | `payment` | true | correct | 1.00 |
| OT04/E1 | `unknown` | `checkout->payment` | false | incorrect | 0.50 |
| OT04/E2 | `unknown` | `checkout->payment` | false | incorrect | 0.50 |
| OT05/E1 | `product-catalog` | `product-catalog` | true | partial | 0.50 |
| OT05/E2 | `product-catalog` | `product-catalog` | true | partial | 0.25 |

组件一致来自非空字符串的完全相等；机制、coverage 则来自冻结 rubric-v1.1 下的原 DeepSeek 判断。这三项不能互相替代。本批没有评分缺失；历史评分失败仍保留缺失，不填零、不重评。

## OT02：组件 false 与机制 partial 是两件事

[冻结答案](../../eval/scenarios/otel-demo/OT02.yaml)把故障组件定义为 `cart`。E1 与 E2 的原 `root_cause.component` 都是 **`cart->valkey-cart`**，并非 `cart`，原程序精确比较后得到 false；本分析不把链路包含 cart 改成一致，也不把所有 false 解释为完全没找到业务方向。

两份报告都用 EmptyCart 的 FAILED_PRECONDITION 与无法连接 Redis 兼容存储的日志，定位到清空购物车及连接异常。E1 还保留日志与 trace 关联尚未证实的限制，E2 使用同一 trace 关联。原 DeepSeek 给 partial 的理由是两者都没有识别 **EmptyCart 专门使用 `badhost:1234` 的另一份 ValkeyCartStore，而 AddItem/GetCart 仍使用正常存储**。两份建议继续核对 `valkey-cart:6379`，不能写成已证明这个正常存储实例宕机；已保存工具结果也未返回 `badhost` 或 `1234`。

两组的原 coverage 都为 1，说明四项证据里程碑被原评委记为覆盖；它不等于完整机制已识别。原 component=false、mechanism=partial、coverage=1 全部并存保留。

## OT04：终报明确 unknown，缺少错误支付地址证据

[冻结机制](../../eval/scenarios/otel-demo/OT04.yaml)是 checkout 连接 `badAddress:50051`。两份原组件均为 **`unknown`**，报告只能确认结账 HTTP 500、checkout PlaceOrder 耗时和失败传播，未判断到底是内部业务、连接还是下游问题。E1 引用步骤 12 的调用树被截断；E2 引用步骤 14，也承认关键后半段不可见。原评委均给 incorrect、coverage=0.5。

完整步骤核实两组相关 `get_trace` 都实际返回 `truncated=true`，原始数据大小为 5200 与 8352 字节，工具只展示开头；这不是本分析展示输出造成的截断。已保存工具返回中没有 `badAddress`、`50051` 或 `UNAVAILABLE`。这证明医生当时可见的材料不足，不能倒推 Jaeger 中原始链路根本没有错误信息，也不能声称换一种呈现就一定诊断成功。

原评委把第二项里程碑记为达成，理由同时承认仅缩小到 PlaceOrder、未定位支付下游；后两项为未达成。本分析保留这一判定边界，不自行补分或扣分。报告保持未知比捏造地址原因更谨慎，但在本次冻结机制判断中仍是未识别主因。

## OT05：部件找到，商品范围和主动错误未证实

[冻结答案](../../eval/scenarios/otel-demo/OT05.yaml)要求定位商品 `OLJCESPC7Z` 的 GetProduct 在查询 SQL 之前主动返回 Internal。两组都写 `product-catalog`，组件一致；报告都明确承认没有取得失败请求的商品 ID、完整错误状态和具体代码原因。已保存工具结果中未出现 `OLJCESPC7Z`。

这里与 OT04 不同：相关树形 `get_trace` 原结果为 `truncated=false`，但树中仍未呈现所需的商品属性与关键错误状态。报告以某些 GetProduct 没有数据库子 span、同链其他查询有 SQL 子 span 为旁证，同时保留采样/埋点缺失的可能；不能把“没有展示数据库子 span”写成“已证明错误在 SQL 之前”。两组原机制均为 partial。

E1 原 coverage 为 0.5，第四项“与其他商品成功查询对照”被原评委判为覆盖；E2 为 0.25，原评委认为没有做足商品级失败/成功对照。这个差异照录，不据此给检索手册下优劣结论。

## 其余报告与 EventStream 的实际位置

OT01 两组都把根因部件定位为 ad，并用 GetAds 的 UNAVAILABLE 与同时存在的成功请求支持间歇性失败；底层触发开关仍被报告列为未知。原评委按冻结机制边界给 correct，不能扩写成医生已经发现随机故障实现。OT03 两组都定位 payment 的 Invalid token 向结账传播，原机制均 correct；E1 第四项传播证据被原评委判未达成，coverage=0.75，E2 原 coverage=1，均不修改。

完整步骤中，EventStream 实际出现在以下 `query_metrics` **工具返回**，并作为历史证据进入后续 LLM 请求。下表的步骤编号是审计位置，不是唯一错误事件数。

| 报告 | 工具结果步骤 | 后续 LLM 请求含该历史证据 | 可见 LLM 响应提及 | 终报提及 |
|---|---|---|---|---|
| OT01/E1 | 7 | 12, 19, 26 | 无 | 无 |
| OT01/E2 | 12, 13, 21 | 19, 27, 33 | 无 | 无 |
| OT05/E1 | 21 | 23, 29, 34 | 无 | 无 |

其余报告的工具返回未匹配 `EventStream` 或 `stream closed due to server-side timeout`。十份终报与可见 LLM 响应均未匹配这些词，也没有把 EventStream 写成根因；终报同样没有展开一段明确排除它的论证。**LLM 请求中的命中是工具历史回填，不能误写成模型主动推理认定了 EventStream。**

因此可报告它作为旁支观测进入过输入，未被可见输出列为本次根因；不能证明模型内部未受干扰，也不能把所有失败归因于该噪声。此处未修改、屏蔽或清理这些证据。

## 结论的适用范围

五个场景每组各一次，结果只描述这批样本。DeepSeek 是同一冻结评估中的自动评委，并非独立人工金标准；原 GT、医生报告、组件精确匹配和评委意见分别保留，不能用评委分数自证绝对正确。Collector 的 normal、零报告丢弃/映射错误只说明对应窗口审计通过，不保证端到端零丢失，也不保证医生看到的工具摘要包含全部原始字段。

本轮只归档及解释已有结果，没有改提示 v8.0、工具、医生 agent、手册 v1.1 或 rubric-v1.1，也没有新增模型调用。若后续处理截断、字段呈现或组件粒度，应先另行批准新版本及两个系统的重跑成本，不能回填本批成绩。
