// 本文件：处理 Alertmanager 推送的通知，验证身份后逐条记录告警摘要。请求头和完整正文不写日志。

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

// Handler 先验身份再读正文。认证时比较定长摘要，避免比较耗时随令牌共同前缀变化。
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
			// 通知来自外部，只写所需字段，防止控制字符或超长文本污染日志。
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
