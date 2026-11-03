# 医生的部署配置

医生可以单独运行。它需要自己的 MySQL 保存用户、诊断、审计和费用记录；被诊断系统可以已经部署在别处。这里的独立 Compose 只包含 `doctor` 和 `doctor-mysql`，不启动 shop、监控栈或故障控制器，也不要求任何 `SHOP_` 变量。

| 文件或目录 | 解决的问题 |
|---|---|
| [compose.yaml](compose.yaml) | 单独启动医生和全新的医生数据库 |
| [standalone.env.example](standalone.env.example) | 医生自身账号、模型选项和挂载路径的假值示例 |
| [targets](targets/README.md) | 每套系统的数据源、独立凭据变量名和架构说明格式 |
| [mysql-init](mysql-init/README.md) | 原完整演示的专用集成测试库初始化，独立部署不挂载 |

## 先准备两份本机配置

从仓库根目录执行。已有本机配置不要覆盖，首次使用时才复制：

```powershell
Copy-Item -LiteralPath deploy/doctor/standalone.env.example -Destination deploy/doctor/.env.standalone
```

bash：

```bash
cp -n deploy/doctor/standalone.env.example deploy/doctor/.env.standalone
```

编辑 `.env.standalone`，替换其中的演示密码和 JWT 密钥。登录密码需要 8–72 字节；JWT 至少 32 字节，实际部署使用随机值。`DOCTOR_ADMIN_EMAIL` 和 `DOCTOR_ADMIN_PASSWORD` 只在邮箱首次创建时使用，已有账号不会因重启被覆盖。

另外准备一个系统凭据文件，例如 `deploy/doctor/.env.targets`。文件里的变量名与目标 YAML 的 `alert_token_env`、`mysql_readonly_dsn_env`、`redis_readonly_url_env`、OpenSearch 的 `username_env` / `password_env` 等对应。凭据值写在此文件中，YAML 只引用变量名。每个系统使用自己的前缀和独立账号；告警、MCP 与运维令牌均至少 32 字节且不能共用。

这两份以 `.env.` 开头的本机文件已被[仓库忽略规则](../../.gitignore)排除。自选其他文件名时，先确认该文件也被 Git 忽略，不把真实值放进示例或提交记录。

| 变量 | 用途 |
|---|---|
| `DOCTOR_MYSQL_PASSWORD` / `DOCTOR_MYSQL_ROOT_PASSWORD` | 新医生库的普通账号密码与 root 初始化密码 |
| `DOCTOR_ADMIN_EMAIL` / `DOCTOR_ADMIN_PASSWORD`、`JWT_SECRET` | 首次登录与认证 |
| `DOCTOR_TARGETS_DIR` | 宿主机上只包含所需系统配置和说明书的目录 |
| `DOCTOR_TARGET_ENV_FILE` | 上述系统凭据文件的路径，由 Compose `env_file` 传给医生 |
| `DOCTOR_HTTP_PORT` / `DOCTOR_BIND_ADDR` | 可选，默认只在 `127.0.0.1:8090` 开放页面与 API |
| `DOCTOR_RUNBOOKS_PATH` | 可选，默认挂载仓库的通用手册目录 |

路径可以写绝对路径；相对路径以本目录的 `compose.yaml` 为基准。例如 `DOCTOR_TARGET_ENV_FILE=./.env.targets` 指向 `deploy/doctor/.env.targets`。Windows 的绝对路径建议使用 `D:/example/targets` 这样的写法。

两份配置有不同的读取路径。命令行 `--env-file` 为 Compose 提供变量插值；容器的 `env_file` 则把系统凭据传入进程。医生自身和模型选项由 Compose 的 `environment` 明确设置，优先于容器 `env_file`，所以把它们放在 `.env.standalone`；把任意系统凭据放在 `.env.targets`。Compose 不逐条列出系统变量，不会用空的 `SHOP_` 等字段覆盖新系统的凭据。宿主 shell 中同名变量还会优先于命令行 env 文件，排查时核对变量名，不打印秘密值。

## 选择要接入的系统

`DOCTOR_TARGETS_DIR` 必须指向已经存在的目录，包含 1–32 份 YAML 和各自的 `.profile.md`。最小目标也要有 `name`、`profile`、`alert_token_env` 与 `alert_recovery_window`，说明书要有组件表。缺少的数据源不填写，对应工具降级为不可用；没有 DDL 或 `ops_api` 时也可以只读接入。配置字段见[系统接入说明](targets/README.md)。

不要直接把仓库整个 `targets` 当作所有部署的默认值：其中包含 shop 和 otel-demo，加载它们就必须提供它们实际引用的全部凭据。只需要外部系统时，将所需的 YAML 与说明书放进单独目录，再填写这个目录的路径。Compose 不替新系统补 shop 账号，也不自动连接原项目网络。

