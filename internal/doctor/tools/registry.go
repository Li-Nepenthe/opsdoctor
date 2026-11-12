// 本文件：让 HTTP、命令行和 Agent 共用一个只读工具执行入口。
// Run 检查工具名、参数和调用时限，再给出至多 4 KiB 的 JSON 结果，所有入口都要遵守同一套限制。

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// 全部入口共用这些常量，命令行参数不能扩大执行预算。
const (
	Timeout            = 10 * time.Second // 等连接、查后端和整理结果共用这一份时间。
	MaximumResultBytes = 4 << 10          // 计算完整 Result JSON，而不是只计算正文。
)

// ErrInvalidArgument 让工具的语义检查也能返回与结构检查相同的参数错误。
var ErrInvalidArgument = errors.New("参数不合法")

// Definition 是公开的工具说明，不包含数据源地址、账号或执行函数。
type Definition struct {
	Name           string `json:"name"`            // 稳定工具名，调用时精确匹配。
	Description    string `json:"description"`     // 解释能查到什么，不给故障预设答案。
	ReadOnly       bool   `json:"read_only"`       // 注册表公开的工具都用于只读取证。
	TimeoutSeconds int    `json:"timeout_seconds"` // 供调用者了解预算，不是可修改参数。
	Parameters     Schema `json:"parameters"`      // 与运行时校验共用同一份字段定义。
}

// Result 把查询失败也作为一次工具结果返回，调用者可以据此调整下一步。
type Result struct {
	Tool          string          `json:"tool"`
	Status        string          `json:"status"`         // ok、invalid_argument、timeout、cancelled、unavailable、internal。
	Summary       string          `json:"summary"`        // 简短说明结果数量或失败原因。
	Data          json.RawMessage `json:"data"`           // 合法 JSON；超长时改成带 preview 的对象。
	Truncated     bool            `json:"truncated"`      // true 表示内容不完整，不能作全量结论。
	OriginalBytes int             `json:"original_bytes"` // 工具整理后、最终截断前的 Data JSON 字节数。
	Bytes         int             `json:"bytes"`          // 完整 Result 紧凑 JSON 的实际大小。
	DurationMS    int64           `json:"duration_ms"`    // 从开始校验到完成工具调用的耗时。
}

// output 是内部结果，先保留结构化数据，最后才统一编码和截断。
type output struct {
	data    any
	summary string // 不依赖模型生成的说明。
}

// tool 把公开说明与内部实现放在一起，调用者只会拿到 Definition 的副本。
type tool struct {
	definition Definition                                       // 用同一份 Schema 做说明和校验。
	parameters *parameterValidator                              // 构建菜单时解析规则，调用时只做校验。
	run        func(context.Context, arguments) (output, error) // 只能捕获构造时绑定的数据源。
}

// Registry 创建完成后不再增删工具，不能在一次调用中临时塞进高权限操作。
type Registry struct{ tools map[string]tool }

// List 按名称稳定排序，使同一系统的工具在各次调用中保持相同顺序。
func (registry *Registry) List() []Definition {
	definitions := make([]Definition, 0, len(registry.tools))
	for _, item := range registry.tools {
		// JSON 往返得到独立副本，外部修改参数说明不会顺便修改运行时的校验规则。
		data, _ := json.Marshal(item.definition)
		var definition Definition
		_ = json.Unmarshal(data, &definition)
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions
}

// Run 记录整次调用的耗时。参数错误和后端异常都以有大小上限的结果返回给调用者。
func (registry *Registry) Run(ctx context.Context, name string, raw json.RawMessage) (result Result) {
	started := time.Now()
	result = Result{Tool: name, Status: "internal", Summary: "工具未完成", Data: json.RawMessage(`null`)}
	defer func() {
		if recover() != nil {
			// panic 可能夹带凭据或原始日志，不打印它，只保留固定分类。
			result = Result{Tool: name, Status: "internal", Summary: "工具执行发生内部错误", Data: json.RawMessage(`null`)}
		}
		result.DurationMS = time.Since(started).Milliseconds()
		finish(&result)
	}()
	// 连工具名也限制长度，未知名字不能绕过四千字节结果预算。
	if len(name) > 64 {
		result.Tool = ""
		result.Status = "invalid_argument"
		result.Summary = "工具名过长"
		return
	}
	item, ok := registry.tools[name]
	if !ok {
		result.Status = "invalid_argument"
		result.Summary = "该系统没有这个只读工具"
		return
	}
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	item.execute(bounded, raw, &result)
	return
}

func (item tool) execute(ctx context.Context, raw json.RawMessage, result *Result) {
	args, err := item.parameters.validate(raw)
	if err != nil {
		result.Status = "invalid_argument"
		result.Summary = err.Error()
		return
	}
	value, err := item.run(ctx, args)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		setFailure(result, err)
		return
	}
	data, err := json.Marshal(value.data)
	if err != nil {
		result.Summary = "工具结果无法编码"
		return
	}
	result.Status = "ok"
	result.Summary = value.summary
	result.Data = data
}

// setFailure 只放行适配器明确筛选的查询错误；其他后端错误可能带地址和凭据。
func setFailure(result *Result, err error) {
	var queryError *source.QueryError
	// 取消和超时优先：用户已经停止诊断时，不再把旧的数据库错误当作新的修正建议。
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Status = "timeout"
		result.Summary = "工具十秒预算已用尽，或调用方的更短预算已用尽"
	case errors.Is(err, context.Canceled):
		result.Status = "cancelled"
		result.Summary = "调用方已取消工具"
	case errors.Is(err, ErrInvalidArgument):
		result.Status = "invalid_argument"
		result.Summary = err.Error()
	case errors.As(err, &queryError):
		result.Status = "unavailable"
		result.Summary = queryError.Message
	default:
		result.Status = "unavailable"
		result.Summary = "数据源查询未完成；请检查连接、权限或缩小查询范围"
	}
}

// finish 不截断 JSON 字节流，而是把保留部分放进 preview 字符串，仍返回合法 JSON。
func finish(result *Result) {
	result.OriginalBytes = len(result.Data)
	if len(encoded(result)) <= MaximumResultBytes {
		return
	}
	original := string(result.Data)
	result.Truncated = true
	result.Summary += fmt.Sprintf("；完整数据 %d 字节，下面只保留开头，不能视为全量", len(original))
	if len(result.Summary) > 768 {
		result.Summary = prefix(result.Summary, 768)
	}
	keep := min(len(original), MaximumResultBytes)
	for {
		preview := struct {
			Preview string `json:"preview"`
		}{prefix(original, keep)}
		result.Data, _ = json.Marshal(preview)
		data := encoded(result)
		if len(data) <= MaximumResultBytes {
			return
		}
		// JSON 转义会增加长度，按实际超出的字节数继续缩短，不猜测中文字符占几字节。
		keep = max(0, keep-(len(data)-MaximumResultBytes)-1)
	}
}

// encoded 把大小字段本身也计入总大小；数字位数改变后再算一次即可收敛。
func encoded(result *Result) []byte {
	var data []byte
	for range 4 {
		data, _ = json.Marshal(result)
		if result.Bytes == len(data) {
			return data
		}
		result.Bytes = len(data)
	}
	data, _ = json.Marshal(result)
	return data
}

// prefix 不切开一个 UTF-8 字符，避免截断后出现乱码或无效 JSON 文本。
func prefix(text string, bytes int) string {
	bytes = min(bytes, len(text))
	for bytes > 0 && !utf8.ValidString(text[:bytes]) {
		bytes--
	}
	return text[:bytes]
}
