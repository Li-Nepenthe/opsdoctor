# 医生的部署配置

医生自己需要一个数据库来保存用户和诊断任务，也需要知道去哪查看被诊断系统的数据。这里把两件事分开：

| 目录 | 解决的问题 |
|---|---|
| [targets](targets/README.md) | 配置每套系统的数据源、独立账号和架构说明 |
| [mysql-init](mysql-init/README.md) | 在医生自己的 MySQL 实例创建专用测试库 |

运行时只把 targets 挂给 doctor。医生容器不能读取实验场景、恢复记录或 Docker socket。诊断工具只使用 shop 的只读凭据；doctor 数据库的写账号只能访问医生自己的库。
