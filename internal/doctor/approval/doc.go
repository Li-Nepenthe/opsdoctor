// Package approval 把模型的建议变成人能检查、批准或拒绝的具体操作。
//
// 模型说“请执行”不代表已经授权。先把参数、语句、证据和撤销方法存成审批单，
// 执行器再从数据库领取已批准的单子。真正执行时不再接受模型提供的第二份参数。
//
// 文件清单：
//   - service_test.go：证明只执行原单参数，恢复先核对连接池。
//   - permissions_integration_test.go：实测 INDEX 能力和业务读写拒绝。
//   - prepare_test.go：检查写工具参数和已有索引的恢复路径。
//   - doc.go：审批解决的问题与阅读入口。
//   - model.go：审批单、依赖和执行状态。
//   - service.go：申请、等待和领取后执行。
//   - prepare.go：校验三个写工具的参数，生成可审阅的计划。
//   - definitions.go：向模型说明写工具的参数和审批要求。
//   - backend.go：绑定独立数据库账号和固定运维地址。
//   - execute.go：执行之前核对真实状态，避免崩溃后盲目重做。
//
// 阅读顺序：Service.Handle → prepare → Repository.ClaimApproval → execute。
// 本包没有批准接口；批准入口只提供给已登录的人，不进入模型或 MCP 工具菜单。
package approval
