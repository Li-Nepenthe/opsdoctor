// 本文件：为页面提供有数量上限的评测批次清单。
//
// 主要内容：EvalRuns 读取最近五十个批次，随后复用 EvalRun 的字段约定。
// 阅读建议：先关闭列表游标，再读详细信息，避免连接池只剩一条连接时互相等待。

package store

import "context"

// EvalRuns 不把账户 Key 或数据库连接配置带进页面，批次配置本来就只有公开版本信息。
func (store *Store) EvalRuns(ctx context.Context) ([]EvalRun, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT id FROM eval_runs ORDER BY created_at DESC,id DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	problem := rows.Err()
	closeErr := rows.Close()
	if problem != nil {
		return nil, problem
	}
	if closeErr != nil {
		return nil, closeErr
	}
	// 不在持有游标时申请第二个连接，和命令行导出采用相同的边界。
	result := []EvalRun{}
	for _, id := range ids {
		value, err := store.EvalRun(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
