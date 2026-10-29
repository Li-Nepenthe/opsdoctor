// 本文件：为审批准备和恢复提供有限的只读元数据检查。
//
// 建索引账号只有 INDEX 权限，不能借它查询业务内容。列名、索引形状和旧连接是否
// 还在运行，都使用原来的 diag_ro 连接池读取，不再额外开一套只读连接。
//
// 主要内容：Columns 验证列名；IndexShape 核对已完成索引；ConnectionAlive 检查旧 DDL 连接。
// 阅读建议：这些方法不放进模型工具菜单，只服务于程序的审批边界。

package mysqlinspect

import (
	"context"
	"fmt"
)

// Columns 使用绑定参数查固定库的列名，不能通过表名跨库。
func (client *Client) Columns(ctx context.Context, table string) ([]string, error) {
	if !tableName(table) {
		return nil, fmt.Errorf("表名不合法")
	}
	data, err := client.query(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", 256, client.schema, table)
	if err != nil {
		return nil, err
	}
	columns := []string{}
	for _, row := range data.Rows {
		columns = append(columns, row[0])
	}
	return columns, nil
}

// IndexShape 比较完整列顺序和唯一性，不把碰巧同名的另一种索引当作执行成功。
func (client *Client) IndexShape(ctx context.Context, table, index string) ([]string, bool, bool, error) {
	if !tableName(table) || !tableName(index) {
		return nil, false, false, fmt.Errorf("表或索引名不合法")
	}
	data, err := client.query(ctx, "SELECT COLUMN_NAME,NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND INDEX_NAME=? ORDER BY SEQ_IN_INDEX", 64, client.schema, table, index)
	if err != nil {
		return nil, false, false, err
	}
	if len(data.Rows) == 0 {
		return nil, false, false, nil
	}
	columns := []string{}
	for _, row := range data.Rows {
		columns = append(columns, row[0])
	}
	return columns, data.Rows[0][1] == "0", true, nil
}

// ConnectionAlive 只返回线程是否存在，不读取当前 SQL 文本或锁里的业务值。
func (client *Client) ConnectionAlive(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("连接编号不合法")
	}
	data, err := client.query(ctx, "SELECT PROCESSLIST_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=? LIMIT 1", 1, id)
	return len(data.Rows) > 0, err
}
