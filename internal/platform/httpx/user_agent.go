// 本文件：把客户端类型作为有界、可安全编码的访问日志字段。
//
// User-Agent 是客户端自己填写的说明，常见内容是浏览器名字和版本。它也可能
// 是任意文字，不能当作可信指令。这里限制大小并省略疑似凭据，JSON 日志负责转义。
//
// 主要内容：clientAgent 截断过长文字，遇到疑似密钥或密码时省略整个字段值。
// 阅读建议：不读取其他请求头，更不记录 Authorization、Cookie 或请求正文。

package httpx

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// sensitiveAgent 只用于避免把误放在客户端说明里的凭据写进日志，不承担模型提示安全。
var sensitiveAgent = regexp.MustCompile(`(?i)sk-|bearer\s|(?:password|token|secret|api_key)\s*[:=]|eyJ[A-Za-z0-9_-]+\.`)

// clientAgent 保留不可信文字本身，但不能让它扩大每条日志或携带疑似认证信息。
func clientAgent(text string) string {
	if sensitiveAgent.MatchString(text) {
		return "[已省略疑似凭据]"
	}
	text = strings.ToValidUTF8(text, "�")
	const maximum = 512
	if len(text) <= maximum {
		return text
	}
	text = text[:maximum]
	// 截断不能把一个汉字切在中间，否则不同日志工具可能各自显示不同内容。
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "…"
}
