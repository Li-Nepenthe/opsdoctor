// 本文件：让另一家模型评分，并在采纳前检查格式和证据引用。
//
// 评委也可能胡乱引用步骤。程序只接受这份诊断里实际存在的编号，并要求所有
// 待评里程碑恰好出现一次。格式失败会保留为评分失败，不偷偷重问直到得到好分数。
//
// 主要内容：Input、Grade 定义审阅材料；Evaluate 调用已绑定账本的客户端；
// Check 校验回复；RubricHash 标识实际嵌入的评分标准。
// 阅读建议：本包不读取 Key，真实 HTTP 客户端与预算账户由评测入口装配。

package judge

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/scoring"
)

// RubricVersion 评分标准改动时递增；正式批次不允许混用不同版本。
const RubricVersion = "rubric-v1.1"

// rubric 随二进制冻结，不能在运行中被另一份同名文件替换。
//
//go:embed rubric.md
var rubric string

// Input 不含模型名字和实验组，避免评委根据供应商品牌偏好评分。
type Input struct {
	Truth      any                      `json:"reference"`    // 标准答案只送给评委。
	Report     json.RawMessage          `json:"report"`       // 医生已经提交的原报告。
	Symptoms   json.RawMessage          `json:"symptoms"`     // 最初告警或例行检查输入。
	Steps      []Observation            `json:"steps"`        // 排除模型自述，保留真实工具、输入和审批记录。
	Milestones []string                 `json:"milestones"`   // 完整项目列表，以一为起点编号。
	Rules      []scoring.EvidenceResult `json:"rule_results"` // 程序已经判定的项目。
}

// Observation 留下判断所需内容，去掉模型名和 token，避免无关线索影响评分。
type Observation struct {
	Step   int             `json:"step"`           // 原始步骤号。
	Kind   string          `json:"kind"`           // tool、system 或 approval。
	Tool   string          `json:"tool,omitempty"` // 查看或提议的操作名。
	Args   json.RawMessage `json:"args"`           // 原有的脱敏参数。
	Result string          `json:"result"`         // 完整的有界工具结果或审批结果。
	Status string          `json:"status"`         // 失败结果也保留。
}

// Grade 把机制、证据和不当写操作分开，不用一个总分掩盖差异。
type Grade struct {
	Mechanism       string                   `json:"mechanism"`        // correct、partial、incorrect。
	Reason          string                   `json:"reason"`           // 机制结论的依据。
	Milestones      []scoring.EvidenceResult `json:"milestones"`       // 只填写未由规则确定的部分。
	UnrelatedAction bool                     `json:"unrelated_action"` // 是否提出与证据无关的写操作。
	ActionReason    string                   `json:"action_reason"`    // 包括没有不当提议时的说明。
}

// Observations 不发送大模型自己的思考过程，避免把“打算查询”混成“真的查到了”。
func Observations(steps []agent.Step) []Observation {
	values := []Observation{}
	for _, step := range steps {
		if step.Kind != "tool" && step.Kind != "approval" && !(step.Kind == "system" && step.Status == "input_evidence") {
			continue
		}
		args := step.Args
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		values = append(values, Observation{Step: step.Seq, Kind: step.Kind, Tool: step.ToolName, Args: args, Result: step.Result, Status: step.Status})
	}
	return values
}

// Evaluate 只发起一次逻辑评分；供应商网络重试仍由同一个收费客户端负责审计。
func Evaluate(ctx context.Context, client llm.Client, model, auditID string, input Input) (Grade, llm.Response, error) {
	if len(input.Report) == 0 || !json.Valid(input.Report) {
		return Grade{}, llm.Response{}, fmt.Errorf("没有报告，不调用评委")
	}
	raw, tool, err := judgeRequest(input)
	if err != nil {
		return Grade{}, llm.Response{}, err
	}
	// JSON 字符串会转义材料中的边界伪造。评委只收到一个 user 数据对象。
	request := llm.Request{DiagnosisID: auditID, CallID: "eval-grade-v1", Model: model, MaxTokens: 2048, DiagnosisLimitMicros: 300000,
		Messages: []llm.Message{{Role: "system", Content: rubric}, {Role: "user", Content: string(raw)}},
		Tools:    []llm.Tool{tool}}
	response, err := client.Complete(ctx, request)
	if err != nil {
		return Grade{}, response, err
	}
	value, err := CheckResponse(response, input)
	return value, response, err
}

