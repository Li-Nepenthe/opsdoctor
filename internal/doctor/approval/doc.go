// Package approval 保存模型建议的写操作，供人决定是否执行。
// 申请中有具体参数、预览、证据和撤销方法。批准入口只向已登录的人开放，模型和 MCP 均不能批准。
// 执行器从数据库领取已批准的计划。恢复同一调用时，它仍用原单的参数，先确认远端是否已经执行。
//
// 文件清单：
//   - model.go：定义审批单、执行计划和持久化接口。
//   - service.go：根据审批状态创建申请、等待决定或领取已批准的计划。
//   - prepare.go：核验本次诊断的证据及写操作范围，生成供人审阅的计划。
//   - definitions.go：向模型列出已配置的写操作和申请所需的字段。
//   - backend.go：绑定独立索引账号和固定运维地址，限制连接能力。
//   - execute.go：核对远端状态后执行已领取的计划，避免崩溃恢复时重复写入。
//
// 从 Service.Handle 看申请怎样进入数据库，再读 prepare、Repository.ClaimApproval 和 execute。
package approval
