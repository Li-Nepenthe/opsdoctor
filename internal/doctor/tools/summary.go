// 本文件：把长时间线和重复的调用步骤整理成可阅读的证据摘要。
//
// 摘要先保留数量、最后值、极值和关系，最后的 4KiB 限制才负责裁掉仍然过长的正文。
// 不能只把原始 JSON 从中间切断，否则读者既不知道丢了多少，也可能拿不到合法 JSON。
//
// 主要内容：
//   - summarizeMetrics、summarizeTable：保留指标和元数据的关键字段。
//   - summarizeTraces、summarizeTree：分别显示最慢步骤和父子关系。
//   - recentChanges：从启动时间与版本指标推导近期变化，不访问 Docker。
//
// 阅读建议：先看摘要里明确保留的数量字段，再看缺失父节点和采样时间的限制。

package tools

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// metricSummary 保留范围中的极值和最后值，不把几百个重复采样点全部打印出来。
type metricSummary struct {
	Labels  map[string]string `json:"labels"`        // 用来区分服务或接口。
	Points  int               `json:"points"`        // 参与本次摘要的采样数量。
	Last    string            `json:"last"`          // 最后一个值可能是 NaN，保留原样。
	Minimum *float64          `json:"min,omitempty"` // 只统计有限数字。
	Maximum *float64          `json:"max,omitempty"` // 指针让“没有有效数字”不同于零。
}

// summarizeMetrics 的 min/max 来自完整的有界采样，不从最终截断的预览反算。
func summarizeMetrics(result source.MetricResult) output {
	rows := make([]metricSummary, 0, len(result.Series))
	for _, series := range result.Series {
		row := metricSummary{Labels: series.Labels, Points: len(series.Points)}
		for _, point := range series.Points {
			row.Last = point.Value
			value, err := strconv.ParseFloat(point.Value, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			if row.Minimum == nil || value < *row.Minimum {
				copy := value
				row.Minimum = &copy
			}
			if row.Maximum == nil || value > *row.Maximum {
				copy := value
				row.Maximum = &copy
			}
		}
		rows = append(rows, row)
	}
	return output{data: struct {
		Series  []metricSummary `json:"series"`  // 每条时间线的摘要。
		Limited bool            `json:"limited"` // 后端边界造成的省略，与最终输出截断分别说明。
	}{rows, result.Limited}, summary: fmt.Sprintf("%d 条序列；展示最后值及范围内极值，limited 表示未取得全部数据", len(rows))}
}

// summarizeTable 的说明明确这些是计划或元数据，不能把表统计中的估计值当精确行数。
func summarizeTable(table source.Table, description string) output {
	return output{data: table, summary: fmt.Sprintf("%s；%d 行元数据，limited=%t", description, len(table.Rows), table.Limited)}
}

// slowTrace 只保留最慢几步，其余步骤可通过同一个 ID 继续查。
type slowTrace struct {
	ID        string        `json:"trace_id"`      // get_trace 的参数。
	SpanCount int           `json:"span_count"`    // 整条调用链实际采集到的步骤数。
	Services  []string      `json:"services"`      // 实际出现的服务，不靠系统说明书猜测。
	Slowest   []source.Span `json:"slowest_spans"` // 最多五个步骤。
}

// summarizeTraces 复制 span 切片再排序，不改动数据源可能复用的响应对象。
func summarizeTraces(traces []source.Trace) output {
	// 先复制再排序，不能因为做一份摘要而改变原始调用链的步骤顺序。
	rows := make([]slowTrace, 0, len(traces))
	for _, trace := range traces {
		spans := append([]source.Span(nil), trace.Spans...)
		sort.SliceStable(spans, func(i, j int) bool { return spans[i].Duration > spans[j].Duration })
		services := make(map[string]bool)
		for _, span := range spans {
			services[span.Service] = true
		}
		names := make([]string, 0, len(services))
		for name := range services {
			names = append(names, name)
		}
		sort.Strings(names)
		rows = append(rows, slowTrace{trace.ID, len(trace.Spans), names, spans[:min(5, len(spans))]})
	}
	return output{data: rows, summary: fmt.Sprintf("%d 条调用链，每条只显示最慢五步；父步骤耗时包含子步骤，不能直接相加", len(rows))}
}

// summarizeTree 给丢失父节点和环路留出明确标记，不能因为后端数据不完整而无限递归。
func summarizeTree(trace source.Trace) output {
	// 先索引步骤，再连接父子关系；最后用 seen 防止坏数据中的环路让递归停不下来。
	byID := make(map[string]source.Span)
	children := make(map[string][]string)
	for _, span := range trace.Spans {
		byID[span.ID] = span
	}
	var roots []string
	for _, span := range trace.Spans {
		if _, exists := byID[span.ParentID]; span.ParentID == "" || !exists {
			roots = append(roots, span.ID)
		} else {
			children[span.ParentID] = append(children[span.ParentID], span.ID)
		}
	}
	less := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool { return byID[ids[i]].Start < byID[ids[j]].Start })
	}
	less(roots)
	for _, ids := range children {
		less(ids)
	}
	var lines []string
	seen := make(map[string]bool)
	var walk func(string, int)
	walk = func(id string, depth int) {
		if seen[id] || len(seen) >= 200 || depth > 32 {
			return
		}
		seen[id] = true
		span := byID[id]
		line := treeLine(span, depth, byID)
		lines = append(lines, line)
		for _, child := range children[id] {
			walk(child, depth+1)
		}
	}
	for _, id := range roots {
		walk(id, 0)
	}
	for _, span := range trace.Spans {
		if !seen[span.ID] && len(seen) < 200 {
			lines = append(lines, "（未连到已知根节点，可能存在环路或过深关系）")
			walk(span.ID, 0)
		}
	}
	return output{data: map[string]any{"trace_id": trace.ID, "span_count": len(trace.Spans), "shown_spans": len(seen), "tree": strings.Join(lines, "\n")}, summary: fmt.Sprintf("采集到 %d 个 span，树中显示 %d 个；最多 200 个、32 层", len(trace.Spans), len(seen))}
}

