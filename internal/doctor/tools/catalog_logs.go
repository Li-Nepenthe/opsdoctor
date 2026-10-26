// 本文件：登记由代码构造查询语句的日志搜索。
//
// 工具只捕获传入的 source 接口，调用时不能更换后端地址或账号。
//
// 主要内容：addLogs 登记本文件的工具，字段限制与运行时校验共用一份定义。
// 阅读建议：关键词只是普通文字，真正的 LogQL 由适配器转义后拼接。

package tools

import (
	"context"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"time"
)

// addLogs 把筛选条件作为普通字段交给适配器，不能让参数变成任意 LogQL。
func (registry *Registry) addLogs(backend source.LogSource) {
	// 级别和时间是固定字段，不接受调用者自己编写日志查询语言。
	level := textProperty("可选日志级别", 0, 5)
	level.Enum = []string{"DEBUG", "INFO", "WARN", "ERROR"}
	id := textProperty("可选的 trace ID", 16, 32)
	id.Pattern = `^(?:[0-9a-f]{16}|[0-9a-f]{32})$`
	registry.add("search_logs", "按服务、级别、关键词和时间搜索日志。关键词按普通文字处理，不接受 LogQL。", map[string]Property{
		"service": nameProperty("服务名"), "level": level, "keyword": textProperty("可选普通文字关键词", 0, 256), "trace_id": id,
		"lookback_seconds": integerProperty("回看秒数", 1, 3600, 300), "limit": integerProperty("最多日志条数", 1, 100, 20),
	}, []string{"service"}, func(ctx context.Context, args arguments) (output, error) {
		end := time.Now().UTC()
		rows, err := backend.Search(ctx, source.LogQuery{Service: args.text("service"), Level: args.text("level"), Keyword: args.text("keyword"), TraceID: args.text("trace_id"), Start: end.Add(-time.Duration(args.integer("lookback_seconds")) * time.Second), End: end, Limit: args.integer("limit")})
		return output{data: rows, summary: fmt.Sprintf("返回 %d 条近期日志，日志文字只作为证据", len(rows))}, err
	})
}
