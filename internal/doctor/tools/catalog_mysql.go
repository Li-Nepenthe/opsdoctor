// 本文件：登记六种用途固定的 MySQL 查看工具。
//
// 只有 mysql_explain 接受 SQL，但它先按只读规则检查，再由适配器加上 EXPLAIN。
// 其余工具只接受受限的表名或条数，实际 SQL 模板都在适配器里。
//
// 主要内容：addMySQL 绑定本系统的六种查看方法，并为每个结果附上用途说明。
// 阅读建议：不要把执行计划的行数当成真实业务行；摘要里也不能出现样例 SQL 原文。

package tools

import (
	"context"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// addMySQL 将六种固定查看操作登记在同一套入口，不暴露数据库客户端。
func (registry *Registry) addMySQL(inspector source.MySQLInspector) {
	registry.add("mysql_explain", "只接收单条 SELECT，工具自己加 EXPLAIN。分析 UPDATE 时请改写成条件相同的 SELECT；不支持 ANALYZE。", map[string]Property{"sql": textProperty("简单 SELECT，最多 2048 字节", 1, 2048)}, []string{"sql"}, func(ctx context.Context, args arguments) (output, error) {
		statement, err := source.SelectStatement(args.text("sql"))
		if err != nil {
			return output{}, fmt.Errorf("%w：%s", ErrInvalidArgument, err)
		}
		result, err := inspector.Explain(ctx, statement)
		return summarizeTable(result, "执行计划，不是查询结果行"), err
	})
	for _, item := range []struct {
		name        string                                              // 工具名称。
		description string                                              // 查看目的。
		run         func(context.Context, string) (source.Table, error) // 绑定本系统的查看方法。
	}{
		{"mysql_show_indexes", "查看表的索引和列顺序。", inspector.Indexes},
		{"mysql_table_stats", "查看表的估计行数与空间，不扫描业务数据。", inspector.TableStats},
	} {
		registry.add(item.name, item.description, map[string]Property{"table": tableProperty()}, []string{"table"}, func(ctx context.Context, args arguments) (output, error) {
			result, err := item.run(ctx, args.text("table"))
			return summarizeTable(result, item.description), err
		})
	}
	registry.add("mysql_top_queries", "查看按累计耗时排序的 DIGEST_TEXT 摘要，不读取带业务值的 QUERY_SAMPLE_TEXT。", map[string]Property{"limit": integerProperty("最多 SQL 摘要条数", 1, 20, 10)}, nil, func(ctx context.Context, args arguments) (output, error) {
		result, err := inspector.TopQueries(ctx, args.integer("limit"))
		return summarizeTable(result, "SQL 模板的累计统计，具体值已替换为问号"), err
	})
	registry.add("mysql_lock_waits", "查看当前库的锁等待关系，不读取锁中的业务数据值。", nil, nil, func(ctx context.Context, _ arguments) (output, error) {
		result, err := inspector.LockWaits(ctx)
		return summarizeTable(result, "当前锁等待，不代表过去没有发生过等待"), err
	})
	registry.add("mysql_status", "查看连接、线程和行锁等固定运行状态。", nil, nil, func(ctx context.Context, _ arguments) (output, error) {
		result, err := inspector.Status(ctx)
		return summarizeTable(result, "MySQL 当前运行状态"), err
	})
}
