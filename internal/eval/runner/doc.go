// Package runner 让同一次实验能找回相同的输入和诊断，而不是每次重试都重新花钱。
//
// prepare/pilot 保留最初试跑入口。正式运行用 freeze 固定五组顺序，再由 run
// 逐轮调用宿主机的 chaos 注入、诊断与恢复。医生看不到这些控制状态。
// export 读取已保存结果；calibrate 生成空白盲评表，不代填独立分数。
//
// 文件清单：
//   - generalization.go：为第二系统建立独立批次、账户边界和冻结顺序。
//   - generalization_scenario.go：加载官方故障定义，通过有界 HTTP 修改开关并核对指标。
//   - generalization_run.go：保存每轮完整恢复文件，推进外部系统注入、诊断、恢复与冷却。
//   - generalization_environment.go：按 Docker 实际上限检查外部实验内存，不套用 shop 的暂停线。
//   - generalization_collection.go：独占保存每份诊断的Collector日志窗口，异常先落摘要再停止评分。
//   - generalization_collection_test.go：用模拟Docker输出验证最终丢弃、映射错误、窗口与续跑证据。
//   - generalization_test.go：用假接口验证开关范围、独立账户、异常暂停、恢复快照和说明书哈希。
//   - load.go：从冻结场景计算诊断期的精确流量，并持续核对实际参数。
//   - load_test.go：遍历全部场景，验证流量、热点及未变基线的严格匹配。
//   - execution.go：在命令、轮次和槽位执行前追加当时的代码版本。
//   - baseline_amendment.go：只批准基线检查与审计的变化，重启没有模型槽位的中断轮次。
//   - baseline_amendment_test.go：证明本次白名单不接受提示、工具、手册、场景或评分变化。
//   - calibration.go：从原始诊断与固定抽样导出盲评材料，导出后由独立评分者判断。
//   - automatic.go：按冻结矩阵注入、诊断、恢复和等待，遇到不确定状态停下核对。
//   - automatic_test.go：核对本机恢复记录只能对应原来的场景和注入时刻。
//   - injection.go：通过正常商品请求写入安全实验材料，并核对实际暴露和写操作数。
//   - paired_test.go：验证五配置完整性、账户隔离和 Mock 评委开关。
//   - paired.go：冻结每次故障的五配置随机顺序，按原输入续跑并保留可靠性补跑。
//   - grading.go：把评委接到人民币账本，保存评分失败并恢复已收到的回复。
//   - clock.go：独立预检或花钱前检查一分钟容器时钟，并在等待诊断时持续检查，跳变就停止。
//   - clock_test.go：验证暂停原因传递与采样取消，不启动 Docker。
//   - cli.go：解析试跑命令，打开医生自己的数据库，安排模式切换和任务等待。
//   - client.go：登录并读取正在运行的医生状态、告警和故障开放期。
//   - environment.go：核对实际压测参数与本项目容器六 GiB 内存暂停线。
//   - environment_test.go：检查基线变化、异常内存读数和非法命令的拒绝。
//   - manifest.go：记录提示、代码、手册和模型版本，拒绝把改动后的运行混入旧批次。
//   - amendment.go：核对只发生已批准的校时修订，并追加独立版本记录。
//   - amendment_test.go：验证校时例外仍拒绝提示和其他行为变化。
//   - doc.go：说明批次、槽位和医生之间的职责。
//
// 阅读顺序：RunCLI → prepare → pilot → waitDiagnosis。
// 长跑的入口是 automatic；所有真实模型请求仍经过费用和批次 token 账户。
package runner
