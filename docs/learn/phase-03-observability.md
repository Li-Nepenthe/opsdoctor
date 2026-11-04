# 阶段 3 学习材料：可观测性与告警

前三个阶段，系统已经能正常工作了。但出了问题，你几乎什么都“看不见”：系统慢了吗？慢在哪一步？哪个订单失败了？

这个阶段给系统装上“眼睛”：

- **指标**：一段时间内的数字，比如每秒请求数、P99 耗时、错误率。
- **调用链**：一次请求经过了哪些操作、每一步花了多久。
- **日志**：每件事发生时留下的记录。
- **看板**：把上面三样放在一个页面里看。
- **告警**：指标异常时自动发出通知。

这也是医生能工作的前提：阶段 5 以后，医生诊断时调用的工具，查的就是这些数据。

这个阶段还有一次很有价值的**真实排障**（第 3.5 节）：从一个奇怪的现象出发，一步步找到藏在 gRPC 库里的根因。面试时讲这种经历，比背概念有说服力得多。

**配合小林coding 阅读。** 和前几个阶段一样，用到基础知识时先用几句话讲清楚（标着“补基础”），再给出 [小林coding](https://www.xiaolincoding.com/) 里讲同一知识点的文章（标着“对照阅读”）。小林coding 没有专门讲 Prometheus、Jaeger 这些监控工具，所以这一阶段的对照阅读集中在排障思路、DNS 和 Go 写法上；监控本身的概念，以本材料的补基础为准。

| 本材料 | 小林coding 对应文章 | 去那边重点看什么 |
|---|---|---|
| 1、3.5 用三种数据排障 | 《系统设计面试题》[对外提供一个 API 服务，客户说请求接口超时了，怎么排查？](https://www.xiaolincoding.com/interview/systemdesign.html#对外提供一个api服务-客户说请求接口超时了-怎么排查) | 先看监控定范围，再用请求 ID 串起日志、用链路追踪拆开耗时；为什么不能只把超时调大 |
| 3.5 DNS | 《图解网络》[真实地址查询 —— DNS](https://www.xiaolincoding.com/network/1_base/what_happen_url.html#真实地址查询-dns) | 域名怎样一级一级解析成 IP，本地 DNS 服务器和缓存起什么作用 |
| 5.4 context 带着 span 往下传 | 《Golang面试题》[6.2 Go 语言的 context 有什么作用？](https://www.xiaolincoding.com/interview/golang.html#_6-2-go语言的context有什么作用) | context 的三个用途：超时、取消、传递请求级数据；调用链信息属于第三种 |
| 5.6 错误率告警 | 《图解网络》[HTTP 常见的状态码有哪些？](https://www.xiaolincoding.com/network/2_http/http_interview.html#http-常见的状态码有哪些) | 4xx 和 5xx 的区别，错误率告警为什么只数 5xx |
| 5.6 goroutine 告警 | 《Golang面试题》[10.5 Go 语言在什么情况下会发生内存泄漏？](https://www.xiaolincoding.com/interview/golang.html#_10-5-go语言在什么情况下会发生内存泄漏)、[10.6 发生了内存泄漏如何定位和优化？](https://www.xiaolincoding.com/interview/golang.html#_10-6-go语言发生了内存泄漏如何定位和优化) | goroutine 泄漏是最常见的一种；先看 goroutine 数量的曲线，再用 pprof 找它们卡在哪里 |
| 6.1 给切片类型加方法 | 《Golang面试题》[2.4 slice 作为函数参数传递，会改变原 slice 吗？](https://www.xiaolincoding.com/interview/golang.html#_2-4-slice作为函数参数传递-会改变原slice吗)、[2.2 slice 是怎么扩容的？](https://www.xiaolincoding.com/interview/golang.html#_2-2-go语言里slice是怎么扩容的) | 为什么 `Set` 必须用指针接收者 |
| 6.2 装饰器 | 《Golang面试题》[7.4 Go 语言 interface 有哪些应用场景？](https://www.xiaolincoding.com/interview/golang.html#_7-4-go语言interface有哪些应用场景) | 依赖注入和解耦、Web 框架的中间件，都靠“同一个接口，换一个实现” |
| 实验二 熔断器状态 | 《分布式面试题》[限流、熔断和降级有什么区别？](https://www.xiaolincoding.com/interview/cap.html#限流、熔断和降级有什么区别) | 熔断器的关闭、打开、半开三个状态怎样切换 |

## 1. 这个阶段做了什么

**三种数据，各回答一个问题：**

| | 回答什么问题 | 存在哪里 | 怎么看 |
|---|---|---|---|
| 指标 | 系统整体怎么样？变慢了吗？错误变多了吗？ | Prometheus | Grafana 看板 |
| 调用链 | 这一笔请求慢在哪一步？ | Jaeger | Jaeger 页面，或 Grafana |
| 日志 | 当时具体发生了什么？哪个订单失败了？ | Loki | Grafana 的 Explore |

三者互相补充。比如：看板显示下单变慢了（指标）→ 找一笔慢的下单，看它的调用链，发现慢在扣库存那一步（调用链）→ 用这笔请求的 trace_id 查日志，看库存服务当时报了什么错（日志）。

**数据是怎么流动的：**

```mermaid
flowchart LR
    S[三个服务] -->|"/metrics，每 10 秒被采集一次"| P[Prometheus]
    E[MySQL / Redis / Kafka 的 exporter] --> P
    S -->|span| J[Jaeger]
    S -->|"JSON 日志打印到标准输出"| A[Alloy] --> L[Loki]
    P --> G[Grafana]
    J --> G
    L --> G
    P -->|告警| AM[Alertmanager] --> AS[alertsink]
```

现在一共有 17 个容器：原来的 7 个（MySQL、Redis、Kafka、库存服务、订单服务、order-worker、压测工具），加上 Prometheus、3 个 exporter、Jaeger、Loki、Alloy、Grafana、Alertmanager 和告警接收器 alertsink。

alertsink 是临时的：它只把收到的告警写进日志，不判断原因，也不修复。阶段 5 以后由医生接替。

**补基础：拉取和推送。** 三种数据进入存储的方式不一样。指标是 Prometheus 主动来“拉”：每 10 秒访问一次各服务的 `/metrics` 地址，把当时的数字取回去，这一步叫采集（scrape）。调用链和日志则是服务主动“推”出去：span 攒成一批发给 Jaeger；日志直接打印到标准输出，由 Alloy 从容器里收走，再转给 Loki。拉取有个好处：Prometheus 自己知道“这次没拉到”，会把这个目标的 `up` 记成 0，所以服务挂了也能发现（第 9 节第 6 题）。

**补基础：时间序列。** Prometheus 里的一条数据由“指标名 + 一组标签”确定，比如 `http_server_request_duration_seconds_count{service="order-service", route="/orders", method="POST", status="201"}`。每采集一次，这条线上就多一个点（时间，数值），这样一串点叫一条**时间序列**。第 5.3 节说的“标签组合太多会撑爆存储”，指的就是这种线太多。

## 2. 怎么跑起来、看什么

所有命令都在仓库根目录运行，开始前先打开 Docker Desktop。全部 17 个容器大约占 2 GB 内存。

### 2.1 启动全部服务

你的 `.env` 里已经有阶段 3 需要的几个变量（Grafana 管理员密码、监控账号密码、告警令牌），Astra 开发时已经补好了。如果启动时提示缺少某个变量，参照[操作手册](../operations.md#启动靶场环境)的“启动靶场环境”一节补上。

```powershell
$env:LOADGEN_QPS = '0'
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
Invoke-WebRequest http://127.0.0.1:3100/ready
```

你应该看到 17 个容器都在运行；配了健康检查的显示 `healthy`；最后一条（Loki）返回 `ready`。Loki 刚启动时可能要再等十几秒。

### 2.2 开一点流量，看指标

```powershell
Invoke-RestMethod http://127.0.0.1:6070/control -Method Post -ContentType 'application/json' -Body '{"qps":20,"hot_sku_id":0}'
(Invoke-WebRequest http://127.0.0.1:6061/metrics).Content | Select-Object -First 1
```

然后在浏览器打开 http://127.0.0.1:9090/targets，你应该看到 8 个采集目标都是 UP。

在 Prometheus 的首页（http://127.0.0.1:9090）输入查询 `mysql_up`，结果应该是 1。UP 只说明 Prometheus 能访问到 MySQL 的 exporter，`mysql_up=1` 才说明 exporter 能连上 MySQL。

### 2.3 看看板

打开 http://127.0.0.1:3000，用户名 `admin`，密码是 `.env` 里的 `GRAFANA_ADMIN_PASSWORD`。在 OpsDoctor 文件夹里打开“shop 总览”，等两分钟，让数据积累起来。你应该看到 12 个面板都有数据：请求数、P99、错误率、连接池、缓存命中率、消息积压、goroutine 数、内存、熔断器状态、重试次数、连接等待时间、幂等处理次数。

### 2.4 追踪一笔订单

因为默认只记录十分之一的请求，随便下一单不一定能在 Jaeger 里查到。下面手动生成一个 trace ID，并在请求头里要求“一定记录这个请求”：

```powershell
$traceID = [guid]::NewGuid().ToString('N')
$headers = @{
    'traceparent' = "00-$traceID-1234567890abcdef-01"
    'Idempotency-Key' = 'learn-trace-' + [guid]::NewGuid().ToString('N')
}
$body = '{"user_id":880000303,"sku_id":499995,"quantity":1}'
$order = Invoke-RestMethod http://127.0.0.1:8081/orders -Method Post -Headers $headers -ContentType 'application/json' -Body $body
$order.data
$traceID
```

`traceparent` 是 W3C 规定的调用链请求头，格式是“版本-trace ID-上一步的 span ID-标志”，最后的 `01` 表示“需要记录”。

**在 Jaeger 里看：** 打开 http://127.0.0.1:16686，在右上角的搜索框粘贴 trace ID。等几秒，你应该看到这笔订单经过了三个服务：订单服务、库存服务、order-worker。依次是：HTTP 请求 → Redis 占位 → gRPC 扣库存 → 库存服务的 SQL → 写订单的 SQL → 发 Kafka 消息 → order-worker 消费 → 加销量的 SQL。

一条 SQL 可能显示成好几个 span（准备、执行、提交），这不代表执行了好几次。span 的总数也会随缓存是否命中而变化，不用纠结具体数字。

**在 Grafana 里查日志：** 左侧菜单打开 Explore，数据源选 Loki，输入下面的查询（把占位文字换成你的 trace ID）：

```logql
{service=~"order-service|inventory-service|order-worker"} | json | trace_id="填入你的 trace ID"
```

你应该看到三条日志，分别来自三个服务：订单服务的 HTTP 请求、库存服务的 gRPC 请求、order-worker 的消息处理。它们的 trace_id 相同，span_id 不同。日志旁边有一个 TraceID 链接，点击可以直接跳到这条调用链。

### 2.5 结束

```powershell
Invoke-RestMethod http://127.0.0.1:6070/control -Method Post -ContentType 'application/json' -Body '{"qps":0}'
```

等消息积压归零，可以用 `shopctl check` 核对一次（方法见阶段 2 学习材料第 2.7 节），应该是 `gap=0`。不学的时候，记得停掉容器释放内存：

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml down
```

## 3. 开发过程回放

这一节按实际发生的顺序讲。原始记录在 [notes](../notes.md#phase-03)，失败的尝试都保留着。

### 3.1 先修一个小问题：订单写成功后，消息要发出去

**要做什么：** 阶段 2 验收时发现，订单写进数据库以后，发 Kafka 消息用的还是这次 HTTP 请求的 context。如果用户这时断开了连接，context 被取消，消息就发不出去，销量会少算一笔。

**改法：** 发消息改用“不带取消状态、但有自己时限”的 context（`context.WithoutCancel` 再加 `WithTimeout`，阶段 0 第 6.6 节讲过）。用户断开只表示他不再等响应，不代表订单可以撤销。新的 context 还保留着原来的调用链信息，所以发消息这一步仍然在同一条 trace 里。

**怎么验证：** 写了一个测试：订单提交后取消原来的 context，确认发送消息时拿到的 context 没有被取消、带着原来的信息、并且有时限。

**这不是完整的解决方案：** 进程在订单提交后崩溃，或者 Kafka 本身发送失败，消息还是会丢。根本的办法是 Outbox（阶段 2 第 5.8 节），这个阶段没有实现。

### 3.2 把指标、调用链、日志接到业务上

**要做什么：** 每个服务输出同一套名字的指标；一笔订单能从订单服务一路追到 order-worker；日志能和调用链对上。

**动手前先想清楚的问题：**

- 一个请求会产生很多 span，但它们必须共用同一个 trace ID，才能连成一条链。
- 指标的标签只能用种类有限的值，不能用订单号这种无限增长的值（第 5.3 节）。
- 业务代码不应该自己去设置全局的监控对象。

**写的顺序：**

1. 写 telemetry 包：每个服务一个独立的指标注册表、调用链的 Provider 和 Propagator。
2. 把 Gin、gRPC、SQL、Redis 的现成插件接到各服务的组装代码上。
3. Kafka 没有现成插件，手写消息头的读写（kafka.go）。
4. 让日志自动带上当前 span 的 trace_id 和 span_id。
5. 给缓存、幂等这些操作加计数：用包装的方式（第 5.9 节），业务包不用改。
6. 补测试，再启动整套监控组件做真实验证。

**怎么验证：** Kafka 的单元测试确认消费者拿到的 trace ID 和父 span 都对；HTTP 的单元测试确认 `/orders/1` 和 `/orders/2` 记在同一个标签 `/orders/:id` 下。真实环境里，一笔订单的调用链经过三个服务，Grafana 按 trace ID 能查到三条对应的日志。

**遇到的问题：** commentcheck 查出新文件没写进 doc.go、长函数缺少注释，逐个补上。订单服务的 `Run` 函数变得太长，于是拆出 dependencies.go 和 servers.go，读起来不用一下子面对一大段连接代码。

### 3.3 启动监控组件，用真实业务验收

**要做什么：** 从“代码里埋了点”走到“看板能看、告警能收到”。

**遇到的问题：**

1. **Loki 的健康检查失败。** Loki 的官方精简镜像里没有 `wget` 命令，健康检查执行不了。改成从宿主机和 CI 里检查它的 `/ready` 地址。
2. **Grafana 查不了 Jaeger。** 实测发现 Jaeger 2.22.0 去掉了 Grafana 还在用的一个接口，返回 404。Astra 按规则先停下来问你，你同意后把 Jaeger 固定在 2.20.0，同样的配置就能用了。对照证据和以后什么时候可以升级，都记录在 [decisions](../decisions.md#jaeger-compatibility) 里。
3. **旧数据卷没有监控账号。** MySQL 的监控账号由初始化脚本创建，但旧数据卷不会重新执行初始化脚本，于是在已有的数据卷上手动执行了一次。

**结果：** 8 个采集目标都是 UP，12 个面板都有数据。200 QPS 下停掉 order-worker，167 秒后收到了 ConsumerLagHigh 告警，恢复后收到了 resolved 通知。暂停流量后积压归零，订单件数和销量件数相等。

### 3.4 全新环境暴露了初始化脚本的问题

**现象：** GitHub 上的 CI 从空数据卷启动时，MySQL 启动失败退出了，但在本机的旧数据卷上一直是好的。

**排查：** 本机用一个全新的临时容器直接挂载脚本，启动成功；但把脚本的文件权限改成 Linux 上检出代码后的 0644（没有执行权限），就复现了失败。

**原因：** MySQL 官方镜像对没有执行权限的 `.sh` 脚本，会用 `source` 的方式运行。`source` 的意思是“在当前的 shell 里执行这个文件的内容”，而不是另开一个进程。脚本里写了 `set -u`（遇到未定义的变量就报错），这个设置就留在了镜像自己的启动脚本里，后面的步骤用到某个可选的未定义变量时就报错退出了。

**修复：** 用圆括号把脚本内容包起来。圆括号会开一个子 shell，里面的设置和密码变量都不会影响外面。修复后在全新的空数据卷上验证通过，CI 也通过了。

这件事说明：在旧环境里测试通过，不等于在全新环境里也能通过。CI 每次都从空白开始，正好能发现这类问题。

**补基础：文件权限 0644 和 0755。** Linux 给每个文件记三组权限：文件主人、同组用户、其他人。每组三项：读（r，记 4）、写（w，记 2）、执行（x，记 1），加起来就是一个数字。`0644` 是“主人能读写（4+2），其他人只能读（4）”，谁都不能执行；`0755` 是三组都能执行（7=4+2+1，5=4+1）。Windows 不记录这种权限，所以同一个脚本，在你电脑上检出和在 CI 的 Linux 上检出，权限可能不一样。

**补基础：直接执行、source 和圆括号。** 直接执行一个脚本（比如 `./init.sh`），会另开一个新的 shell 进程来跑，脚本里的 `set -u` 和变量都随这个进程结束而消失。`source init.sh` 则是在当前的 shell 里逐行执行，脚本改过的设置会留下来，影响后面的命令。圆括号 `( ... )` 会开一个子 shell，括号里的设置出了括号就失效。所以把脚本内容包进圆括号，不管别人用哪种方式运行它，都不会影响外面。

### 3.5 真实排障：查到了 IP，为什么调用还是卡住了

这是这个阶段最值得学的一段，完整地讲一遍。

**现象。** 你在验收时，暂停流量 34.5 分钟后又恢复了流量。恢复后的头一两秒，所有调用库存服务的请求都等满了 800 毫秒才超时，两个熔断器都打开了，产生了 12 笔“结果待核对”的订单。十几秒后一切恢复正常。

此前 Astra 也遇到过两次奇怪的超时（数据台账 P3-13、P3-14），但当时没有找到原因。

**第一条线索：调用链里少了一段。** 打开一笔失败订单的调用链，里面有 HTTP 请求的 span、Redis 操作的 span，唯独**没有 gRPC 调用库存服务的 span**。整个请求花了 800 毫秒，但 Redis 那几步加起来才几毫秒。那 800 毫秒花在哪里了？

gRPC 的 span 是在调用真正发出去的时候才开始记录的。没有这个 span，说明调用很可能卡在了“发出去之前”。

**第二条线索：两次 DNS 查询的耗时差别很大。** 调用发出去之前要先做什么？要先把服务名 `inventory-service` 解析成 IP 地址。在订单服务的容器里分别测两种 DNS 查询：

```text
docker compose --env-file .env -f deploy/docker-compose.yml exec -T order-service nslookup -type=A inventory-service
docker compose --env-file .env -f deploy/docker-compose.yml exec -T order-service nslookup -type=TXT _grpc_config.inventory-service
```

- **A 记录**（服务名对应的 IP 地址）：约 0.19 秒就返回了。
- **TXT 记录**（一种存文本的记录）：约 5.2 秒后超时。

为什么要查 TXT？grpc-go 支持一个功能：从 DNS 的 TXT 记录 `_grpc_config.<服务名>` 里读取“服务配置”（比如重试策略）。这个项目没有用这个功能，但 grpc-go 默认还是会去查。Docker 内置的 DNS 不认识这条记录，就把查询转发到外面，外面一直没有回应，于是要等 5 秒才超时。

**第三条线索：读源码确认顺序。** 只知道“TXT 查询慢”还不够，还要确认 gRPC 是不是真的要等它查完才发调用。于是按 go.mod 里固定的版本（v1.84.0），去本机的模块缓存里读 grpc-go 的源码：DNS 解析器先查 A 记录，再查 TXT 记录，**两个都查完**，才把地址交给连接管理器。而调用要等拿到地址才能发出去。这就解释了为什么调用卡住，以及为什么没有 gRPC 的 span。

**为什么闲置一段时间后才出现。** grpc-go 默认在 30 分钟没有调用后，释放连接和解析器。之后再有调用，就要重新解析一遍，又会撞上 5 秒的 TXT 查询。阶段 2 加的“启动时先等连接建立好”只覆盖了启动那一次，管不了之后每次闲置后的重新连接。你那次恰好暂停了 34.5 分钟，超过了 30 分钟。

**修复。** 创建 gRPC 客户端时加上 `grpc.WithDisableServiceConfig()`，告诉它“不要从 DNS 读服务配置”。这样就只查 A 记录，不再查 TXT。一共改了三处：订单服务调用库存服务的客户端、发送调用链数据给 Jaeger 的导出器（它底层也用 gRPC，P3-14 的 5 秒超时就是它）、shopctl 的库存命令。

注意这不是关掉 DNS：服务名照样会被解析成 IP，只是不再读一个没用到的配置。也没有用“把超时从 800 毫秒加长到 6 秒”这种办法来掩盖问题。

**验证。** 为了不用真等 30 分钟，把空闲时间做成了可以配置的（`INVENTORY_GRPC_IDLE_TIMEOUT`），测试时临时改成 10 秒。用同样的脚本、同样的流量对比修复前后：

| 场景 | 修复前 | 修复后 |
|---|---|---|
| 闲置后恢复流量（500 个请求） | 228 个失败或降级，12.3 秒才恢复 | 全部成功，第一个请求 7.6 毫秒 |
| 有流量时重启库存服务 | 18 个失败或降级，1.25 秒恢复 | 17 个失败或降级，1.26 秒恢复 |

重启库存服务这一项没有改善，这是预期之中的：容器重启后 IP 不变，gRPC 直接重连旧地址，不需要重新解析 DNS。剩下的那 1 秒多失败，是库存服务真的停机的时间。

**这次排障能学到的方法：**

1. 先看现象，不急着改参数。把超时加长也能让问题“消失”，但根因还在。
2. 调用链里**缺了什么**，和有什么一样重要。
3. 把一个大问题拆成能单独测量的小问题：DNS 查询分成 A 和 TXT 两种，分别计时。
4. 读源码确认因果，而且读的是项目实际使用的那个版本。
5. 用同样的条件对比修复前后，没有改善的结果也要如实记录。

**补基础：DNS、A 记录和 TXT 记录。** 代码里写的是服务名 `inventory-service`，真正发网络请求要用 IP 地址。把名字翻译成 IP 的系统叫 DNS。DNS 里一个名字可以挂好几种“记录”：A 记录存 IPv4 地址，是最常用的；TXT 记录存一段文本，常用来放配置或验证信息。Docker 给每个容器配了一个内置的 DNS（地址是 127.0.0.11）：容器名、服务名它自己就能回答；不认识的名字，比如 `_grpc_config.inventory-service`，它转给宿主机设置的 DNS 去问，外面一直不回答，就只能等到超时。

> **对照阅读**：
> - 《图解网络》[真实地址查询 —— DNS](https://www.xiaolincoding.com/network/1_base/what_happen_url.html#真实地址查询-dns)：小林讲了一个域名怎样从本地 DNS 服务器一路问到根、顶级域、权威 DNS 服务器，以及浏览器和操作系统的缓存。本节的 Docker 内置 DNS 就扮演“本地 DNS 服务器”的角色。小林没有讲 A、TXT 这些记录类型，看上面的补基础即可。
> - 《系统设计面试题》[对外提供一个 API 服务，客户说请求接口超时了，怎么排查？](https://www.xiaolincoding.com/interview/systemdesign.html#对外提供一个api服务-客户说请求接口超时了-怎么排查)：小林的步骤和本节几乎一一对应：先看 P99 和错误率定范围，用请求 ID 串起日志，用链路追踪拆开耗时。他也专门提到了“DNS 解析慢”，以及“不能只是把所有超时参数都调大”。读完试着用他的框架把本节的排障重讲一遍，面试时很好用。

## 4. 阅读顺序

| 顺序 | 文件 | 重点看什么 |
|---|---|---|
| 1 | [telemetry/metrics.go](../../internal/platform/telemetry/metrics.go) | 每个服务有哪些共用的指标；为什么每个服务一个独立的注册表 |
| 2 | [telemetry/tracing.go](../../internal/platform/telemetry/tracing.go) | `New` 创建了什么；退出时怎样把剩下的 span 发完；`newTraceExporter` 的 DNS 修复 |
| 3 | [telemetry/middleware.go](../../internal/platform/telemetry/middleware.go) | HTTP 请求什么时候开始、什么时候结束计时；没有匹配到路由的请求标签是什么 |
| 4 | [orderservice/dependencies.go](../../internal/shop/app/orderservice/dependencies.go) | 各种插件是怎样装到 MySQL、Redis、gRPC 客户端上的 |
| 5 | [orderservice/observations.go](../../internal/shop/app/orderservice/observations.go) | 怎样在不改业务代码的情况下加计数 |
| 6 | [telemetry/kafka.go](../../internal/platform/telemetry/kafka.go)、[events/publisher.go](../../internal/shop/events/publisher.go)、[events/consumer.go](../../internal/shop/events/consumer.go) | 调用链信息怎样写进消息头、又怎样被读出来 |
| 7 | [logging/logging.go](../../internal/platform/logging/logging.go) 的 `FromContext` | 日志里的 trace_id 为什么要每次从当前 span 取 |
| 8 | [prometheus/rules/shop.yml](../../deploy/prometheus/rules/shop.yml) | 5 条告警规则 |
| 9 | [alertsink/handler.go](../../internal/platform/alertsink/handler.go) | 收到告警以后做了什么 |
| 10 | [deploy/README.md](../../deploy/README.md) 及各子目录的 README | 每个监控组件的配置，以及它们怎样连成一套 |

### 4.1 代码走读：一次下单请求怎样变成指标

假设第 2.2 节的流量里，有一次 `POST /orders` 返回了 201，用时 0.037 秒（数字是示意）：

| 顺序 | 在哪里 | 做了什么 |
|---|---|---|
| 1 | `HTTPMiddleware`（[telemetry/middleware.go](../../internal/platform/telemetry/middleware.go)） | 记下开始时间，调用 `ctx.Next()`，把请求交给后面的下单处理函数 |
| 2 | 下单处理函数 | 照常下单，返回 201 |
| 3 | 回到 `HTTPMiddleware` | `ctx.FullPath()` 取到的是路由模板 `/orders`，不是请求的原始地址；没有匹配到路由时它是空字符串，改记成 `unmatched`。方法 `POST` 在已知列表里，原样保留；不认识的方法统一记成 `OTHER`。状态码是 `201` |
| 4 | 同上 | `metrics.HTTP.WithLabelValues("POST", "/orders", "201").Observe(0.037)`：0.037 秒不超过 0.05，所以 `le="0.05"` 以及更大的每个区间计数都加 1；同时总耗时 `_sum` 加 0.037，总次数 `_count` 加 1 |
| 5 | Prometheus | 每 10 秒访问一次 `/metrics`，把这些计数存成时间序列。`service="order-service"` 这个标签是服务创建注册表时统一加上的（[metrics.go](../../internal/platform/telemetry/metrics.go) 里的 `WrapRegistererWith`），所以每条序列都带着它 |

一个直方图在 `/metrics` 里会展开成很多行，大致是这样（数字是示意）：

```text
http_server_request_duration_seconds_bucket{method="POST",route="/orders",service="order-service",status="201",le="0.025"} 812
http_server_request_duration_seconds_bucket{method="POST",route="/orders",service="order-service",status="201",le="0.05"} 1530
...
http_server_request_duration_seconds_bucket{method="POST",route="/orders",service="order-service",status="201",le="+Inf"} 1600
http_server_request_duration_seconds_sum{method="POST",route="/orders",service="order-service",status="201"} 47.3
http_server_request_duration_seconds_count{method="POST",route="/orders",service="order-service",status="201"} 1600
```

注意 `le` 是“小于等于”：`le="0.05"` 的 1530 已经包含了 `le="0.025"` 的 812。区间计数是**累计**的，最后的 `+Inf` 就等于总次数。第 5.2 节算 P99，用的就是这种累计计数。

### 4.2 代码走读：trace 怎样跟着消息走到 order-worker

接着第 2.4 节那笔订单。它的请求头带着 `traceparent`，最后两位是 `01`，表示要记录：

| 顺序 | 在哪里 | 做了什么 |
|---|---|---|
| 1 | `Publish`（[events/publisher.go](../../internal/shop/events/publisher.go)） | 以当前 context 里的 span 为父节点，开一个 `order.created publish` span，类型是 Producer（生产者）。trace ID 沿用这笔订单的 |
| 2 | 同上：`Propagator.Inject(ctx, &headers)` | Propagator 从 context 取出当前 span，调用 `headers.Set("traceparent", "00-<trace ID>-<publish span 的 ID>-01")`。`headers` 就是第 6.1 节那种加了方法的切片 `KafkaHeaders` |
| 3 | `writer.WriteMessages` | 消息带着这个消息头写进 Kafka，键是订单号 |
| 4 | order-worker 的 `process`（[events/consumer.go](../../internal/shop/events/consumer.go)） | 读到消息，把 `message.Headers` 转成 `KafkaHeaders`；`Propagator.Extract` 调用 `Get("traceparent")`，把 trace ID 和父 span ID 放进 context |
| 5 | 同上 | 开 `order.created process` span，类型是 Consumer（消费者）。它的父节点是第 1 步的 publish span，trace ID 不变，所以在 Jaeger 里接在同一条链上 |
| 6 | 之后的 SQL 和日志 | 都从这个 context 往下传：SQL 插件创建子 span，日志自动带上同一个 trace_id（第 5.5 节） |

如果这笔订单没被选中记录，最后两位是 `00`，第 2 步写进消息头的也是 `00`。各服务的采样方式是“跟随父节点”（`parentbased_traceidratio`，由 [docker-compose.yml](../../deploy/docker-compose.yml) 里的环境变量设置），order-worker 看到 `00` 也就不记录。所以一条链要么都记、要么都不记（第 5.7 节）。

### 4.3 代码走读：一条告警从规则到通知

以第 7 节实验一为例：200 QPS 下停掉 order-worker。下面用的是阶段 3 当时的[告警配置](https://github.com/Li-Nepenthe/opsdoctor/blob/482aa22/deploy/alertmanager/alertmanager.yml)。阶段 5 以后，接收方从 alertsink 换成了医生，分组方式也改了，但流程相同。

| 顺序 | 在哪里 | 发生了什么 |
|---|---|---|
| 1 | Kafka exporter → Prometheus | 每 10 秒采集一次消费组的积压量 `kafka_consumergroup_lag` |
| 2 | 告警规则（[rules/shop.yml](../../deploy/prometheus/rules/shop.yml) 里的 `ConsumerLagHigh`） | Prometheus 每 10 秒算一次 `sum(kafka_consumergroup_lag{consumergroup="order-worker",topic="order.created"}) > 1000`。积压第一次超过 1000 时，告警进入 **pending**（等待），开始计时 |
| 3 | 同上 | 接下来 2 分钟（`for: 2m`）里每次计算都仍超过 1000，告警变成 **firing**，发给 Alertmanager。中间只要有一次不满足，就退回去重新计时 |
| 4 | Alertmanager | 按告警名和服务分组，先等 5 秒（`group_wait`），让同一组的告警凑齐再发；然后 POST 到 `http://alertsink:8091/api/v1/alerts`，请求头带 `Authorization: Bearer <令牌>`。令牌从容器里的临时文件读取，不写在配置里 |
| 5 | alertsink（[alertsink/handler.go](../../internal/platform/alertsink/handler.go)） | 先核对令牌，不对就返回 401（第 7 节实验三）；再解析 JSON，状态只能是 firing 或 resolved；每条告警写一行日志“收到告警通知”，返回 204 |
| 6 | 恢复以后 | 积压降到 1000 以下，告警恢复。配置了 `send_resolved: true`，所以会再发一次 resolved 通知 |

把时间加起来，就能理解第 3.3 节那次为什么是 167 秒。规则旁边的注释估计，默认流量下大约每秒 40 笔订单，积压涨到 1000 要二十多秒；之后还要再持续 2 分钟才 firing；采集和计算各有最多 10 秒的间隔，Alertmanager 还要等 5 秒。加在一起，大约两分半到三分钟。

**补基础：比较令牌，为什么要先算哈希、再用 `subtle.ConstantTimeCompare`。** 普通的字符串比较，遇到第一个不同的字节就返回。攻击者可以反复猜，测量每次响应是快是慢，一个字节一个字节地猜出令牌，这叫**时序攻击**。`ConstantTimeCompare` 不管哪里不同，都把整段比完，耗时一样。但它在两段长度不同时会直接返回，所以先把双方都算成固定 32 字节的 SHA-256 再比较，长度也不会泄露。

## 5. 关键设计与为什么

### 5.1 三种指标类型

| 类型 | 特点 | 本项目的例子 |
|---|---|---|
| **Counter（计数器）** | 只会增加，程序重启后归零 | `rpc_client_retries_total`：查库存的重试次数；`idempotency_requests_total`：幂等处理次数 |
| **Gauge（仪表）** | 可增可减，表示“当前是多少” | `circuit_breaker_state`：熔断器状态（0 关闭、1 半开、2 打开）；连接池里正在使用的连接数 |
| **Histogram（直方图）** | 把每个值分到不同的区间里计数，用来估计分布 | `http_server_request_duration_seconds`：HTTP 请求的耗时 |

Counter 的值本身没什么意义（“从启动到现在一共重试了 37 次”），有意义的是它增长的速度，所以查询时通常用 `rate()`：每秒增加了多少。程序重启后 Counter 归零，`rate()` 会自动处理这种情况。

**补基础：rate 和 increase。** `rate(x[2m])` 是“最近 2 分钟里平均每秒增加多少”；`increase(x[2m])` 是“最近 2 分钟一共增加了多少”，大约等于 `rate` 乘以 120。两者都只用窗口里实际采到的点来算，再按比例推算到整个窗口，所以 `increase` 的结果经常不是整数。阶段 9 就碰到过这种情况：外部 Demo 的业务指标大约每 60 秒才更新一次，2 分钟的窗口里只有两个点，`increase` 把这一分钟的增量按比例放大成两倍，查出来的几乎都是偶数。所以不能把 `increase` 的结果直接当成“发生了几次”。

### 5.2 P99 是怎么从直方图算出来的

Histogram 不保存每一个请求的耗时，而是预先划分好一组区间，只记“落在每个区间里的有多少个”。本项目的区间边界是：1ms、5ms、10ms、25ms、50ms、100ms、250ms、500ms、1s、2s、5s、10s。

假设 100 个请求里，95 个在 100ms 以内，99.5 个在 250ms 以内，那么第 99 个请求（P99）一定落在 100ms 到 250ms 之间。具体是多少，Prometheus 假设请求在这个区间里均匀分布，按比例估算出来。所以 **P99 是一个估计值**，精度取决于区间划分得有多细。

查询写法：

```promql
histogram_quantile(0.99,
  sum by (le) (rate(http_server_request_duration_seconds_bucket{service="order-service",route="/orders",method="POST"}[2m]))
)
```

从里往外读：`rate(...[2m])` 算出最近 2 分钟每个区间每秒增加了多少；`sum by (le)` 把多个实例的同一个区间加在一起（`le` 是区间的上界）；`histogram_quantile(0.99, ...)` 估算出 P99。

**不能把几个实例各自的 P99 平均。** 正确的做法就是上面这样：先把各实例的区间计数加起来，再算 P99。

**补基础：P99 是什么意思。** 把一段时间里所有请求的耗时从小到大排好，排在 99% 位置上的那个值就是 P99。P99 是 300 毫秒，意思是 99% 的请求在 300 毫秒以内完成，最慢的那 1% 超过它。只看平均值，会被大量的快请求“拉低”：平均 20 毫秒的接口，也可能有 1% 的请求等了好几秒，而用户抱怨的正是这部分。所以看板和告警用 P99。

**算一遍。** 假设最近 2 分钟一共有 1000 个下单请求，累计区间计数是这样（数字是示意）：

| 区间上界 `le` | 累计请求数 |
|---|---:|
| 0.05 秒 | 600 |
| 0.1 秒 | 900 |
| 0.25 秒 | 985 |
| 0.5 秒 | 995 |
| 1 秒 | 1000 |

1. P99 是第 0.99 × 1000 = 990 个请求。
2. 从上往下找第一个累计数不少于 990 的区间，是 0.5 秒那一行（995）。上一行是 985，所以第 990 个请求落在 0.25 到 0.5 秒之间，是这个区间 10 个请求里的第 5 个。
3. Prometheus 假设这 10 个请求在区间里均匀分布，按比例算：0.25 + (0.5 − 0.25) × 5 ÷ 10 = 0.375 秒。

所以 P99 约为 375 毫秒。真实值可能是 0.25 到 0.5 秒之间的任何数，这就是“估计值”的意思；区间分得越细，估计越准。

**为什么不能平均，也算一遍。** 实例 A 处理了 100 个请求，全都是 10 毫秒，P99 是 10 毫秒。实例 B 也是 100 个，其中 2 个用了 2 秒，P99 是 2 秒。两个 P99 平均，大约是 1 秒。可是把 200 个请求放在一起，慢的只有 2 个，第 198 个（0.99 × 200）仍然是 10 毫秒的快请求，真实的 P99 还是 10 毫秒左右，和平均出来的结果差了一百倍。

### 5.3 标签基数：为什么订单号不能做标签

指标可以带标签，比如 `http_server_request_duration_seconds{service="order-service", route="/orders", method="POST", status="201"}`。**每一种不同的标签组合，Prometheus 都要单独存一条时间序列。**

服务只有几个，路由只有十几个，组合起来也就几百条，没问题。但如果把订单号当标签，每个新订单都会产生一条新的时间序列，200 多万个订单就是 200 多万条，Prometheus 的内存和存储很快就会撑爆。这个问题叫**高基数**。

所以：

- 路由标签用模板：`/orders/1`、`/orders/2` 都记为 `/orders/:id`。
- 没有匹配到任何路由的请求（比如乱写的地址），统一记为 `unmatched`，否则别人随便访问不同的地址就能制造出无数条序列。
- 要查某一笔具体的订单，去日志或调用链里查，不要用指标。

### 5.4 调用链怎样跨服务传递

先分清三个概念：

- **trace**：一次请求的完整路径，有一个全局唯一的 trace ID。
- **span**：路径上的一步操作，比如一次 SQL、一次 gRPC 调用。每个 span 记录了开始时间、结束时间，以及它的“父 span”是谁。
- **context**：Go 里沿着函数调用往下传的对象，当前的 span 就放在它里面。

在一个服务内部，span 跟着 context 往下传就行。**跨服务时**，要把 trace ID 和当前 span ID 放进请求里，对方收到后取出来，作为自己 span 的父节点：

- **HTTP 和 gRPC**：有现成的插件，自动在请求头里写入、读出 `traceparent`。
- **Kafka**：kafka-go 没有插件，所以项目自己写了 `KafkaHeaders`。订单服务发消息前，把 `traceparent` 写进消息头；order-worker 收到后，从消息头里读出来，再创建自己的 span。

如果 Kafka 这一段不传，order-worker 的处理就会变成一条新的、孤立的 trace，下单和消费就串不起来了。

具体的传递过程，见第 4.2 节。

> **对照阅读**：《Golang面试题》[6.2 Go 语言的 context 有什么作用？](https://www.xiaolincoding.com/interview/golang.html#_6-2-go语言的context有什么作用)：小林总结了 context 的三个用途：超时控制、传递取消信号、传递请求级的数据，并提到第三种“在分布式链路追踪中特别有用”。本节放在 context 里的当前 span，就是第三种用法。

### 5.5 日志里的 trace_id

每条日志都会自动带上 `trace_id` 和 `span_id`，日志和调用链就能对上了。

细节在于：日志包每次写日志时，都会从 context 里取**当前的** span。一次请求进来时有一个入口 span，执行 SQL 时又会创建子 span。如果一直用入口 span 的 ID，SQL 出错时写的日志就会指向错误的位置。trace_id 在整条链上都一样，span_id 则精确指向具体是哪一步。

### 5.6 告警：只写症状

5 条告警规则描述的都是症状：下单慢了、错误多了、消息积压了、goroutine 一直在涨、服务采集不到了。没有一条写的是“数据库缺索引”这样的原因。

因为同一个症状可能有很多原因。消息积压可能是 order-worker 停了，也可能是数据库变慢了，或者消息格式坏了。告警只负责发现“有问题”，找出原因是医生的工作。

每条规则还有一个持续时间（`for`），比如积压要持续 2 分钟才报。这样能过滤掉偶尔的抖动，代价是发现得晚一点。

**补基础：告警的三个状态。** 规则每 10 秒算一次。条件不满足时是 inactive（没事）；第一次满足，进入 pending（等待），开始计 `for` 的时间；在 `for` 规定的时间里每次都满足，才变成 firing（报警），发给 Alertmanager。等待期间只要有一次不满足，就回到 inactive，下次满足时重新计时。完整的过程见第 4.3 节。

> **对照阅读**：
> - 《图解网络》[HTTP 常见的状态码有哪些？](https://www.xiaolincoding.com/network/2_http/http_interview.html#http-常见的状态码有哪些)：4xx 表示客户端发来的请求有问题，5xx 表示服务器自己处理出错。错误率告警 `HTTPErrorRateHigh` 只数 5xx：参数填错的 400、地址不存在的 404 是客户端的问题，不代表服务坏了。
> - 《Golang面试题》[10.5 Go 语言在什么情况下会发生内存泄漏？](https://www.xiaolincoding.com/interview/golang.html#_10-5-go语言在什么情况下会发生内存泄漏)、[10.6 发生了内存泄漏如何定位和优化？](https://www.xiaolincoding.com/interview/golang.html#_10-6-go语言发生了内存泄漏如何定位和优化)：小林说 goroutine 泄漏是最常见的一种，定位时先看 goroutine 数量的曲线，再用 pprof 看它们卡在哪里。`GoroutinesGrowing` 告警做的就是“看曲线”这一步；用 pprof 查原因，是阶段 5 以后医生的工具。

### 5.7 采样和资源

- **只记录十分之一的请求。** 每个请求都记录的话，本机的 Jaeger 和网络开销都太大。但一旦某个请求被选中记录，它经过的所有服务都会跟着记录（“跟随父节点”），不会出现一条链只记了一半的情况。
- **Jaeger 只在内存里存最近 5000 条 trace。** 本机学习够用。代价是旧的会被挤掉：拿着一个 trace ID，不一定还能查到。
- **每个容器都设了内存上限。** MySQL 原来的上限是 768 MiB，实测已经用到 98.8%，返工时调到了 1.5 GiB。

### 5.8 每个服务一个独立的指标注册表

Prometheus 的 Go 库提供了一个全局默认的注册表，直接用能省几行代码。项目没有用它，而是每个服务在启动时创建自己的注册表，再显式地传给需要的组件。

原因是测试：如果用全局注册表，同一个测试进程里创建两个服务时，同名的指标会被注册两次，直接报错；测试之间也会互相影响。显式传递多写几行代码，但每个测试都是独立的，依赖关系也一目了然。

### 5.9 用“包装”的方式加计数

要统计缓存命中了多少次，最直接的办法是去改 product 包，在缓存代码里加一行计数。但那样业务包就要依赖监控库了。

项目的做法是在组装代码里（observations.go）给缓存“包一层”：包装对象实现和原来一样的接口，每次调用时先调用原来的缓存，再记一次数。业务代码完全不知道有计数这回事。这就是**装饰器模式**，第 6.2 节有例子。

有一点要注意：缓存为了防止重复回源，有时会查两次缓存，这两次都会被计数。所以“缓存查询次数”不等于“HTTP 请求次数”。

## 6. 本阶段用到的 Go 写法

每个例子都是能独立运行的小程序。在仓库外面新建一个空目录，保存成 `main.go`，运行 `go run main.go`。

### 6.1 给切片类型加方法

```go
package main

import "fmt"

// Headers 是一个新类型，底层是字符串切片。可以给它加方法。
type Headers []string

// Set 用指针接收者：append 可能会换一块新的底层数组，必须把结果写回原来的变量。
func (headers *Headers) Set(value string) {
	*headers = append(*headers, value)
}

func main() {
	headers := Headers{}
	headers.Set("traceparent")
	headers.Set("tracestate")
	fmt.Println(headers, len(headers)) // [traceparent tracestate] 2
}
```

项目里的 `KafkaHeaders` 就是这样定义的：`type KafkaHeaders []kafka.Header`。给它加上 `Get`、`Set`、`Keys` 三个方法后，它就满足了 OpenTelemetry 要求的接口，调用链信息就能写进 Kafka 消息头了。

为什么 `Set` 要用指针接收者？`append` 在容量不够时会分配一块新的数组，返回新的切片。如果接收者是值（`headers Headers`），改的只是一份拷贝，调用方的变量不会变。

> **对照阅读**：《Golang面试题》[2.4 slice 作为函数参数传递，会改变原 slice 吗？](https://www.xiaolincoding.com/interview/golang.html#_2-4-slice作为函数参数传递-会改变原slice吗)：小林的结论是，传 slice 时拷贝的是一个小结构体（指向底层数组的指针、长度、容量）。改元素会影响外面；但 `append` 改了长度，甚至换了底层数组，外面就看不到了。要让外面也变，就返回新的 slice，或者传指针。值接收者就相当于“传一份拷贝”，所以 `Set` 要用指针接收者。扩容时为什么会换数组，见 [2.2 slice 是怎么扩容的？](https://www.xiaolincoding.com/interview/golang.html#_2-2-go语言里slice是怎么扩容的)

### 6.2 装饰器：包一层，加点东西

```go
package main

import "fmt"

type Cache interface {
	Get(key string) string
}

type redisCache struct{}

func (redisCache) Get(key string) string { return "商品" + key }

// countingCache 把一个 Cache 嵌入进来，只重写 Get：先调用原来的 Get，再记一次数。
type countingCache struct {
	Cache
	count *int
}

func (cache countingCache) Get(key string) string {
	result := cache.Cache.Get(key)
	*cache.count++
	return result
}

func main() {
	count := 0
	var cache Cache = countingCache{Cache: redisCache{}, count: &count}
	cache.Get("1")
	cache.Get("2")
	fmt.Println("查了", count, "次") // 查了 2 次
}
```

`countingCache` 和 `redisCache` 实现的是同一个接口，所以用到 `Cache` 的业务代码不需要任何修改。项目里的 `observedCache`、`observedProducts`、`observedIdempotency` 都是这样做的，只是把 `count++` 换成了给 Prometheus 指标加 1。

> **对照阅读**：《Golang面试题》[7.4 Go 语言 interface 有哪些应用场景？](https://www.xiaolincoding.com/interview/golang.html#_7-4-go语言interface有哪些应用场景)：小林列的第一条是“依赖注入和解耦”，还提到 Web 框架的中间件也靠接口来扩展。装饰器能成立，前提就是业务代码只依赖接口，不关心背后是原来的缓存，还是包了一层的缓存。

### 6.3 采集时再读取当前值

```go
package main

import "fmt"

func main() {
	state := 0 // 假设这是熔断器的状态
	read := func() int { return state }

	fmt.Println(read()) // 0
	state = 2           // 熔断器打开了
	fmt.Println(read()) // 2：每次调用都读当前的值
}
```

熔断器状态的指标用的是 `GaugeFunc`：注册时交给 Prometheus 一个函数，Prometheus 每次来采集时调用它，读到的总是当前的状态，不用在状态变化时手动更新指标。

注意：这个例子只有一个 goroutine。项目里读取的是熔断器库提供的 `State()` 方法，它内部有锁保护。闭包本身并不会让并发读写变得安全。

## 7. 动手实验

每个实验做完都要**恢复原状**，再做下一个。不要删除数据卷。

### 实验一：让消息积压，等告警

1. 开 200 QPS 的流量（第 2.2 节的命令，把 20 改成 200），等看板稳定。
2. 停掉 order-worker：`docker compose --env-file .env -f deploy/docker-compose.yml stop order-worker`
3. 在看板上看“订单消费者积压”面板，数字会一直往上涨。
4. 大约两三分钟后，查看告警接收器的日志：

   ```powershell
   docker compose --env-file .env -f deploy/docker-compose.yml logs --tail 20 alertsink
   ```

   你应该看到一条 `"alertname":"ConsumerLagHigh"`、`"status":"firing"` 的记录。可能还会更早看到一条 `ServiceUnavailable`：order-worker 停了，它的管理端口也采集不到了。
5. 恢复：`docker compose --env-file .env -f deploy/docker-compose.yml start order-worker`，然后暂停流量。等积压归零后再看 alertsink 的日志，应该能看到 `"status":"resolved"`。

告警多快到达取决于流量大小：规则要求积压超过 1000 条、再持续 2 分钟。流量小的话，积压涨得慢，告警也会晚到。实测 200 QPS 时，大约 2.5 到 3 分钟。

### 实验二：停掉库存服务，看指标的变化

只查商品，不下单，避免产生“待核对”的订单。

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml stop inventory-service
1..15 | ForEach-Object { (Invoke-RestMethod http://127.0.0.1:8081/products/1).data.stock.status }
```

你应该看到 `unavailable`（降级）。去看板上看熔断器状态面板，应该变成 2（打开）。

恢复：`docker compose --env-file .env -f deploy/docker-compose.yml start inventory-service`。再查几次商品，熔断器经过半开（1）回到关闭（0）。

> **对照阅读**：《分布式面试题》[限流、熔断和降级有什么区别？](https://www.xiaolincoding.com/interview/cap.html#限流、熔断和降级有什么区别)：熔断器的关闭、打开、半开三个状态，正好对应看板上的 0、2、1。半开时只放少量请求去试探，成功了才回到关闭；库存服务恢复后再查几次商品，就是在给半开状态送试探请求。商品查询显示 `unavailable` 而不是直接报错，就是小林说的“降级”。

### 实验三：用错误的令牌推送告警

```powershell
try {
    Invoke-WebRequest http://127.0.0.1:8091/api/v1/alerts -Method Post -Headers @{Authorization='Bearer deliberately-wrong'} -ContentType 'application/json' -Body '{}'
} catch {
    $_.Exception.Response.StatusCode
}
```

你应该看到 `Unauthorized`（401），alertsink 的日志里也不应该出现“收到告警通知”。不要为了做实验把真实的令牌写进命令里，那样它会留在命令历史中。

### 实验四：访问不存在的地址，看指标标签

```powershell
foreach ($path in @('/not-a-route-1', '/not-a-route-2')) {
    try { Invoke-WebRequest ('http://127.0.0.1:8081' + $path) } catch { $_.Exception.Response.StatusCode }
}
```

两个请求都应该返回 404。然后在 Prometheus 里查询：

```promql
http_server_request_duration_seconds_count{service="order-service", route="unmatched"}
```

两个不同的地址都记在同一个 `route="unmatched"` 标签下，没有产生两条新的序列（第 5.3 节）。

## 8. 常见误区

- **“看指标就能知道每笔订单的情况。”** 指标是汇总的数字。具体某一笔订单，要看日志和调用链。
- **“Counter 的值变小了，说明出错了。”** 程序重启后 Counter 会归零，这是正常的。看增长速度要用 `rate()`。
- **“缓存命中率就是 HTTP 请求的命中率。”** 统计的是缓存查询的次数。一个 HTTP 请求可能查好几次缓存，也可能一次都不查（比如订单列表）。
- **“有 trace ID 就一定能在 Jaeger 里查到。”** 可能没被采样，可能还没发到 Jaeger，可能已经被新的 trace 挤掉了，也可能发送时出了错。
- **“启动日志里没有 trace_id，说明日志关联坏了。”** 启动时还没有请求，自然没有 trace_id。
- **“容器都是 running，监控就通了。”** 要实际查一次数据。Jaeger 和 Grafana 的接口不兼容，就是实际查询时才发现的。
- **“修好了 DNS 问题，就再也不会超时了。”** 网络真的断了、库存服务真的挂了，照样会超时；扣库存超时后结果不确定的问题也依然存在。

## 9. 必答问题

先自己写下答案，再展开最后的参考要点对照。

1. Counter、Gauge、Histogram 有什么区别？各举一个本项目里的例子。
2. P99 是怎样从直方图算出来的？为什么只是估计值？为什么不能把几个实例的 P99 平均？
3. trace、span、context 是什么关系？调用链信息怎样跨 HTTP、gRPC 和 Kafka 传递？
4. 为什么告警规则写“消息积压”，而不写“数据库缺索引”？
5. 为什么订单号、trace ID 不能做指标的标签？
6. `up=1` 和 `mysql_up=1` 有什么区别？
7. 同一笔订单的消息被消费了两次，一次正常处理、一次判定为重复，销量和相关的指标分别会怎样变化？
8. 拿着一个 trace ID 却在 Jaeger 里查不到，可能有哪些原因？
9. 讲一讲 DNS TXT 那次排障：现象是什么、线索是什么、根因是什么、怎么修、怎么验证？
10. 为什么重启库存服务的恢复时间，修复前后没有变化？

## 10. 本阶段新术语

| 术语 | 解释 |
|---|---|
| 可观测性 | 通过系统输出的数据（指标、调用链、日志）判断它内部发生了什么 |
| 指标 | 一段时间内的数字，比如每秒请求数、P99 耗时 |
| Counter | 只增不减的计数器，程序重启后归零 |
| Gauge | 可增可减的当前值，比如正在使用的连接数 |
| Histogram | 按区间计数的直方图，用来估计耗时分布 |
| 标签 / 标签基数 | 指标的附加维度 / 不同标签组合的数量，太多会撑爆存储 |
| 采集（scrape） | Prometheus 定时去访问 `/metrics`，取回当前的指标 |
| exporter | 帮 MySQL、Redis 这类组件把状态翻译成 Prometheus 格式的程序 |
| trace / span | 一次请求的完整路径 / 路径上的一步操作 |
| traceparent | W3C 规定的请求头，用来在服务之间传递 trace ID 和父 span ID |
| 采样 | 只记录一部分请求的调用链，节省资源 |
| Provider / Propagator | 负责创建 span 的对象 / 负责在服务之间写入和读出调用链信息的对象 |
| OTLP | OpenTelemetry 定义的、把观测数据发给后端的协议 |
| LogQL / PromQL | Loki 的日志查询语言 / Prometheus 的指标查询语言 |
| provisioning | Grafana 启动时根据配置文件自动创建数据源和看板 |
| 症状告警 | 告警只描述看得到的现象，不推断原因 |
| firing / resolved | 告警正在发生 / 已经恢复 |
| 装饰器 | 用实现同一接口的对象把原对象包起来，在调用前后加上额外的逻辑 |
| DNS A / TXT 记录 | 域名对应的 IPv4 地址 / 域名附带的一段文本 |

<details>
<summary>必答问题参考要点（先自己回答）</summary>

1. Counter 只增不减，比如重试次数；Gauge 是当前值，可增可减，比如熔断器状态、连接数；Histogram 按区间计数，比如 HTTP 请求耗时。
2. 先算每个区间的增长速度，把各实例同一个区间的计数加起来，再找到 99% 所在的区间，在区间内按比例估算。因为没有保存每个请求的原始耗时，只知道落在哪个区间，所以是估计值。百分位不能相加或平均，必须先合并区间计数，再重新计算。
3. trace 是一次请求的完整路径，span 是其中一步，context 携带当前的 span，在函数之间往下传。HTTP 和 gRPC 由插件自动在请求头里传递 `traceparent`；Kafka 由项目自己把 `traceparent` 写进消息头，消费者读出来作为父节点。
4. 同一个症状可能有很多原因。告警只负责发现问题、提供调查的入口，把猜测写进告警，等于把没有验证的原因当成了结论。
5. 每一种不同的标签组合都是一条单独的时间序列。订单号、trace ID 每个都不一样，会让序列数量无限增长，撑爆存储。具体某一笔的情况，去日志和调用链里查。
6. `up=1` 说明 Prometheus 能访问到 exporter；`mysql_up=1` 说明 exporter 能连上 MySQL 并读到数据。前者正常、后者为 0，说明问题出在 exporter 和 MySQL 之间。
7. 销量只加一次。`worker_messages_total` 里，处理成功的那次记为 processed，判定为重复的那次记为 duplicate，各加 1（前提是位点都提交成功了；提交失败会记为 failed）。
8. 这个请求没有被采样；span 还在发送的路上；Jaeger 重启过或者已经被新的 trace 挤掉了；只有一个外部传进来的父 span，但那一段从来没有发给 Jaeger；或者发送真的出了错。
9. 现象：闲置 34.5 分钟后恢复流量，库存调用全部超时，熔断器打开。线索：调用链里没有 gRPC 的 span；DNS 的 A 记录查询很快，TXT 记录查询要 5 秒才超时。根因：grpc-go 解析地址时会查 A 和 TXT，两个都查完才发调用；连接闲置 30 分钟后会被释放，再次调用时要重新解析。修复：创建 gRPC 客户端时加上 `WithDisableServiceConfig()`，不再查 TXT。验证：把空闲时间调成 10 秒，用同样的流量对比，失败从 228 个降到 0 个。
10. 容器重启后 IP 不变，gRPC 直接重连原来的地址，不需要重新解析 DNS，所以 TXT 查询本来就不影响这个场景。剩下的 1 秒多失败，是库存服务真的停机的时间。

</details>

### 面试追问：面试官接着会问

1. Prometheus 是拉数据还是等推送？各有什么好处？
2. 一个接口变慢了，你怎么一步步定位？
3. 调用链只记录 10%，会不会正好漏掉出错的请求？
4. 告警太多、太吵怎么办？
5. 日志、指标、调用链，为什么三样都要？只要日志行不行？

<details>
<summary>答题要点</summary>

1. 本项目是拉：Prometheus 定时访问各服务的 `/metrics`。好处是它自己知道目标挂没挂（`up=0`），服务也不需要知道监控系统在哪。推是服务主动发出去，适合运行时间很短的任务，或者监控拉不到的网络环境；阶段 9 的 OTel Demo 就是通过 OTLP 把指标推给 Prometheus 的。
2. 先看指标，确认是哪个接口、从什么时候开始慢（P99、错误率）；再找一笔同一时段的慢请求，用调用链看慢在哪一步；再用它的 trace_id 查日志，看那一步发生了什么；最后用数据库、网络这些底层数据验证猜想。第 3.5 节的 DNS TXT 排障就是这样一步步查出来的。
3. 会。现在的做法叫“头部采样”：请求一开始就决定记不记，出错的请求也可能没被选中。改进办法是“尾部采样”：先把整条链缓存起来，等请求结束、知道它出错了或者很慢，再决定保留。代价是采集器要多缓存很多数据。
4. 只对用户能看到的症状告警，不对猜测的原因告警；用 `for` 要求异常持续一段时间；Alertmanager 按告警名和服务分组，先等 5 秒凑齐再一起发；重复提醒之间有间隔。阶段 5 以后，医生还会把同一场故障的多次通知合进同一个诊断。
5. 指标便宜，适合看全局趋势和触发告警，但看不到某一笔请求；调用链能看一笔请求慢在哪一步，但有采样；日志细节最多，但量大、难聚合。三者用 trace_id 串起来，就能从“整体有问题”一路走到“这一笔为什么错”。

</details>

学完后，在[进度看板](../../进度.md)上勾选阶段 3 的“学习验收”。
