# AGENTS.md

本文件写给在本仓库工作的 AI 编码助手（Astra 等）。**每次开始工作前先读本文件。**

## 1. 以什么为准

- [项目计划书.md](项目计划书.md) 是唯一的需求来源。第一次进入仓库、或者开始一个新阶段之前，先完整读一遍。
- 本文件只是工作流程和规则摘要。如果和计划书冲突，以计划书为准，并告诉用户哪里冲突了。
- [进度.md](进度.md) 记录当前做到了哪里。

## 2. 开工前

1. 读 `进度.md`，确认当前阶段。
2. 确认上一阶段已经得到用户同意：用户已勾选“学习验收”，或者在对话里明确说了“进入下一阶段”。**没有得到同意，不得开始新阶段。**
3. 从 `main` 切出本阶段的分支，分支名用 `phase-NN-<短名>`（短名见下面第 5 节）。

## 3. 不可违反的规则

| 规则 | 计划书位置 |
|---|---|
| 几乎所有地方都要写像人写的中文注释，代码不能晦涩；注释是讲解，不是检查清单 | 第 1 节前提一、第 3 节 |
| 说明分三层：目录（`README.md` / `doc.go`）、文件（文件说明）、代码 | 第 3.1 节、第 5.1.8 节 |
| 学习材料要有“开发过程回放”，只写真实发生的过程 | 第 4.2 节 |
| doc.go、README 和学习材料写给初学者：先讲问题、术语先解释、一段只讲一件事；Go 示例必须能直接运行。范例是阶段 0–3 的学习材料 | 第 3.6 节 |
| 一次只做一个阶段，做完必须停下等用户学习 | 第 1 节前提二、第 2.2 节 |
| 所有数字来自真实运行，不宣称没实现的能力，不替用户勾选学习验收 | 第 1 节前提三 |
| 代码按规定的目录和分层放置 | 第 5.1 节 |
| 各部分之间的接口格式 | 第 5.8 节 |
| 明确不做的事 | 第 2.4 节 |
| 密钥和密码只放在环境变量里 | 第 2.7 节 |
| 医生的权限边界靠代码和账号权限保证，不靠提示词；不给医生超出该节的权限 | 第 5.5.9 节 |

另外两条：

- **这是公开仓库**，任何人都能看到。API Key、密码、令牌绝不能提交；`.env.example` 里只放演示用的假值。
- CI 的运行环境要写死版本（例如 `ubuntu-24.04`），不要用 `ubuntu-latest`。

## 4. 阶段完成时必须做的事（按顺序）

1. 跑完全部检查：`gofmt`、`go vet`、`staticcheck`、`commentcheck`、`archcheck`、`go test -race ./...`。
2. 按计划书第 3.1、3.2 节自查三层说明：目录、文件、代码。
3. 更新目录与文件说明：新增的分组目录要有 `README.md`；`doc.go` 的文件清单与包内文件保持一致；用 `commentcheck -map` 重新生成 `docs/代码地图.md` 的文件清单。
4. 写阶段汇报书 `docs/reports/phase-NN-<短名>.md`（格式见计划书第 4.1 节）。
5. 写学习材料 `docs/learn/phase-NN-<短名>.md`（格式见计划书第 4.2 节，包括“开发过程回放”）。
6. 把本阶段的关键实测数字追加到 `docs/数据台账.md`（规则见计划书第 4.3 节和该文件开头），并在汇报书第五节列出新增的条目编号。
7. 更新 `进度.md`（方法见第 5 节）。
8. 提交并推送本阶段分支。**不要合并到 `main`**，合并由用户在学完之后决定。
9. 告诉用户本阶段已完成，然后停下。

## 5. 如何更新 进度.md

**阶段进行中**：每完成一项交付，就勾选对应的方框，让用户随时能看到进展。

**阶段完成时**：

1. 勾选本阶段已经完成的交付项。没完成的保持不勾，并在这一行后面用括号写明原因。
2. 勾选“**工程验收**”。**不得勾选“学习验收”**，那一项只能由用户本人勾选。
3. 把“（完成后由 Astra 填入链接）”替换成文档链接，例如：
   - `汇报书：[phase-00-foundation](docs/reports/phase-00-foundation.md)`
   - `学习材料：[phase-00-foundation](docs/learn/phase-00-foundation.md)`
4. 填写完成日期和一句话总结。
5. 更新顶部“总览”：当前阶段、已完成阶段数、里程碑、最近更新（日期加一句话）。

**链接规则**：

