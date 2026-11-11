// 本文件：推送方可能重发同一分组。
// Receive 使用已认证路由传来的系统名，在仓储保存后才向 HTTP 层返回成功。

package alerts

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// Alert 保存外部证据。标签和说明中的文字只能作为数据处理。
type Alert struct {
	Status      string            `json:"status"` // firing 或 resolved。
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`      // 未恢复时可能为零时间。
	Fingerprint string            `json:"fingerprint"` // Alertmanager 根据标签计算的稳定标识。
}

// Notification 的 GroupKey 是推送方的分组键，仓储还会加上系统名形成去重身份。
type Notification struct {
	Status      string            `json:"status"`
	GroupKey    string            `json:"groupKey"`    // 同组重发时保持一致。
	GroupLabels map[string]string `json:"groupLabels"` // 用来显示任务标题。
	Alerts      []Alert           `json:"alerts"`
}

// Receipt 说明通知已保存或被忽略，不表示诊断已经执行。
type Receipt struct {
	Suppressed       bool   `json:"suppressed"`             // 评测期间只捕获通知，不运行自动诊断。
	RecoveryFollowup bool   `json:"recovery_followup"`      // 观察期内重新出现的症状，只补证据，不重新执行报告。
	DiagnosisID      string `json:"diagnosis_id,omitempty"` // 新建或复用的 pending 任务。
	Created          bool   `json:"created"`                // false 可表示一次重复推送。
	Ignored          bool   `json:"ignored"`                // 只有恢复通知且没有现有任务时，不凭空创建任务。
}

// Receiver 负责持久化通知并处理同一系统分组的重发。
type Receiver interface {
	ReceiveAlerts(context.Context, string, Notification) (Receipt, error)
}

// Service 不保留某个“默认系统”，每次身份都由经过认证的接口传入。
type Service struct{ receiver Receiver }

// New 将通知校验接到统一的仓储入口。
func New(receiver Receiver) *Service { return &Service{receiver: receiver} }

// Receive 校验错误与存储错误分开，HTTP 层才能分别返回 400 和可重试的 503。
func (service *Service) Receive(ctx context.Context, target string, value Notification) (Receipt, error) {
	if err := validate(value); err != nil {
		return Receipt{}, err
	}
	// 保存通知也有总预算，数据库拥塞时让发送方重试，不能无期限占着 HTTP 请求。
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return service.receiver.ReceiveAlerts(bounded, target, value)
}

// Invalid 表示通知格式错误；错误文字由代码生成，不包含请求正文。
type Invalid string

// Error 返回适合在 HTTP 响应中说明的格式错误。
func (err Invalid) Error() string { return string(err) }

// validate 不信任推送内容的条数和字段长度，认证通过也不意味着可以无限占用数据库。
func validate(value Notification) error {
	if (value.Status != "firing" && value.Status != "resolved") || value.GroupKey == "" || len(value.GroupKey) > 2048 {
		return Invalid("告警状态或分组键不合法")
	}
	if len(value.Alerts) == 0 || len(value.Alerts) > 64 || !validLabels(value.GroupLabels) {
		return Invalid("告警分组条数或标签不合法")
	}
	seen := map[string]bool{}
	for _, alert := range value.Alerts {
		// 整组恢复必须真的没有 firing，不能让一条错误摘要提前结束故障开放期。
		if value.Status == "resolved" && alert.Status != "resolved" {
			return Invalid("整组 resolved 仍包含 firing 告警")
		}
		// 指纹必须来自标签身份，不能在一次通知里重复覆盖同一项。
		if !regexp.MustCompile(`^[a-f0-9]{16}$`).MatchString(alert.Fingerprint) || seen[alert.Fingerprint] {
			return Invalid("告警指纹不合法或重复")
		}
		seen[alert.Fingerprint] = true
		if (alert.Status != "firing" && alert.Status != "resolved") || alert.StartsAt.IsZero() || alert.Labels["alertname"] == "" {
			return Invalid("告警缺少状态、时间或名称")
		}
		if !validLabels(alert.Labels) || !validLabels(alert.Annotations) {
			return Invalid("告警标签或说明过长")
		}
	}
	return nil
}

// validLabels 限制单字段和数量，整体正文另由 HTTP 的一 MiB 上限限制。
func validLabels(values map[string]string) bool {
	if len(values) > 32 {
		return false
	}
	for key, value := range values {
		if key == "" || len(key) > 128 || len(value) > 4096 {
			return false
		}
	}
	return true
}

// Title 只用于列表展示，数据库身份始终是系统名与分组键，不用可变的标题去重。
func Title(value Notification) string {
	// 取分组内告警名排序，重发时顺序改变也不会改变标题。
	names := []string{}
	for _, alert := range value.Alerts {
		if name := alert.Labels["alertname"]; name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "收到告警分组"
	}
	slices.Sort(names)
	// 给数量后缀留出空间，数据库 title 最多保存 256 个字符。
	name := []rune(names[0])
	text := string(name[:min(len(name), 220)])
	if len(value.Alerts) > 1 {
		text = fmt.Sprintf("%s 等 %d 条告警", text, len(value.Alerts))
	}
	return text
}