// CheckResponse 也供崩溃恢复使用，已结算回复与新回复必须经过相同的校验。
func CheckResponse(response llm.Response, input Input) (Grade, error) {
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Function.Name != "submit_grade" {
		return Grade{}, fmt.Errorf("评委必须且只能提交一次 submit_grade")
	}
	return Check([]byte(response.Message.ToolCalls[0].Function.Arguments), input)
}

// Check 不接受未知字段、重复项目、编造步骤或改写规则项；错误不转换成零分。
func Check(raw []byte, input Input) (Grade, error) {
	var value Grade
	// 指针区分“明确填 false”和“根本没填”，否则缺字段会悄悄得到零值。
	var wire struct {
		Mechanism       string `json:"mechanism"`
		Reason          string `json:"reason"`
		UnrelatedAction *bool  `json:"unrelated_action"`
		ActionReason    string `json:"action_reason"`
		Milestones      []struct {
			Milestone int    `json:"milestone"`
			Achieved  *bool  `json:"achieved"`
			Steps     []int  `json:"steps"`
			Reason    string `json:"reason"`
		} `json:"milestones"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 32768 || decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF || wire.UnrelatedAction == nil || wire.Milestones == nil {
		return value, fmt.Errorf("评分 JSON 格式不合法")
	}
	value = Grade{Mechanism: wire.Mechanism, Reason: wire.Reason, UnrelatedAction: *wire.UnrelatedAction, ActionReason: wire.ActionReason, Milestones: []scoring.EvidenceResult{}}
	for _, item := range wire.Milestones {
		if item.Achieved == nil || item.Steps == nil {
			return value, fmt.Errorf("里程碑缺少必填字段")
		}
		value.Milestones = append(value.Milestones, scoring.EvidenceResult{Milestone: item.Milestone, Achieved: *item.Achieved, Steps: item.Steps, Reason: item.Reason})
	}
	if value.Mechanism != "correct" && value.Mechanism != "partial" && value.Mechanism != "incorrect" {
		return value, fmt.Errorf("机制评分不在三档范围内")
	}
	if strings.TrimSpace(value.Reason) == "" || strings.TrimSpace(value.ActionReason) == "" {
		return value, fmt.Errorf("评分缺少理由")
	}
	known := map[int]bool{}
	for _, step := range input.Steps {
		known[step.Step] = true
	}
	ruled, seen := map[int]bool{}, map[int]bool{}
	for _, rule := range input.Rules {
		ruled[rule.Milestone] = true
	}
	for index := range value.Milestones {
		item := &value.Milestones[index]
		if item.Milestone < 1 || item.Milestone > len(input.Milestones) || ruled[item.Milestone] || seen[item.Milestone] || strings.TrimSpace(item.Reason) == "" {
			return value, fmt.Errorf("里程碑缺失、重复或覆盖规则项")
		}
		seen[item.Milestone], item.Method = true, "judge"
		if item.Achieved && len(item.Steps) == 0 {
			return value, fmt.Errorf("已达成项目没有引用步骤")
		}
		for _, number := range item.Steps {
			if !known[number] {
				return value, fmt.Errorf("评委编造了证据步骤号")
			}
		}
	}
	if len(seen)+len(ruled) != len(input.Milestones) {
		return value, fmt.Errorf("评委没有覆盖全部待评项目")
	}
	return value, nil
}

// RubricHash 记录实际字节，版本名没改但正文被改过也能被开跑检查发现。
func RubricHash() string {
	digest := sha256.Sum256([]byte(rubric))
	return hex.EncodeToString(digest[:])
}
