# deploy/alertmanager：告警通知

Prometheus 只负责判断“告警条件是否成立”，发通知是 Alertmanager 的事。它把告警整理一下，再推送给接收方。

[alertmanager.yml](alertmanager.yml) 的设置：

- **分组**：同一种告警、同一个服务的多条告警，合成一次通知。一次故障往往同时触发很多条告警，合并以后不会一下子收到一大堆。
- **推送给谁**：现在推给临时的告警接收器 alertsink（`http://alertsink:8091/api/v1/alerts`）。阶段 5 改为推给医生。
- **恢复通知**：告警条件不再成立时，也会推一条 `resolved`（已恢复）通知。
- **令牌**：推送时在请求头带上令牌，接收方据此确认请求确实来自 Alertmanager。令牌来自环境变量，容器启动时写进一个临时文件，不保存在仓库里。

Alertmanager 只负责通知，不会自动执行任何修复。
