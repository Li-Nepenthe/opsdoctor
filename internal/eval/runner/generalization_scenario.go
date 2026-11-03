// 本文件：读取外部靶场的实验定义，并把故障开关与给医生的症状分开。
//
// 开关名称、标准答案和恢复内容只留在宿主机。医生只收到 symptoms 和故障时间。
// 查询表达式由已冻结的场景提供，用来确认真正出现了症状，而不是只确认开关已写入。
//
// 主要内容：loadGeneralizationCases 校验场景；demoHTTP 有界访问本机 Demo；
// changeDemoFlag 只改指定开关；waitDemoQuery 用真实指标验证故障或恢复。
// 阅读建议：先看 demoScenario，再对照 eval/scenarios/otel-demo/ 中的 YAML。

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/goccy/go-yaml"
)

// demoScenario 把公开出处、注入参数、真实验证与评分答案保存在同一个冻结文件里。
type demoScenario struct {
	ID                string           `yaml:"id"`                  // OT01–OT08，与 shop 编号分开。
	Title             string           `yaml:"title"`               // 给操作者看的实验标题。
	SourceURL         string           `yaml:"source_url"`          // 固定版本文档或官方说明的出处。
	Flag              string           `yaml:"flag"`                // Demo 已有的开关名，不接受脚本。
	Variant           string           `yaml:"variant"`             // 必须在上游开关的 variants 中存在。
	OverrideTargeting bool             `yaml:"override_targeting"`  // 只改三元 if 的命中分支，保留原条件。
	Symptoms          string           `yaml:"symptoms"`            // 只描述观测到的现象，不含开关或答案。
	Symptom           scenario.Symptom `yaml:"symptom"`             // 有限非空 PromQL 样本表示症状成立。
	HealthyPromQL     string           `yaml:"healthy_promql"`      // 恢复后的独立健康判断。
	GroundTruth       scenario.Truth   `yaml:"ground_truth"`        // 仅给评分器。
	Evidence          []string         `yaml:"evidence_milestones"` // 沿用阶段 8 的评委输入格式。
}

// loadGeneralizationCases 只读取明确选中的文件；拼错字段与文件名都在注入前报错。
func loadGeneralizationCases(directory string, names []string) (map[string]demoScenario, error) {
	values := map[string]demoScenario{}
	// 只读操作者明确选择的编号，新增一个 YAML 不会自动扩大本批实验。
	for _, name := range names {
		if !regexp.MustCompile(`^OT0[1-8]$`).MatchString(name) || values[name].ID != "" {
			return nil, fmt.Errorf("外部场景必须为不重复的 OT01–OT08")
		}
		raw, err := os.ReadFile(filepath.Join(directory, name+".yaml"))
		if err != nil {
			return nil, fmt.Errorf("读取外部场景 %s：%w", name, err)
		}
		value, err := decodeDemoScenario(raw)
		if err != nil || value.ID != name {
			return nil, fmt.Errorf("外部场景 %s 与文件名或契约不符：%v", name, err)
		}
		values[name] = value
	}
	return values, nil
}

