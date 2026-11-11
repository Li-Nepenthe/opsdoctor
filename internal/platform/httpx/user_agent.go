// 本文件：RequestIDs 记录访问日志前，用 clientAgent 整理客户端自行填写的 User-Agent。
// 从 clientAgent 读起，看它怎样省略疑似凭据，并在缩短过长的字段时保留完整字符。

package httpx

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var sensitiveAgent = regexp.MustCompile(`(?i)sk-|bearer\s|(?:password|token|secret|api_key)\s*[:=]|eyJ[A-Za-z0-9_-]+\.`)

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
	// 长度按字节限制，落在一个汉字中间时要退到完整字符的边界。
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "…"
}
