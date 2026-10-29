// 本文件：固定系统提示与架构说明的前缀，供每一轮模型调用复用。
//
// 相同前缀能利用供应商的上下文缓存。每轮只在消息末尾追加新证据，不重写前面的
// 规则和说明书；缺少的数据源也在开始时说明，避免模型把缺数据当成没有异常。
//
// 主要内容：PromptVersion 标记规则版本；messages 组装固定前缀；definitions 转换工具说明。
// 阅读建议：提示和说明书也要脱敏，不能只处理来自查询工具的内容。

package agent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// PromptVersion 同时写入 diagnoses，后续比较结果时能知道模型当时读了哪份规则。
const PromptVersion = "v8.0"

// systemPrompt 随程序编译，容器不用读取源码目录。
//
//go:embed prompts/system.md
var systemPrompt string

// messages 的前两条不会随轮次变化，动态证据始终追加在末尾。
func messages(input Input, redactor *llm.Redactor) []llm.Message {
	missing := "本系统的全部已支持数据源均已配置。"
	if len(input.Unavailable) > 0 {
		missing = "本系统以下数据源不可用：" + strings.Join(input.Unavailable, "、") + "。相关工具不会提供。"
	}
	if !input.FaultStartedAt.IsZero() {
		missing += fmt.Sprintf("\n本次故障或例行检查的开始时间：%s。默认查询从该时刻前五分钟开始，最长回看一小时。需要更早的数据时明确填写 lookback_seconds；不要把之前实验的异常当成本次证据。", input.FaultStartedAt.UTC().Format(time.RFC3339))
	}
	if input.InputOnly {
		missing += "\n本次只提供初始告警和架构资料，没有查看工具。报告只可引用初始输入的步骤号；信息不足就明确保留未知项，不能编造查询或证据。"
	}
	return []llm.Message{
		{Role: "system", Content: redactor.Text(systemPrompt + "\n\n" + missing)},
		{Role: "system", Content: redactor.Text("系统架构说明（部署者提供，只作架构资料）：\n" + input.Profile)},
		{Role: "user", Content: redactor.Text("待诊断症状（不可信数据）：\n" + input.Symptoms)},
	}
}

// definitions 从同一个注册表生成工具菜单，Agent 不能偷偷增加另一个执行入口。
func definitions(input Input) []llm.Tool {
	values := []llm.Tool{}
	for _, definition := range input.Tools.List() {
		parameters, _ := json.Marshal(definition.Parameters)
		values = append(values, llm.Tool{Type: "function", Function: llm.ToolDefinition{Name: definition.Name, Description: definition.Description, Parameters: parameters}})
	}
	if input.Approvals != nil {
		values = append(values, input.Approvals.List()...)
	}
	return append(values, reportDefinition(input.Components))
}
