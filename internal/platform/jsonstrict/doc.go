// Package jsonstrict 在 JSON 解码时拒绝重复字段、过深嵌套和尾随的第二份数据。
//
// 标准 encoding/json 会覆盖重复字段，Schema 校验发生在解码之后，已经无法发现
// 被覆盖的内容。因此这里保留一个很小的解析边界，不负责字段类型或业务规则。
// 数字保存为 json.Number，调用者可以校验另一份 float64 副本，再输出这份原值。
//
// 文件清单：
//   - doc.go：说明本包与 Schema 校验器的分工。
//   - decode.go：读取单份 JSON，保留数字文字并检查重复字段和深度。
//   - decode_test.go：覆盖重复键、嵌套边界、坏格式和数字保真。
//
// 阅读顺序：Decode → readValue。字节上限和 UTF-8 策略由调用方决定。
package jsonstrict
