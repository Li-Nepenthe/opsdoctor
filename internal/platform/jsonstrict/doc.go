// Package jsonstrict 在医生校验工具参数和报告之前检查 JSON。重名字段必须在解码时发现，结构校验由调用方接手。
//
//   - decode.go：解析原始输入，从 Decode 开始可以看到重复字段、嵌套深度和尾随数据在哪里被拒绝。
package jsonstrict
