// 本文件：阶段 3 临时接收 Alertmanager 的通知，证明告警管道已经接通。
//
// 它只验令牌并打印摘要，不创建诊断、不访问业务数据库。阶段 5 再把目标换成医生。
// 请求体有大小上限，日志只挑固定字段，不能把请求头中的令牌或整份正文打印出来。
//
// 主要内容：
//   - notification：只读取验收需要的分组和告警信息，其余 Alertmanager 字段允许忽略。
//   - Handler：验证 Bearer 令牌，解析通知，记录分组与告警状态。
//
// 阅读建议：认证在读正文之前执行。哈希后再比较，避免比较耗时随令牌共同前缀变化。

package alertsink

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// webhookBodyLimit 给一组普通告警足够空间，同时限制临时接收器的内存用量。
const webhookBodyLimit = 1 << 20

// notification 不保存注解正文，验收只需要确定哪个服务的哪个症状已经触发或恢复。
type notification struct {
	Status      string            `json:"status"`      // firing 表示仍在触发，resolved 表示已经恢复。
	GroupLabels map[string]string `json:"groupLabels"` // 分组依据为 alertname 和 service。
	Alerts      []struct {
		Status string            `json:"status"` // 同组里每条告警可能处于不同状态。
		Labels map[string]string `json:"labels"` // 只读取 alertname 和 service，不打印整个 map。
	} `json:"alerts"` // Alertmanager 已限制每批条数，接收侧仍限制正文总字节。
}

// Handler 由 app 注入令牌和日志器，测试可以直接发送内存请求，不需要启动 Alertmanager。
func Handler(token string, logger *slog.Logger) http.Handler {
	expected := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		actual := sha256.Sum256([]byte(request.Header.Get("Authorization")))
		if token == "" || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		defer request.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, webhookBodyLimit))
		var value notification
		if err := decoder.Decode(&value); err != nil || (value.Status != "firing" && value.Status != "resolved") || len(value.Alerts) == 0 {
			http.Error(writer, "invalid notification", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			// 一次通知只接受一个 JSON 值，避免前半段合法就忽略后面的大段内容。
			http.Error(writer, "invalid trailing content", http.StatusBadRequest)
			return
		}
		for _, alert := range value.Alerts {
			// 标签由本项目规则生成。仍裁掉控制字符与超长文本，日志只保留验收需要的摘要。
			logger.InfoContext(request.Context(), "收到告警通知", "status", safeLabel(alert.Status),
				"alertname", safeLabel(alert.Labels["alertname"]), "alert_service", safeLabel(alert.Labels["service"]))
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

// safeLabel 控制日志中每个外部字段的长度，错误通知不能无限撑大一条日志。
func safeLabel(value string) string {
	const maximumLabelBytes = 128 // 本项目服务与告警名很短，128 字节已足够。
	value = strings.Map(func(char rune) rune {
		if char < 32 || char == 127 {
			return -1
		}
		return char
	}, value)
	if len(value) > maximumLabelBytes {
		return value[:maximumLabelBytes]
	}
	return value
}
