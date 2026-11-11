// 本文件：医生在校验工具参数和报告格式之前，先用 Decode 检查原始 JSON。
// 从 Decode 读起，readValue 在字段被覆盖前检查重名，并保留下游需要的数字原文。
// 将来改用标准库的重复字段检查时，还需保留这里的深度限制和数字精度。

package jsonstrict

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode 接受单份 JSON，保留数字原文。调用方负责限制输入字节数，并按入口要求检查 UTF-8。
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

func readValue(decoder *json.Decoder, depth int) (any, error) {
	// 沿用报告解析器的计数，根值在第零层，最多允许到第十六层。
	if depth > 16 {
		return nil, fmt.Errorf("JSON 嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON 在第 %d 字节附近格式错误：%v", decoder.InputOffset(), err)
	}
	switch token {
	case json.Delim('{'):
		value := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("JSON 在第 %d 字节附近对象字段不合法：%v", decoder.InputOffset(), err)
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("对象字段名必须是字符串")
			}
			// 同名字段一旦写进结果，前一个值就会消失。必须在这里拒绝重复，后面的 Schema 校验看不到它。
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
