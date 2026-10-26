# internal：项目的全部实现代码

除了程序入口（`cmd/`）和自动生成的代码（`gen/`），项目的 Go 代码都在这里。

`internal` 这个目录名对 Go 有特殊含义：放在它下面的包，只有本仓库的代码能导入，别的项目导入不了。这样项目内部的代码可以随时调整，不用担心影响外人。

## 里面有什么

| 目录 | 放什么 |
|---|---|
| [platform](platform/README.md) | 通用基础设施：配置、日志、数据库连接、HTTP 中间件等。不认识任何业务概念 |
| [shop](shop/README.md) | 靶场：库存、订单、商品、销量等电商业务 |
| [doctor](doctor/README.md) | 医生：登录、按系统接告警、保存任务与只读取证工具 |
| [loadgen](loadgen/doc.go) | 压测工具的实现：生成流量、控制速率、统计结果 |
| [chaos](chaos/README.md) | 宿主机实验工具：改动真实原因、恢复环境、检查症状和健康 |

## 谁能依赖谁

- `shop` 和 `doctor` 都可以使用 `platform`。
- `platform` 不能使用 `shop` 或 `doctor`，因为它是给所有人用的通用代码。
- `shop` 和 `doctor` 之间不能互相导入。医生只能像外人一样，通过监控数据和只读账号观察靶场。

这些规则由 [archcheck](../tools/archcheck/doc.go) 在 CI 里自动检查，违反了会报错。

## 从哪里开始读

不要从这里逐个目录往下翻。先在 [cmd](../cmd/README.md) 里挑一个程序，从它的 `main.go` 进来，再跟着函数调用往里读。
