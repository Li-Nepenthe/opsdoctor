// 本文件：用百万分之一账户货币单位预留和结算费用。发送前按最贵输入价预留，取得用量后按实际档位结算。

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Error 固定三类预算拒绝原因，供调度器区分账户、单次诊断和评测批次的上限。
type Error string

// Error 把预算拒绝原因交给调用者，无需额外读取或公开账户信息。
func (err Error) Error() string { return string(err) }

// ErrBudget 既用于调度拦截，也用于两次模型调用之间的额度检查。
const ErrBudget Error = "全局模型费用上限已用尽或不足以预留本次请求"

// ErrDiagnosisBudget 与账户额度分开，页面才能解释是本次调查停止，而非所有诊断停用。
const ErrDiagnosisBudget Error = "本次诊断费用已达上限或不足以预留本次请求"

// ErrEvaluationBudget 只停止这一批评测，不把它误报为供应商余额不足。
const ErrEvaluationBudget Error = "评测 token 额度已用尽、预留不足或需要人工核对"

// Prices 保存每百万 token 的单价，单位同样是百万分之一账户货币单位。
type Prices struct {
	CacheWriteMicros int64 // 缓存写入价；DeepSeek 没有这一档时为零。
	InputMicros      int64 // 缓存未命中输入价。
	CacheHitMicros   int64 // 缓存命中的输入价；没有命中用量时按未命中价结算。
	OutputMicros     int64
}

// Money 将最多六位小数的金额转换为整数，不接受科学计数法或负数。
func Money(text string) (int64, error) {
	parts := strings.Split(text, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("金额必须是非负十进制数，最多六位小数")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 6 {
		return 0, fmt.Errorf("金额最多六位小数")
	}
	// 只接受数字，避免 ParseInt 把带正负号的值也当成有效配置。
	for _, digit := range parts[0] + fraction {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("金额只能含数字和小数点")
		}
	}
	value, err := strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
	if err != nil || value > 1000000*1000000 {
		return 0, fmt.Errorf("金额超出支持范围")
	}
	return value, nil
}

// Cost 分别向上取整，不因微小舍入把未花的钱误认为仍可使用。
func (prices Prices) Cost(input, output int64) int64 {
	const tokensPerMillion = int64(1000000)
	return (input*prices.InputMicros+tokensPerMillion-1)/tokensPerMillion + (output*prices.OutputMicros+tokensPerMillion-1)/tokensPerMillion
}

// Charge 只信任供应商明确返回的命中数，剩余输入全部按未命中计算。
// 三部分分别向上取整到微元，和余额接口的分位金额相比会有很小的舍入差。
func (prices Prices) Charge(usage Usage) int64 {
	const million = int64(1_000_000)
	hit := min(max(usage.CacheHit, 0), usage.Input)
	write := min(max(usage.CacheWrite, 0), usage.Input-hit)
	return prices.Cost(usage.Input-hit-write, usage.Output) + (hit*prices.CacheHitMicros+million-1)/million + (write*prices.CacheWriteMicros+million-1)/million
}

// ReserveCost 按最贵的输入类别预留。OpenAI 写缓存比普通输入贵，不能只按未命中价估算。
func (prices Prices) ReserveCost(input, output int64) int64 {
	prices.InputMicros = max(prices.InputMicros, prices.CacheWriteMicros)
	return prices.Cost(input, output)
}

// Reservation 在网络发送之前落库，原文已经脱敏，不能包含 Authorization 头。
type Reservation struct {
	MaximumTokens        int64  // 每次 HTTP 尝试的输入上界加最大输出，评测批次据此预留。
	CallID               string // 对应 Agent 发送前保存的请求步骤，不能由模型指定。
	DiagnosisID          string
	Model                string
	Request              json.RawMessage // 实际发出的 JSON 字节。
	MaximumMicros        int64           // 本次输入上界加最大输出的费用。
	DiagnosisLimitMicros int64           // 本次诊断所有尝试合计的上限，失败预留也计入。
	Prices               Prices          // 保存发送时的各档单价，以便调价后仍能复算旧账。
}

// Settlement 保存供应商回复与结算额；失败文本只用代码生成的简短原因。
type Settlement struct {
	Response   json.RawMessage // 已脱敏的接口回复；没有回复时为 null。
	Usage      Usage
	CostMicros int64  // 能确认时为实际用量对应的结算费用及未知预留额。
	Unknown    bool   // 网络中断等情况保留全部预留额。
	Status     string // ok、cancelled、http_429 等有限分类。
}

// Ledger 必须原子地检查总额并预留；两个同时运行的诊断不能各花掉同一份余额。
type Ledger interface {
	Reserve(context.Context, Reservation) (string, error)
	Settle(context.Context, string, Settlement) error
}
