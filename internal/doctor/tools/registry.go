// 本文件：让所有工具共用参数校验、十秒预算和四千字节结果限制。
//
// HTTP、命令行和以后的 Agent 都调用 Run。不能为某个入口另开一条直接访问数据源的路，
// 否则同一个工具会有两套权限边界。
//
// 主要内容：
//   - Registry、Definition：保存工具名字、说明、参数结构和执行函数。
//   - Run：先检查参数，再带截止时间调用工具，最后整理成有界结果。
//   - finish：保留原始大小和截断标志，不能悄悄丢掉内容却声称结果完整。
//
// 阅读建议：从 Run 顺着读，校验失败也返回 Result，不需要调用者处理 panic。

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
	ReadOnly       bool   `json:"read_only"`       // 本阶段的工具全部只读。
	TimeoutSeconds int    `json:"timeout_seconds"` // 供调用者了解预算，不是可修改参数。
	Parameters     Schema `json:"parameters"`      // 与运行时校验共用同一份字段定义。
}

// Result 的失败也是普通结果。以后模型拿到它可以改参数，进程不用退出。
type Result struct {
	Tool          string          `json:"tool"`           // 实际调用的工具名。
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
	data    any    // 各工具已经按用途做过的摘要。
	summary string // 不依赖模型生成的说明。
}

// tool 把公开说明与内部实现放在一起，调用者只会拿到 Definition 的副本。
type tool struct {
	definition Definition                                       // 用同一份 Schema 做说明和校验。
	run        func(context.Context, arguments) (output, error) // 只能捕获构造时绑定的数据源。
}

// Registry 创建完成后不再增删工具，不能在一次调用中临时塞进高权限操作。
type Registry struct{ tools map[string]tool }

// List 按名字排序，保证命令行输出和测试结果不受 Go map 遍历顺序影响。
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

// Run 的具名返回值让 defer 能统一记录耗时，也能把意外 panic 转成固定错误结果。
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
	args, err := item.definition.Parameters.validate(raw)
	if err != nil {
		result.Status = "invalid_argument"
		result.Summary = err.Error()
		return
	}
	value, err := item.run(bounded, args)
	if bounded.Err() != nil {
		err = bounded.Err()
	}
	if err != nil {
		setFailure(&result, err)
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
	return
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
