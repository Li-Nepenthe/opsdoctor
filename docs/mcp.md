# 从外部客户端使用医生的只读工具

有时我们想在编辑器里手动查指标、查调用链，不需要先创建一个自动诊断。MCP 是客户端与工具服务交流的一套协议。OpsDoctor 用它开放已有的只读工具；查数据时仍经过同一套参数校验、十秒超时和 4 KiB 截断。

一个连接只对应一个系统。shop 的地址是 `http://127.0.0.1:8090/mcp/shop`。工具参数没有系统名、URL 或主机名。要查看另一个系统，需要另一个地址和那个系统自己的令牌。

## 准备身份

在本机 `.env` 设置 `SHOP_MCP_TOKEN`，使用至少 32 字节的独立随机值。它不能与告警、运维或登录凭据共用。接入配置里的 `mcp_token_env` 指向这个变量；缺少或共用凭据会在启动时被拒绝。没有配置 MCP 的系统不开放该入口。

重建医生后再连接。这个令牌没有网页登录的一小时期限，需要由部署者主动更换。每个 HTTP 请求都会验证它，不能把它放进 URL，也不要把实际值贴进配置示例、截图或聊天。

## Claude Code 的配置

在本机客户端配置中使用下面的服务器条目。`${SHOP_MCP_TOKEN}` 保持为字面占位符，由客户端从启动它的环境中读取；不要把实际值写进 JSON。Claude Code 的 [官方 MCP 文档](https://code.claude.com/docs/en/mcp) 说明了 HTTP 服务器及 headers 中的环境变量展开方式。

```json
{
  "mcpServers": {
    "opsdoctor-shop": {
      "type": "http",
      "url": "http://127.0.0.1:8090/mcp/shop",
      "headers": {
        "Authorization": "Bearer ${SHOP_MCP_TOKEN}"
      }
    }
  }
}
```

启动客户端之前，在它的环境中设置同名变量。仓库的 `.env` 不会自动成为任意外部程序的环境。编辑器显示未认证时，先核对是否真的继承了变量、地址中的系统名是否正确，以及医生是否已用新配置重建。

在客户端查看工具列表，先用 `service_topology`、参数 `{}` 试一次。它只返回架构说明，不会调用大模型，也不会修改系统。`create_index`、`set_db_pool_size` 和 `warm_cache` 不在这份菜单里，直接猜名字调用也会被拒绝。

本项目没有替你启动 Claude Code 的付费对话。使用外部客户端自身的模型可能产生它那边的费用，与医生的模型账本分开。

## 怎么确认调用确实发生了

医生把每次工具调用写进自己的 `mcp_audits` 表：开始时间、固定系统、工具名、脱敏参数摘要、结果状态和结束时间。先保存开始记录，再访问目标系统；审计不可写时不执行工具。未知工具和参数错误也会留下记录。进程中断时可能留下 `running`，它表示结果未知。

阶段 7 已用官方 Go SDK 客户端连接本机医生：菜单有 16 个只读工具，`service_topology` 调用成功，数据库审计记录为 `shop / service_topology / ok`。完整参数与结果日志只保留脱敏内容，关键数字和来源见[数据台账](数据台账.md)。这证明的是实际协议调用，不是截图中手工拼出的假响应。

实现入口是 [mcpserver](../internal/doctor/mcpserver/doc.go)。客户端身份与系统隔离由 `TestTokenIsolation`、`TestClientBoundary` 验证；数据库保存由 `TestIntegrationMCPAudit` 验证。