- 使用标准 Markdown 相对路径链接 `[文字](路径)`，**不要用 `[[双链]]`**。这样在 Obsidian 和 GitHub 上都能点击跳转。
- **只链接已经存在的文件**。在 Obsidian 里点击指向不存在文件的链接，会自动新建一个空笔记。
- 文件名和路径里不要有空格。

**各阶段的短名**（分支名、汇报书和学习材料的文件名都用它）：

| 阶段 | 短名 |
|---|---|
| 0 | `phase-00-foundation` |
| 1 | `phase-01-inventory` |
| 2 | `phase-02-order` |
| 3 | `phase-03-observability` |
| 4 | `phase-04-chaos` |
| 5 | `phase-05-doctor-tools` |
| 6 | `phase-06-agent` |
| 7 | `phase-07-reliability` |
| 8 | `phase-08-eval` |
| 9 | `phase-09-generalization` |
| 10 | `phase-10-showcase` |

## 6. 遇到这些情况必须先停下来问用户

- 计划书里的做法走不通，或者有明显更好的做法，需要偏离计划
- 要引入计划书技术栈以外的组件
- 要新增顶层目录，或者 `internal/` 下的一级目录
- 任何会花钱的操作（例如调用付费的大模型接口，单元测试一律用 Mock）
- 要合并到 `main`，或者改写已经推送的历史

## 7. 常用命令

从仓库根目录运行，PowerShell 和 bash 的完整示例见 [README](README.md)。

- 首次准备：PowerShell 执行 `Copy-Item .env.example .env`；bash 执行 `cp .env.example .env`。已有 `.env` 不要覆盖。
- 启动三个服务、压测工具、基础依赖与监控栈：`docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait --wait-timeout 240`（首次空库先设 `LOADGEN_QPS=0`，造数后再通过控制接口开始流量）
- 查看健康状态：`docker compose --env-file .env -f deploy/docker-compose.yml ps`
- 停止并保留数据：`docker compose --env-file .env -f deploy/docker-compose.yml down`
- 安装检查工具：`go install honnef.co/go/tools/cmd/staticcheck@v0.8.2`
- 格式检查/修正：`gofmt -l cmd gen internal tools` / `gofmt -w cmd internal tools`（生成文件通过 buf 重建）
- 静态检查：`go vet ./...`、`staticcheck ./...`
- 项目规则：`go run ./tools/commentcheck ./...`、`go run ./tools/archcheck ./...`
- 阅读清单：`go run ./tools/commentcheck -list ./...`
- 文件地图：`go run ./tools/commentcheck -map ./...`，替换 `docs/代码地图.md` 中自动生成标记之间的表格。
- 单元与竞态测试：`go test -race ./... -count=1 -timeout=180s`
- MySQL / Redis 集成测试：按 README 注入全部 12 个 TEST 变量后，执行 `go test -race ./internal/platform/mysqlx ./internal/platform/migrate ./internal/shop/inventory ./internal/shop/seed ./internal/shop/order ./internal/shop/sales ./internal/shop/product ./internal/doctor/store ./internal/doctor/source/mysqlinspect ./internal/doctor/source/redisinspect ./internal/doctor/approval ./tools/recoverycheck -run Integration -count=1 -timeout=90s -v`。
- 初始化：按 README 注入 MYSQL_DSN 后，执行 `go run ./cmd/shopctl seed`（小规模加 `--small`，已有数据不会重置）。
- 手动查询/扣减：`go run ./cmd/shopctl stock get --sku 1` / `go run ./cmd/shopctl stock deduct --sku 1 --quantity 1`。
- 销量核对：暂停流量并等消费追平后，`go run ./cmd/shopctl check`。
- 五分钟压测：先暂停 Compose 内 loadgen，再运行 `go run ./cmd/loadgen --qps 200 --duration 5m --control 127.0.0.1:6071`；按实际 QPS 和 dropped 判断生成器是否达到目标。
- 重新生成契约：安装 README 固定版本工具后，`buf lint`、`buf generate`；生成物要提交。

Windows 的 `-race` 需要 gcc 可见且 `CGO_ENABLED=1`。迁移、库存、造数、订单、销量测试分别只允许各自的专用测试库；不要将测试 DSN 指向业务库。Redis 商品/幂等测试固定用 DB 14/15，只清理自己的 key。宿主机调试服务前先停止同端口的 Compose 容器；三个服务的 ADMIN_ADDR 分别为订单 6061、库存 6062、消费者 6063。