数据源地址必须从医生容器中可达。Docker Desktop 可使用 `host.docker.internal` 访问宿主机已有的端口；Compose 也声明了 Linux 的 `host-gateway` 别名。Linux 上该别名只解决地址解析，不会自动让仅绑定宿主回环的服务对容器可达。独立远端后端则填写容器可访问的固定地址，并使用相应只读账号。

Jaeger 的地址可带安全的部署前缀。例如 Demo 的接入地址为 `http://host.docker.internal:19686/jaeger/ui`，适配器在此之后追加固定搜索和详情 API；不要把 `/api/traces` 当部署前缀。编码路径、点路径、重复斜线、反斜杠、URL 凭据、查询串、片段及重定向仍被拒绝，其他 HTTP 数据源继续使用根地址。真实 Search/Get 接线原件见[搜索](../../docs/reports/phase-09-jaeger-prefix-search.json)和[详情](../../docs/reports/phase-09-jaeger-prefix-get.json)。

医生只读挂载这个目标目录与通用手册，不挂仓库、实验场景、恢复记录或 Docker socket。target 凭据只进入医生，不进入 `doctor-mysql`。医生自己的数据库写账号与被诊断系统的只读账号分开。

## 构建、启动和停止

下列命令在 PowerShell 与 bash 中相同，均从仓库根目录执行：

```text
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml config --quiet
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml up -d --build --wait --wait-timeout 240
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml ps
```

页面默认位于 `http://127.0.0.1:8090`。初始化失败时先查这两个服务的日志；配置渲染可用 `config --quiet`，不要把含密码的完整配置输出交给他人。构建固定使用 `opsdoctor/doctor:phase09-20261102.4`，MySQL 固定为 `8.4.13`；Go 与运行时镜像版本沿用[公共 Dockerfile](../docker/go.Dockerfile)。

默认项目名为 `opsdoctor-standalone`，也可以在每条命令中加入同一个 `-p`。网络与 `doctor-state` 数据卷由这个项目独立创建，不引用原来的 `opsdoctor_doctor-mysql-data`。数据库不发布宿主端口，医生通过本项目的 `doctor-mysql:3306` 连接。MySQL 官方入口创建 `doctor` 库、`doctor` 账号并授予该库权限；医生启动时执行自身表迁移。这里不挂原演示的测试库初始化脚本，不额外创建测试库。已有数据卷不会重跑首次初始化，也不会因修改 env 密码而自动更改库内密码。

停止时保留容器和卷：

```text
docker compose --env-file deploy/doctor/.env.standalone -f deploy/doctor/compose.yaml stop
```

再次使用同一条 `up` 命令会继续使用本项目的数据。不要改用 `down -v` 或把原医生卷挂进来。原 shop 演示继续使用[原 Compose](../docker-compose.yml)，和这里是两个独立入口；启动前确认页面端口没有被另一套医生占用。

## 模型与可选参数

默认 `LLM_PROVIDER=mock`、`EVAL_COMPARISON_ENABLED=false`，即使机器有模型 Key 也不会因此调用真实模型。页面会明确标记 Mock。真实调用仍需用户授权，并按独立预算账户记录；本配置的默认账户名是 `standalone`，不是阶段 7、8 的账户。

可选模型变量包括 `LLM_BASE_URL`、`LLM_API_FORMAT`、`LLM_MODEL`、`LLM_API_KEY`、`LLM_TIMEOUT`，以及 `LLM_INPUT_PRICE_PER_MTOKEN`、`LLM_OUTPUT_PRICE_PER_MTOKEN`、`LLM_CACHE_HIT_PRICE_PER_MTOKEN`、`LLM_CACHE_WRITE_PRICE_PER_MTOKEN`。预算用 `LLM_BUDGET_ACCOUNT`、`LLM_BUDGET_CURRENCY`、`LLM_TOTAL_BUDGET`、`LLM_DIAGNOSIS_BUDGET`。默认金额是部署保护值，实际价格和授权范围仍须开跑前核对，不能把默认单价当作永久价表。

真实对比默认关闭，启用时还需要独立的 `DEEPSEEK_API_KEY` 和 `EVAL_COMPARISON_*` 参数，完整名称与默认值见[Compose](compose.yaml)。调度的 `DOCTOR_LEASE_TTL`、`DOCTOR_RESUME_MAX_AGE`、并发与告警等待参数也可覆盖；HTTPS 部署使用 `DOCTOR_COOKIE_SECURE=true`。这里不自动部署反向代理或远端监控组件。

阶段 9 的专用[薄覆盖](../opentelemetry/doctor.phase09.yaml)固定本阶段预算与评测数据库端口；它不替代通用 Compose。宿主评测器另为每份诊断保存 Collector 原始窗口，并将采集受影响或无法验证的结果单列，先保留诊断及费用再停止评分和后续槽位。这属于本次实验的审计条件，不向医生容器添加 Docker socket。范围和失败保留方式见[架构说明](../../docs/architecture.md#核心流程二注入--评测)与[采集审计实现](../../internal/eval/runner/generalization_collection.go)。
