// 本文件：按本轮请求的上下文、整次诊断的累计用量和费用判断是否还能查证。
// inputBound 用上轮实测 token 和新增字节估算输入，budgetReason 决定达到多个上限时先说明哪一个。

package agent

// inputBound 利用对话只追加的约束，避免每轮都按整段 UTF-8 字节高估历史。
// 前一轮尚未获得有效计量时，只能按全部字节预留。1024 的余量也覆盖拼接处的分词变化。
func (run *session) inputBound(size int) int64 {
	const protocolMargin = 1024
	bound := int64(size + protocolMargin)
	if run.lastInput > 0 {
		bound = min(bound, run.lastInput+int64(max(0, size-run.lastBytes))+protocolMargin)
	}
	return bound
}

// budgetReason 的先后顺序只影响展示原因；任何一个上限耗尽，都不能继续查新证据。
func (run *session) budgetReason() string {
	switch {
	case run.result.Rounds >= run.agent.limits.Rounds:
		return "轮数达到上限"
	case run.result.ToolCalls >= run.agent.limits.Tools:
		return "工具调用达到上限"
	case run.result.Usage.Input+run.result.Usage.Output >= run.agent.limits.Tokens:
		return "累计 token 达到安全上限"
	case run.result.CostMicros >= run.agent.limits.CostMicros:
		return "本次诊断费用达到上限"
	default:
		return ""
	}
}
