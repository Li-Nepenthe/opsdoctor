// Package report 用已保存的诊断、评分和用量生成报告。未完成的槽位和中途换模型的记录仍可追溯；采集受影响或未知的样本单列，美元与人民币分开显示。
// 先看 Summarize 怎样取原始步骤，再看 Markdown 和 Statistics 如何选择分母和展示费用。
//
//   - summary.go：检查实际模型与采集窗口，提取报告、用量和失败原因。
//   - markdown.go：按槽位展示诊断、评分、采集审计和缺失数据。
//   - statistics.go：分别计算各组的均值、范围、样本数和全部费用。
//   - calibration.go：选取分层样本，生成盲评表并核对独立评分。
package report
