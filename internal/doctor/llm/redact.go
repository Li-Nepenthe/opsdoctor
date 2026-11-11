// 本文件：清理告警、工具结果以及其余请求内容中的密钥和令牌。审计和发送共用脱敏后的 JSON，规则只覆盖已知秘密及密钥形状。

package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Redactor 构造后只读，多个诊断可以共用；known 额外覆盖本机明确知道的密钥值。
type Redactor struct {
	rules []*regexp.Regexp // 不依赖语言模型判断的固定形状。
	known []string         // 只在内存中用于替换，不能打印该结构。
}

// NewRedactor 把规则编译一次，避免每个工具结果都重新编译正则。
func NewRedactor(known ...string) *Redactor {
	redactor := &Redactor{}
	for _, expression := range []string{
		`(?i)\bsk-[a-z0-9_-]{8,}`,
		`(?i)\bBearer\s+[a-z0-9._~+/=-]+`,
		`\beyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`,
		`(?i)\b(?:password|passwd|api[_-]?key|access[_-]?token|secret)\s*[:=]\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;}&]+)`,
	} {
		redactor.rules = append(redactor.rules, regexp.MustCompile(expression))
	}
	for _, secret := range known {
		// 太短的值会误伤普通文字；配置本身要求真正密钥有足够长度。
		if len(secret) >= 8 {
			redactor.known = append(redactor.known, secret)
		}
	}
	return redactor
}

// Text 先替换已知值，再替换常见形状；多次调用的结果相同，方便在边界重复保护。
func (redactor *Redactor) Text(text string) string {
	// 日志字段有时装着另一份 JSON；先按结构处理，才能识别 {"password":"..."}。
	trimmed := strings.TrimSpace(text)
	if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(trimmed)) {
		clean, err := redactor.JSON([]byte(trimmed))
		if err == nil {
			return string(clean)
		}
	}
	for _, secret := range redactor.known {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	for _, rule := range redactor.rules {
		text = rule.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}

// JSON 按值递归处理，不直接在编码字节里替换引号，避免破坏合法 JSON。
func (redactor *Redactor) JSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // 大整数保持原样，不能先转 float64 再丢精度。
	var value any
	if decoder.Decode(&value) != nil {
		return nil, fmt.Errorf("脱敏输入不是合法 JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("脱敏输入只能包含一份 JSON")
	}
	return json.Marshal(redactor.clean(value, 0))
}

// clean 只改字符串值，字段名由协议规定；嵌在 content 里的 JSON 文字再按形状清理。
func (redactor *Redactor) clean(value any, depth int) any {
	if depth > 64 {
		return "[内容嵌套过深]"
	}
	switch typed := value.(type) {
	case string:
		return redactor.Text(typed)
	case []any:
		for index := range typed {
			typed[index] = redactor.clean(typed[index], depth+1)
		}
	case map[string]any:
		for key, item := range typed {
			// 对象里的 password 字段可能只有值，不会出现 password= 的文本形状。
			lower := strings.ToLower(key)
			if lower == "password" || lower == "api_key" || lower == "access_token" || lower == "authorization" || lower == "secret" {
				typed[key] = "[REDACTED]"
			} else {
				typed[key] = redactor.clean(item, depth+1)
			}
		}
	}
	return value
}