// treeLine 把一个步骤写成一行，缺失父节点和 SQL 模板都归到这个步骤上。
func treeLine(span source.Span, depth int, byID map[string]source.Span) string {
	line := fmt.Sprintf("%s%s | %s | %.3fms | span=%s", strings.Repeat("  ", depth), span.Service, span.Operation, float64(span.Duration)/1000, span.ID)
	if span.ParentID != "" {
		if _, ok := byID[span.ParentID]; !ok {
			line += "（父节点未采集）"
		}
	}
	// SQL 模板单独附在所属步骤下面，人工取证才能从慢 span 追到执行计划。
	statement := span.Tags["db.query.text"]
	if statement == "" {
		statement = span.Tags["db.statement"]
	}
	if statement != "" {
		line += " | SQL模板=" + statement
	}
	return line
}

// recentChanges 只根据已采集到的启动时间和版本标签说明变化，不声称知道发布原因。
func recentChanges(ctx context.Context, metrics source.MetricsSource, seconds int) (output, error) {
	end := time.Now().UTC()
	start := end.Add(-time.Duration(seconds) * time.Second)
	// 最多 241 个点。一小时窗口用 15s，较短窗口也不比 10s 的实际抓取频率更密。
	step := time.Duration(max(10, (seconds+239)/240)) * time.Second
	query := source.MetricQuery{Expression: "process_start_time_seconds", Start: start, End: end, Step: step}
	starts, err := metrics.Query(ctx, query)
	if err != nil {
		return output{}, err
	}
	query.Expression = "app_build_info"
	versions, err := metrics.Query(ctx, query)
	if err != nil {
		return output{}, err
	}
	restarts := []restart{}
	builds := []version{}
	seen := make(map[string]bool)
	for _, series := range starts.Series {
		for _, point := range series.Points {
			seconds, err := strconv.ParseFloat(point.Value, 64)
			if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < float64(start.Unix()) || seconds > float64(end.Unix()) {
				continue
			}
			key := series.Labels["service"] + "/" + series.Labels["instance"] + "/" + point.Value
			if seen[key] {
				continue
			}
			seen[key] = true
			restarts = append(restarts, restart{series.Labels["service"], series.Labels["instance"], time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)})
		}
	}
	for _, series := range versions.Series {
		if len(series.Points) == 0 {
			continue
		}
		builds = append(builds, version{series.Labels["service"], series.Labels["version"], time.Unix(int64(series.Points[0].Time), 0).UTC().Format(time.RFC3339), time.Unix(int64(series.Points[len(series.Points)-1].Time), 0).UTC().Format(time.RFC3339)})
	}
	return output{data: map[string]any{"restarts": restarts, "versions": builds, "limited": starts.Limited || versions.Limited}, summary: fmt.Sprintf("最近 %d 秒采集到 %d 次启动、%d 组版本标签；首次采样不是精确发布时间，无数据不能证明无变更", seconds, len(restarts), len(builds))}, nil
}

// restart 是指标记录的启动，不代表我们知道是谁重启了服务。
type restart struct {
	Service   string `json:"service"`    // 指标标签中的服务。
	Instance  string `json:"instance"`   // 同服务的不同进程仍分别显示。
	StartedAt string `json:"started_at"` // 指标记录的启动时间。
}

// version 保留窗口内的采样范围，不把第一次采到当成精确发布时间。
type version struct {
	Service     string `json:"service"`      // 来源服务。
	Value       string `json:"version"`      // 应用公开的版本标签。
	FirstSample string `json:"first_sample"` // 当前窗口内第一次采到的时间。
	LastSample  string `json:"last_sample"`  // 当前窗口内最后一次采到的时间。
}
