# 通用排障手册

遇到告警时，先确定哪里不正常，再逐步缩小范围。这些手册提供查证顺序，不替当前系统下结论。它们不包含实验场景编号、标准答案或专门为某次实验设计的判断口诀。

当前内容版本是 `runbooks-v1.0`，仍在阶段 8 的试跑前审阅阶段。正式评测开始前，还会记录全部文件的内容哈希；版本名相同不代表文件一定相同。

| 文件 | 解决的问题 |
|---|---|
| [时间与证据](time-and-evidence.md) | 怎样区分当前故障、历史异常和没有采集到的数据 |
| [接口变慢](request-latency.md) | 怎样沿一次请求找到耗时位置 |
| [数据库](database.md) | 怎样分别检查连接等待、SQL 执行、锁和资源 |
| [缓存](cache.md) | 怎样判断缓存请求、未命中和回源之间的关系 |
| [消息处理](messaging.md) | 怎样区分消息生产、传输和消费的问题 |
| [进程与并发](runtime.md) | 怎样观察资源增长、请求堆积和重试 |
| [变更与安全](changes-and-safety.md) | 怎样核对变更，以及怎样处理日志里的可疑指令 |

只有 E2 提供 `runbook_search`。工具按关键词相关程度返回片段；检索得分不是根因概率。没有匹配时返回空结果，不让另一个模型补写答案。README 只作导航，不进入检索正文。

检索使用 BM25：一个词在某篇手册出现得越集中、在其他手册越少见，通常越值得匹配；长文不能仅靠篇幅占便宜。实现采用固定的 `k1=1.2`、`b=0.75`，不根据评测结果调参。中文按相邻两个字匹配，英文按单词匹配，因此不保证理解同义词。公式背景见 [Stanford《Introduction to Information Retrieval》](https://nlp.stanford.edu/IR-book/html/htmledition/okapi-bm25-a-non-binary-model-1.html)。

实现按段落统计词频，采用 [Lucene 文档给出的正 IDF 公式](https://lucene.apache.org/core/9_12_3/core/org/apache/lucene/search/similarities/BM25Similarity.html)：`log(1 + (段落总数 - 含词段落数 + 0.5) / (含词段落数 + 0.5))`。同分时按文件名和片段编号排序。这里借用公开公式，项目没有引入 Lucene 依赖。
