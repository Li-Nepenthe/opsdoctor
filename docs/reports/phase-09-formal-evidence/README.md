# 阶段9正式评测原件

这个目录保留正式批次 `phase09-formal-20261103T181204Z-terra` 的只读快照证据。它对应真实 Terra E1/E2 正式诊断；双 Mock 流程原件另放在 `phase-09-dualmock-evidence`，两者不混算。

[原始快照](../phase-09-formal-snapshot.json) 的字段、原 TEMP 路径和诊断/评分均未改写。[manifest.json](manifest.json) 将每个源文件与采集来源映射到仓库副本，记录大小与 SHA-256。所有快照清单源先核对原大小与摘要，再以独占新建方式逐字节复制，目的文件随后复核；没有格式化 JSON 或转换换行。`.log` 文件可能装的是 JSON，文件名保留原编号。

[snapshot-helper.py](snapshot-helper.py) 是实际执行只读快照时的脚本快照，其摘要与原快照的 `helper_sha256` 相同。归档不执行它，也没有访问 DB、Docker、API 或模型。快照取数不需要登录 token 或模型 Key；目录不包含私有环境文件或认证头。

快照原件包含原 SQL 输出、原诊断与评分、全部收费尝试和两个分币种账户累计，以及 Collector 原日志/起点/审计/摘要、健康基线、症状首次确认和恢复证明。登录 token 从未写入这些证据；后补完整诊断 GET 的原响应另保存在根级报告文件，认证 token 同样只在请求进程内存使用。

[文件观察原件](../phase-09-formal-observations.json) 逐字节复制自 TEMP，并作为额外清单条目记录。这个观察文件只检查本地证据，不代表重新评分或新的付费准入批准。Collector `normal` 与零报告丢弃只描述已验证窗口，不能扩写为端到端零丢失。
