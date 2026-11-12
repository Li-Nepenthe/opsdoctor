// 本文件：把 OpenSearch 命中项整理为统一日志条目。缺少服务或时间时返回错误，避免把来源不明的数据当作证据。

package opensearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// entries 不信任后端顺序和条数，也不会把别的服务混进当前诊断。
func entries(raw []byte, query source.LogQuery, fields Fields) ([]source.LogEntry, error) {
	var response struct {
		TimedOut bool `json:"timed_out"` // 部分查询超时也不能假装结果完整。
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // 纳秒大于浮点数精确整数范围，不能先经过 float64。
	if !json.Valid(raw) || decoder.Decode(&response) != nil || response.Hits == nil || response.Hits.Hits == nil {
		return nil, fmt.Errorf("OpenSearch 日志 JSON 格式不符合约定")
	}
	if response.TimedOut || response.Shards.Failed > 0 {
		return nil, fmt.Errorf("OpenSearch 日志查询超时或部分分片失败")
	}
	rows := []source.LogEntry{}
	for _, hit := range response.Hits.Hits {
		service, _ := fieldValue(hit.Source, fields.Service).(string)
		if service != query.Service {
			return nil, fmt.Errorf("OpenSearch 日志服务缺失或与查询不一致")
		}
		when, err := timestamp(fieldValue(hit.Source, fields.Timestamp))
		if err != nil {
			return nil, err
		}
		line := logLine(fieldValue(hit.Source, fields.Message))
		rows = append(rows, source.LogEntry{Time: when, Service: service, Line: line})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, _ := strconv.ParseInt(rows[i].Time, 10, 64)
		b, _ := strconv.ParseInt(rows[j].Time, 10, 64)
		return a > b
	})
	return rows[:min(query.Limit, len(rows))], nil
}

// logLine 保留结构化消息的内容；OpenTelemetry 的 body 也可以是对象或数字。
func logLine(value any) string {
	if line, ok := value.(string); ok {
		return line
	}
	if value == nil {
		return ""
	}
	// value 来自已成功解码的 JSON，必定能重新编码；不把对象默默变成空正文。
	raw, _ := json.Marshal(value)
	return string(raw)
}

// fieldValue 优先取完整键，随后尝试每个点号；keyword 是映射子字段，不在 _source 里。
func fieldValue(object map[string]any, path string) any {
	// 先找完整键，避免把采集器保留的 service.name 错拆成不存在的两层对象。
	if value, exists := object[path]; exists {
		return value
	}
	for index, char := range path {
		if char != '.' {
			continue
		}
		if child, ok := object[path[:index]].(map[string]any); ok {
			if value := fieldValue(child, path[index+1:]); value != nil {
				return value
			}
		}
	}
	if strings.HasSuffix(path, ".keyword") {
		return fieldValue(object, strings.TrimSuffix(path, ".keyword"))
	}
	return nil
}

// timestamp 接受日期文本和原始纳秒整数，避免把时间当浮点数而损失精度。
func timestamp(value any) (string, error) {
	switch actual := value.(type) {
	case string:
		if when, err := time.Parse(time.RFC3339Nano, actual); err == nil {
			return strconv.FormatInt(when.UnixNano(), 10), nil
		}
		if nanos, err := strconv.ParseInt(actual, 10, 64); err == nil {
			return strconv.FormatInt(nanos, 10), nil
		}
	case json.Number:
		if nanos, err := actual.Int64(); err == nil {
			return strconv.FormatInt(nanos, 10), nil
		}
	}
	return "", fmt.Errorf("OpenSearch 日志时间字段缺失或格式不合法")
}
