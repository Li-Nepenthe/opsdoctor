// 本文件：限制执行计划工具接受的 SELECT 文本。
//
// 这里不是完整 SQL 解析器，而是只允许诊断常用的查询形状。拿不准的写法直接拒绝，
// 调用者可以改成更简单的 SELECT。数据库账号的只读权限仍然是另一道独立限制。
//
// 主要内容：
//   - SelectStatement：只保留一条 SELECT，拒绝注释、变量和可能执行额外动作的写法。
//   - sqlWords：跳过字符串和标识符里的文字，提取真正的 SQL 关键字。
//
// 阅读建议：先看禁止项，再看为什么不能只用 HasPrefix("SELECT")。

package source

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// SelectStatement 返回规范化后的单条 SELECT；工具和 MySQL 适配器都要调用。
func SelectStatement(statement string) (string, error) {
	text := strings.TrimSpace(statement)
	// 允许普通 SQL 末尾的一次分号，其余位置出现分号都拒绝，宁可少支持一种写法。
	text = strings.TrimSpace(strings.TrimSuffix(text, ";"))
	if len(text) == 0 || len(text) > 2048 || !utf8.ValidString(text) || strings.ContainsAny(text, ";@\x00") || strings.Contains(text, "--") || strings.Contains(text, "/*") || strings.Contains(text, "*/") || strings.Contains(text, "#") {
		return "", fmt.Errorf("只接受不含注释、变量和多条语句的 SELECT，最多 2048 字节")
	}
	words, err := sqlWords(text)
	if err != nil || len(words) == 0 || words[0] != "SELECT" {
		return "", fmt.Errorf("只接受 SELECT；不要提供 EXPLAIN、ANALYZE 或写语句")
	}
	for _, word := range words[1:] {
		switch word {
		case "INTO", "OUTFILE", "DUMPFILE", "FOR", "LOCK", "PROCEDURE", "ANALYZE", "EXPLAIN", "CALL", "INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "SET", "SLEEP", "BENCHMARK", "LOAD_FILE", "GET_LOCK", "RELEASE_LOCK":
			return "", fmt.Errorf("SELECT 含有执行计划工具不允许的语法")
		}
	}
	return text, nil
}

// sqlWords 识别引号边界，同时拒绝任意函数调用，避免让存储函数在优化阶段产生副作用。
func sqlWords(text string) ([]string, error) {
	var words []string
	for index := 0; index < len(text); {
		char := text[index]
		if char == '\'' || char == '"' || char == '`' {
			end, err := quotedEnd(text, index)
			if err != nil {
				return nil, err
			}
			index = end
			// 带引号的函数名也不能绕过普通函数白名单。
			if strings.HasPrefix(strings.TrimLeft(text[index:], " \t\r\n"), "(") {
				return nil, fmt.Errorf("不支持带引号的函数调用")
			}
			continue
		}
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_') {
			index++
			continue
		}
		start := index
		for index < len(text) {
			char = text[index]
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_') {
				break
			}
			index++
		}
		word := strings.ToUpper(text[start:index])
		words = append(words, word)
		if strings.HasPrefix(strings.TrimLeft(text[index:], " \t\r\n"), "(") {
			if strings.HasSuffix(strings.TrimRight(text[:start], " \t\r\n"), ".") {
				// schema.COUNT 可能是自定义函数，不能借同名内建函数进入白名单。
				return nil, fmt.Errorf("不支持限定名称的函数调用")
			}
			// 仅保留常见聚合和条件语法。不是函数的 IN/EXISTS 也可能紧跟括号。
			switch word {
			case "COUNT", "MIN", "MAX", "SUM", "AVG", "COALESCE", "IFNULL", "ABS", "ROUND", "IN", "EXISTS", "SELECT", "WHERE", "AND", "OR", "NOT", "ON", "HAVING":
			default:
				return nil, fmt.Errorf("不支持这个函数，请改成简单 SELECT")
			}
		}
	}
	return words, nil
}

// quotedEnd 跳过一段带引号的内容，避免把字符串里的词误当 SQL 指令。
func quotedEnd(text string, start int) (int, error) {
	quote := text[start]
	for index := start + 1; index < len(text); {
		// 反斜杠会转义下一个字符，不能把那个引号当作结束。
		if text[index] == '\\' {
			index += 2
			continue
		}
		if text[index] == quote {
			// 连续两个相同引号表示内容中的引号，仍没有结束这一段。
			if index+1 < len(text) && text[index+1] == quote {
				index += 2
				continue
			}
			return index + 1, nil
		}
		index++
	}
	return 0, fmt.Errorf("SQL 引号没有闭合")
}
