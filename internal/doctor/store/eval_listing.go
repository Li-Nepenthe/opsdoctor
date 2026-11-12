// 本文件：页面只列最近五十个批次。先关闭列表游标再读每批详情，避免只有一条空闲连接时互相等待。

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
