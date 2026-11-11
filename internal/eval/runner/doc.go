// Package runner 组织可续跑的评测。一个批次先冻结场景、执行配置和评分版本，一次故障中的每组诊断占一个固定槽位。
// 中断后从仓储取回原任务和输入，保留原失败与费用。宿主机负责实验控制，医生只收到诊断所需的症状和告警。
//
// 文件清单：
//   - cli.go：解析命令，安排试跑、诊断等待和恢复后的收尾。
//   - client.go：登录医生服务，读取运行状态和告警，并在中断时取消原诊断。
//   - manifest.go：冻结提示、手册、代码和模型版本，续跑时核对原清单。
//   - execution.go：在命令、轮次和槽位执行前登记当时的评测器版本。
//   - environment.go：核对 shop 压测的实际负载和本项目容器的内存暂停线。
//   - load.go：从冻结场景推导流量，在诊断期间持续核对控制接口的读数。
//   - clock.go：在预检和长时间诊断中监视容器时钟，发现跳变就暂停。
//   - automatic.go：按冻结顺序推进 shop 实验，保留中断记录和恢复文件。
//   - paired.go：固定同一故障的五组配置，复用原输入并单独保存补跑。
//   - injection.go：用普通商品请求产生实验日志，核对医生实际读到的内容和写操作记录。
//   - grading.go：绑定评委账本，保存评分结果并从原回答恢复中断的评分。
//   - calibration.go：固定盲评样本，导出原诊断和观察供独立评分。
//   - generalization.go：为 OpenTelemetry Demo 冻结独立的场景、组序和账户。
//   - generalization_scenario.go：读取 Demo 场景并核对症状和开关，标准答案仅供宿主机评分。
//   - generalization_run.go：推进 Demo 轮次，保存并核对现场与本轮恢复文件。
//   - generalization_baseline.go：等待同一轮的全部健康条件，独立保存每次检查的证据。
//   - generalization_environment.go：按 Docker 实际资源上限核对 Demo 实验的内存。
//   - generalization_collection.go：保存每份诊断的 Collector 日志窗口，采集质量不明时停止评分。
//   - generalization_timing.go：独占保存首次观察，只有同一进程的单调时钟能证明耗时。
//   - amendment.go：核对历史批次获准的校时修订并追加版本记录。
//   - baseline_amendment.go：核对历史批次获准的基线修订，记录尚无诊断的轮次重启。
//
// 从 RunCLI、prepare 和 pilot 看一次试跑，再沿 automatic 和 paired.go 看正式运行。
// 两个修订文件只服务历史批次，初读可以跳过。新批次不产生修订记录，EvalVersions 直接返回冻结版本。
package runner
