# source：把不同后端的证据交给医生

工具想知道“有没有慢请求”，不应该还要知道 Jaeger 的 JSON 怎么排字段。这个目录把两件事分开：本包定义医生需要什么，子包负责和实际产品交谈。这种把一种接口转换成另一种接口的做法，就是适配器。

先读 [doc.go](doc.go) 和 [source.go](source.go)。适配器在创建时绑定地址和账号，查询参数只描述要找的证据。这里没有任意命令执行器，也没有读取业务数据行的接口。

| 子包 | 负责取得的证据 |
|---|---|
| [prometheus](prometheus/doc.go) | 一段时间的指标、当前告警、版本与启动时间 |
| [jaeger](jaeger/doc.go) | 慢请求，以及一笔请求里的父子步骤 |
| [loki](loki/doc.go) | 按服务、时间、关键词和trace ID筛选日志 |
| [mysqlinspect](mysqlinspect/doc.go) | 执行计划、索引、表估计、SQL模板统计、锁等待 |
| [redisinspect](redisinspect/doc.go) | Redis运行状态和删去key/value的慢命令摘要 |
| [pprof](pprof/doc.go) | 哪些函数栈上停着较多goroutine |
| [httpbackend](httpbackend/doc.go) | 这些HTTP查询共同使用的固定地址、取消与响应大小限制 |

不是每个系统都要具备全部来源。缺少某项配置时，工具菜单不登记那一类工具，不能借另一个系统的数据凑答案。
