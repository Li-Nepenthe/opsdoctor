// Package runner 让同一次实验能找回相同的输入和诊断，而不是每次重试都重新花钱。
//
// prepare/pilot 保留最初试跑入口。正式运行用 freeze 固定五组顺序，再由 run
// 逐轮调用宿主机的 chaos 注入、诊断与恢复。医生看不到这些控制状态。
// export 读取已保存结果；calibrate 生成空白人工评分表，不替人打分。
//
// 文件清单：
//   - calibration.go：从原始诊断与固定抽样导出盲评材料，完成后等待人评分。
//   - automatic.go：按冻结矩阵注入、诊断、恢复和等待，遇到不确定状态停下核对。
//   - automatic_test.go：核对本机恢复记录只能对应原来的场景和注入时刻。
//   - injection.go：通过正常商品请求写入安全实验材料，并核对实际暴露和写操作数。
//   - paired_test.go：验证五配置完整性、账户隔离和 Mock 评委开关。
//   - paired.go：冻结每次故障的五配置随机顺序，按原输入续跑并保留可靠性补跑。
//   - grading.go：把评委接到人民币账本，保存评分失败并恢复已收到的回复。
//   - clock.go：独立预检或花钱前检查一分钟容器时钟，并在等待诊断时持续检查，跳变就停止。
//   - clock_test.go：用样本验证前跳、倒退、缺样和暂停原因，不启动 Docker。
//   - cli.go：解析试跑命令，打开医生自己的数据库，安排模式切换和任务等待。
//   - client.go：登录并读取正在运行的医生状态、告警和故障开放期。
//   - environment.go：核对实际压测参数与本项目容器六 GiB 内存暂停线。
//   - environment_test.go：检查基线变化、异常内存读数和非法命令的拒绝。
//   - manifest.go：记录提示、代码、手册和模型版本，拒绝把改动后的运行混入旧批次。
//   - doc.go：说明批次、槽位和医生之间的职责。
//
// 阅读顺序：RunCLI → prepare → pilot → waitDiagnosis。
// 长跑的入口是 automatic；所有真实模型请求仍经过费用和批次 token 账户。
package runner
