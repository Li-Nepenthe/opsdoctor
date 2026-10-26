// 本文件：检查 Alertmanager 通知，并把合法的分组交给仓储保存。
//
// Alertmanager 会重复推送同一分组，网络失败后也会重发。HTTP 成功响应必须在
// 数据库提交之后发出，才能避免“已经告诉发送方成功，任务却没有保存”。
//
// 主要内容：
//   - Notification、Alert：接收阶段真正需要的字段。
//   - Receiver、Service：由使用方定义保存接口，让通知校验不依赖 MySQL。
//   - Receive、validate：限制通知规模，确认分组与告警身份，再保存。
//
// 阅读建议：令牌验证在 httpapi 中完成；这里拿到的 target 来自已认证的路由。

package alerts

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// Alert 保存证据快照；Labels、Annotations 是外部文字，以后也不能把它们当指令。
type Alert struct {
	Status      string            `json:"status"`      // firing 或 resolved。
	Labels      map[string]string `json:"labels"`      // 告警名和服务等标签。
	Annotations map[string]string `json:"annotations"` // 人写的症状说明。
	StartsAt    time.Time         `json:"startsAt"`    // 本次告警开始时间。
	EndsAt      time.Time         `json:"endsAt"`      // 未恢复时可能为零时间。
	Fingerprint string            `json:"fingerprint"` // Alertmanager 根据标签计算的稳定标识。
}

// Notification 的 GroupKey 是推送方的分组键，仓储还会加上系统名形成去重身份。
type Notification struct {
	Status      string            `json:"status"`      // 整组当前状态。
	GroupKey    string            `json:"groupKey"`    // 同组重发时保持一致。
	GroupLabels map[string]string `json:"groupLabels"` // 用来显示任务标题。
	Alerts      []Alert           `json:"alerts"`      // 同组中各告警的最新快照。
}

// Receipt 说明通知已保存或被忽略，不表示诊断已经执行。
type Receipt struct {
	DiagnosisID string `json:"diagnosis_id,omitempty"` // 新建或复用的 pending 任务。
	Created     bool   `json:"created"`                // false 可表示一次重复推送。
	Ignored     bool   `json:"ignored"`                // 只有恢复通知且没有现有任务时，不凭空创建任务。
}

// Receiver 定义在通知处理这一使用方，MySQL 仓储只需实现它，不需要显式 implements。
type Receiver interface {
	ReceiveAlerts(context.Context, string, Notification) (Receipt, error)
}

// Service 不保留某个“默认系统”，每次身份都由经过认证的接口传入。
type Service struct{ receiver Receiver } // 持久化操作由装配时注入。

// New 让测试可以替换仓储，同时生产路径仍只有同一个通知校验入口。
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

// Error 让 Invalid 可作为标准 error 返回，接口层通过 errors.As 判断分类。
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
	text := value.GroupLabels["alertname"]
	if service := value.GroupLabels["service"]; service != "" {
		text = fmt.Sprintf("%s · %s", text, service)
	}
	if text == "" {
		text = "收到告警分组"
	}
	runes := []rune(text)
	if len(runes) > 256 {
		text = string(runes[:256])
	}
	return text
}
