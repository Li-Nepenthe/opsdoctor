// 本文件：离线区分暂时故障和 SQL 配置错误。
//
// TestDatabaseErrorClassification 构造驱动错误，不连接数据库。
// 先看断连、锁等待与死锁，再对比权限和语法错误；全部映射成 UNAVAILABLE 会诱导调用方错误重试。

package inventory

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestDatabaseErrorClassification 区分暂时不可用和永久配置错误，防止所有 SQL 错误都变成可重试。
func TestDatabaseErrorClassification(t *testing.T) {
	for _, testcase := range []struct {
		name        string // name 描述数据库失败的实际含义。
		err         error  // err 是驱动或网络可能返回的原因。
		unavailable bool   // unavailable 决定 gRPC 是否应映射 UNAVAILABLE。
	}{
		{"网络断开", io.EOF, true},                            // 响应未读完连接已经关闭。
		{"连接失效", mysql.ErrInvalidConn, true},              // 池中连接不能再使用。
		{"连接容量耗尽", &mysql.MySQLError{Number: 1040}, true}, // 本机 MySQL 达到连接上限。
		{"锁等待超时", &mysql.MySQLError{Number: 1205}, true},  // 争用消失后查询可能恢复。
		{"死锁回滚", &mysql.MySQLError{Number: 1213}, true},   // 数据库已回滚本次事务。
		{"权限错误", &mysql.MySQLError{Number: 1142}, false},  // 需要修改部署权限，等待不会自行恢复。
		{"语法错误", &mysql.MySQLError{Number: 1064}, false},  // 是代码错误，不能假装成短暂故障。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			err := classifyDatabaseError(t.Context(), testcase.err)
			if errors.Is(err, ErrUnavailable) != testcase.unavailable || !errors.Is(err, testcase.err) {
				t.Fatalf("错误分类：%v", err)
			}
		})
	}
	// 连接因请求取消而关闭时，优先反馈取消，不能误报网络故障。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := classifyDatabaseError(ctx, io.EOF); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消优先级：%v", err)
	}
}
