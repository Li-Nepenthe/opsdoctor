# OT01 开跑前判定修订提案（尚未应用）

Claude 的采集修复审核已通过。付费准备中，OT01 原健康判定失败，尚未注入任何故障或创建付费诊断；不通过等待一个偶然正常窗口绕过这份失败。

## 同窗证据

按同一 Prometheus evaluation time `1793689256.454` 回看两分钟，原服务级健康查询为空。错误增量 `2.000133342222815` 来自 `flagd.evaluation.v2.Service/EventStream` 的 **CLIENT** span；`oteldemo.AdService/GetAds` 的 **SERVER** span 错误增量为 0、请求增量为 `44.00293352890193`。这些是 Prometheus increase 的估计增量，不能冒充精确整数请求数。证据见[同窗原始响应](phase-09-ot01-same-window-proof.json)和[首次失败核对](phase-09-paid-ad-baseline-investigation.json)。

控制面 EventStream 错误的原因尚未确定。它不等于广告 GetAds 业务失败，也不能据此认定控制面完全正常。官方故障 `adFailure` 的目标是 GetAds，由固定版[公开实现](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/ad/src/main/java/oteldemo/AdService.java)和原 OT01 标准答案定义；[开关注入前原件](phase-09-paid-flags-before-injection.json)已保留。

## 建议的最小修改

仅在 OT01 的健康与症状 PromQL 中增加 `span_kind="SPAN_KIND_SERVER",span_name="oteldemo.AdService/GetAds"`，使判断对象与该故障的业务入口一致。保持两分钟窗口、必须有请求且健康时无错误、故障时必须出现错误等要求。具体两行变动见[未应用 diff](phase-09-ot01-baseline-proposal.diff)。

不修改医生提示、工具、手册、评分 rubric、故障开关、标准答案或采集配置；不修改阶段 8。原健康失败仍保留，修订获准后使用新冻结批次并重新做注入与恢复验证，不把只读影子查询当作已验证故障注入。

也可以保留原条件，先定位并解决 EventStream 错误；在用户选择前暂停付费。依据用户“出现异常时停下”和 AGENTS 第 6 节的计划变更确认要求，先交付这份可审阅差异，不自行改场景后继续收费。
