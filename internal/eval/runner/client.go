// 本文件：用人的登录身份读取医生状态和本次告警，不直接访问靶场数据源。
//
// 凭据只在登录正文和 Authorization 头里使用，不进 URL、参数打印或结果表。
// 评测任务仍由医生调度器执行，因此工具、脱敏和费用记录都走原有路径。
//
// 主要内容：newDoctorClient 登录；request 限制 HTTP 响应并拒绝重定向；
// runtime、fault、alerts 和 cancelDiagnosis 分别读取状态、等待开放期、抓告警和取消。
// 阅读建议：客户端失败只给出固定说明，不把登录响应或请求正文带进错误。

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// doctorClient 的令牌只留在本进程，不写入评测配置。
type doctorClient struct {
	refreshAt   time.Time                                    // 长跑前主动续登录，不把一小时登录过期误记成模型失败。
	renew       func(context.Context) (*doctorClient, error) // 只读取原来的环境凭据，不落盘。
	base, token string
	http        *http.Client
}

// runtimeStatus 同时核对模型、模式和当前币种，不能仅看宿主机环境变量。
type runtimeStatus struct {
	Executions map[string]run.ExecutionStatus `json:"executions"` // 配对前核对每个模型和独立账户。
	Provider   string                         `json:"provider"`
	Model      string                         `json:"model"`
	Budget     run.BudgetStatus               `json:"budget"`
}

// newDoctorClient 不读取任何模型 Key，登录仅用于已有的医生 API。
func newDoctorClient(ctx context.Context, reader config.Reader) (*doctorClient, error) {
	// 登录密码只发给明确配置的安全地址，不跟随跳转到另一个服务。
	base, err := reader.String("DOCTOR_URL", "http://127.0.0.1:8090")
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"))) {
		return nil, fmt.Errorf("DOCTOR_URL 只接受 HTTPS 或本机回环 HTTP，不允许凭据、查询串或片段")
	}
	email, err := reader.String("DOCTOR_ADMIN_EMAIL", "")
	if err != nil {
		return nil, err
	}
	password, err := reader.String("DOCTOR_ADMIN_PASSWORD", "")
	if err != nil {
		return nil, err
	}
	client := &doctorClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	raw, _ := json.Marshal(map[string]string{"email": email, "password": password})
	var login struct {
		Token string `json:"access_token"`
	}
	if err = client.request(ctx, "POST", "/api/v1/auth/login", raw, &login); err != nil {
		return nil, err
	}
	if login.Token == "" {
		return nil, fmt.Errorf("医生登录没有返回令牌")
	}
	client.token = login.Token
	client.refreshAt = time.Now().Add(40 * time.Minute)
	client.renew = func(ctx context.Context) (*doctorClient, error) { return newDoctorClient(ctx, reader) }
	return client, nil
}

// request 的正文有界；遇到非成功响应不输出可能夹带认证信息的服务端原文。
func (client *doctorClient) request(ctx context.Context, method, path string, body []byte, output any) error {
	if client.renew != nil && !time.Now().Before(client.refreshAt) {
		renewed, err := client.renew(ctx)
		if err != nil {
			return err
		}
		client.token, client.refreshAt = renewed.token, renewed.refreshAt
	}
	// 错误正文可能含凭据或模型原文，调用方只需要状态码来决定是否继续。
	request, err := http.NewRequestWithContext(ctx, method, client.base+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("无法创建医生请求")
	}
	request.Header.Set("Content-Type", "application/json")
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("医生接口未完成，请检查服务是否已启动")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("医生接口返回 HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope) != nil || envelope.Code != "OK" {
		return fmt.Errorf("医生没有返回有效结果")
	}
	if output == nil {
		return nil
	}
	if json.Unmarshal(envelope.Data, output) != nil {
		return fmt.Errorf("医生结果格式不符合约定")
	}
	return nil
}

// runtime 读取正在运行的配置，防止评测器以为是 Terra、医生却仍在 Mock。
func (client *doctorClient) runtime(ctx context.Context) (runtimeStatus, error) {
	var value runtimeStatus
	err := client.request(ctx, "GET", "/api/v1/status", nil, &value)
	return value, err
}

// fault 通过用户指定的系统列表接口检查开放期，不用固定 sleep 猜告警是否已恢复。
func (client *doctorClient) fault(ctx context.Context, target string) (store.FaultState, store.EvaluationMode, error) {
	var targets []struct {
		Name       string               `json:"name"`
		Fault      store.FaultState     `json:"fault"`
		Evaluation store.EvaluationMode `json:"evaluation"`
	}
	if err := client.request(ctx, "GET", "/api/v1/targets", nil, &targets); err != nil {
		return store.FaultState{}, store.EvaluationMode{}, err
	}
	for _, value := range targets {
		if value.Name == target {
			return value.Fault, value.Evaluation, nil
		}
	}
	return store.FaultState{}, store.EvaluationMode{}, fmt.Errorf("医生没有接入指定系统")
}

// alerts 经过医生现有注册表；失败或截断时停止试跑，不拿残缺输入当完整对照。
func (client *doctorClient) alerts(ctx context.Context, target string) (json.RawMessage, error) {
	var result tools.Result
	if err := client.request(ctx, "POST", "/api/v1/targets/"+url.PathEscape(target)+"/tools/list_firing_alerts/run", []byte(`{}`), &result); err != nil {
		return nil, err
	}
	if result.Status != "ok" || result.Truncated {
		return nil, fmt.Errorf("试跑告警查询失败或截断，请先核对输入")
	}
	return result.Data, nil
}

// cancelDiagnosis 用原来的取消入口，中断信号会继续传到在途模型 HTTP 请求。
func (client *doctorClient) cancelDiagnosis(ctx context.Context, id string) error {
	return client.request(ctx, "POST", "/api/v1/diagnoses/"+url.PathEscape(id)+"/cancel", []byte(`{}`), nil)
}
