// 本文件：读取单份 JSON，保留数字文字并检查重复字段和深度。
//
// Go 正式启用 json/v2 后可以删掉这个包，把拒绝重复字段交给标准库；迁移时仍需
// 核对深度限制和数字保真。现在不启用实验特性，也不在这里重写 Schema 校验。
//
// 主要内容：Decode 只接收一份 JSON；readValue 逐层发现重复键并保留 json.Number。
// 阅读建议：报告需要保留原数字，因此不能在这一遍先把数字转换为 float64。

package jsonstrict

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode 返回保留数字原文的树；无效 UTF-8 沿用标准库行为，由需要拒绝它的调用方先检查。
func Decode(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("只接受一份 JSON")
	}
	return value, nil
}

// readValue 与旧报告解析器使用相同的深度计数，根值为第零层。
func readValue(decoder *json.Decoder, depth int) (any, error) {
	// 逐层读取才能在内容被 map 覆盖之前发现重复字段。
	if depth > 16 {
		return nil, fmt.Errorf("JSON 嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON 在第 %d 字节附近格式错误", decoder.InputOffset())
	}
	switch token {
	case json.Delim('{'):
		value := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("JSON 在第 %d 字节附近对象字段不合法", decoder.InputOffset())
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("对象字段名必须是字符串")
			}
			if _, exists := value[name]; exists {
				return nil, fmt.Errorf("字段 %s 重复", name)
			}
			item, err := readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value[name] = item
		}
		_, err = decoder.Token()
		return value, err
	case json.Delim('['):
		value := []any{}
		for decoder.More() {
			item, err := readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, item)
		}
		_, err = decoder.Token()
		return value, err
	default:
		return token, nil
	}
}