// decodeDemoScenario 先解释必填内容；只有真实指标能证明故障已出现。
func decodeDemoScenario(raw []byte) (demoScenario, error) {
	var value demoScenario
	// 严格解码能发现字段拼错，避免把漏写的验证条件当作默认健康。
	if len(raw) > 64<<10 || yaml.UnmarshalWithOptions(raw, &value, yaml.Strict()) != nil {
		return value, fmt.Errorf("场景超过 64 KiB 或包含未知字段")
	}
	within, err := time.ParseDuration(value.Symptom.Within)
	if err != nil || within <= 0 || within > 3*time.Minute || value.Symptom.Healthy || value.Symptom.PromQL == "" || value.HealthyPromQL == "" {
		return value, fmt.Errorf("症状和健康查询不能为空，等待时限须为三分钟以内")
	}
	link, err := url.Parse(value.SourceURL)
	if err != nil || link.Scheme != "https" || (link.Hostname() != "opentelemetry.io" && link.Hostname() != "github.com") {
		return value, fmt.Errorf("场景需要官方 HTTPS 出处")
	}
	if !regexp.MustCompile(`^OT0[1-8]$`).MatchString(value.ID) || value.Title == "" || !regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,63}$`).MatchString(value.Flag) || value.Variant == "" || value.Symptoms == "" || len(value.Evidence) == 0 || value.GroundTruth.Component == "" || value.GroundTruth.Mechanism == "" || value.GroundTruth.Explanation == "" {
		return value, fmt.Errorf("场景编号、开关、症状、答案与证据不能为空")
	}
	return value, nil
}

// gradingScenario 复用原评分器的数据结构，注入控制参数不会进入评委或医生。
func (value demoScenario) gradingScenario() scenario.Scenario {
	return scenario.Scenario{ID: value.ID, Title: value.Title, GroundTruth: value.GroundTruth, Evidence: value.Evidence}
}

// demoHTTP 不携带医生令牌，不跟随跳转，响应最多一 MiB；只供本机实验端点使用。
func demoHTTP(ctx context.Context, method, address string, input any, output any) error {
	var raw []byte
	var err error
	// 通过 JSON 编码请求体，开关内容只是数据，不能变成 URL 或外部命令。
	if input != nil {
		raw, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("无法创建 Demo 请求")
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("外部靶场 Demo 接口未完成：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("外部靶场 Demo 接口返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return fmt.Errorf("外部靶场 Demo 响应超过限制或读取失败")
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return fmt.Errorf("外部靶场 Demo 响应格式不符")
	}
	return nil
}

// changeDemoFlag 深复制完整配置后只改一处；特定商品的条件不会因注入而丢失。
func changeDemoFlag(original json.RawMessage, value demoScenario) (json.RawMessage, error) {
	var document map[string]json.RawMessage
	// 每层都重新解码，保证保留未知字段，也不会修改 original 对应的字节。
	if json.Unmarshal(original, &document) != nil {
		return nil, fmt.Errorf("原开关文件格式不符")
	}
	var flags map[string]json.RawMessage
	if json.Unmarshal(document["flags"], &flags) != nil || flags[value.Flag] == nil {
		return nil, fmt.Errorf("上游配置没有指定开关")
	}
	var flag map[string]json.RawMessage
	if json.Unmarshal(flags[value.Flag], &flag) != nil {
		return nil, fmt.Errorf("上游开关格式不符")
	}
	var variants map[string]json.RawMessage
	if json.Unmarshal(flag["variants"], &variants) != nil || variants[value.Variant] == nil {
		return nil, fmt.Errorf("上游开关没有指定 variant")
	}
	variant, _ := json.Marshal(value.Variant)
	if value.OverrideTargeting {
		// 官方商品故障只影响命中条件的商品；删掉 targeting 会扩大故障范围。
		var targeting map[string]json.RawMessage
		var branches []json.RawMessage
		if json.Unmarshal(flag["targeting"], &targeting) != nil || json.Unmarshal(targeting["if"], &branches) != nil || len(branches) != 3 {
			return nil, fmt.Errorf("targeting 不是三元 if，不能改变匹配语义")
		}
		branches[1] = variant
		targeting["if"], _ = json.Marshal(branches)
		flag["targeting"], _ = json.Marshal(targeting)
	} else {
		flag["defaultVariant"] = variant
	}
	flags[value.Flag], _ = json.Marshal(flag)
	document["flags"], _ = json.Marshal(flags)
	return json.Marshal(document)
}

// readDemoFlags 读取完整上游配置，保证恢复时不会只把已知的五个开关写回去。
func readDemoFlags(ctx context.Context, base string) (json.RawMessage, error) {
	var value json.RawMessage
	err := demoHTTP(ctx, "GET", base+"/api/read", nil, &value)
	var document struct {
		Flags map[string]json.RawMessage `json:"flags"`
	}
	if err == nil && (json.Unmarshal(value, &document) != nil || len(document.Flags) == 0) {
		err = fmt.Errorf("外部 flagd 没有返回完整的 flags")
	}
	return value, err
}

// writeDemoFlags 等待异步写入读回一致；HTTP 200 本身不能证明开关已经生效。
func writeDemoFlags(parent context.Context, base string, data json.RawMessage) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	// 上游写接口使用异步消息，返回成功后还要等读接口看到同一份内容。
	if err := demoHTTP(ctx, "POST", base+"/api/write", map[string]any{"data": data}, nil); err != nil {
		return err
	}
	for {
		actual, err := readDemoFlags(ctx, base)
		if err != nil {
			return err
		}
		if equalJSON(actual, data) {
			return nil
		}
		if err = waitTick(ctx, time.Second); err != nil {
			return fmt.Errorf("外部 flagd 异步写入未得到读回确认：%w", err)
		}
	}
}

// demoQuery 只把有限样本视为条件成立；NaN、Inf 和接口错误不能当成健康。
func demoQuery(ctx context.Context, base, query string) (bool, error) {
	// 比较表达式过滤掉不成立的样本；缺样本或 NaN 不能证明健康或故障。
	var value struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := demoHTTP(ctx, "GET", base+"/api/v1/query?"+url.Values{"query": {query}}.Encode(), nil, &value); err != nil {
		return false, err
	}
	if value.Status != "success" {
		return false, fmt.Errorf("外部靶场 Demo Prometheus 查询失败")
	}
	for _, sample := range value.Data.Result {
		var number string
		if len(sample.Value) != 2 || json.Unmarshal(sample.Value[1], &number) != nil {
			return false, fmt.Errorf("外部靶场 Demo 指标样本格式不符")
		}
		parsed, err := strconv.ParseFloat(number, 64)
		if err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			return true, nil
		}
	}
	return false, nil
}

// waitDemoQuery 用调用方给定的原始时限等待，不因重启命令重新计算故障确认窗口。
func waitDemoQuery(parent context.Context, base, query string, deadline time.Time) error {
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	for {
		passed, err := demoQuery(ctx, base, query)
		if err != nil {
			return err
		}
		if passed {
			return nil
		}
		if err = waitTick(ctx, 5*time.Second); err != nil {
			return fmt.Errorf("外部靶场 Demo 指标条件未在原窗口内成立：%w", err)
		}
	}
}

// demoBase 拒绝含凭据、路径或跳转的地址；实验控制只能指向本机独立部署。
func demoBase(text string) (string, error) {
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "http" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("外部靶场 Demo 控制和验证地址只接受本机回环 HTTP 根地址")
	}
	return strings.TrimRight(text, "/"), nil
}
