package mysqlx

import (
	"strings"
	"testing"
	"time"
)

// TestPrepare 验证 Go 日期解码和数据库会话都使用 UTC，并禁止多语句查询。
func TestPrepare(t *testing.T) {
	config := Config{DSN: "app:demo@tcp(localhost:3306)/shop?multiStatements=true&loc=Local", MaxOpenConns: 20, MaxIdleConns: 10, ConnMaxLifetime: 5 * time.Minute}
	parsed, err := prepare(config)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.ParseTime || parsed.Loc != time.UTC || parsed.Params["time_zone"] != "'+00:00'" || parsed.MultiStatements {
		t.Fatal("UTC 或多语句约束没有生效")
	}
	// 错误应指向连接池配置；对 DSN 错误则不得暴露测试密码。
	for _, change := range []func(*Config){
		// 连接上限必须为正数，不能让 0 变成没有上限的连接池。
		func(value *Config) { value.MaxOpenConns = 0 },
		// 空闲连接不能多于最大连接数；21 刚好超过本例的上限 20。
		func(value *Config) { value.MaxIdleConns = 21 },
		// 连接寿命必须为正，避免错误配置使旧连接永远不按寿命回收。
		func(value *Config) { value.ConnMaxLifetime = 0 },
		// 无法解析的 DSN 应被拒绝，错误消息不能带出这个模拟密码。
		func(value *Config) { value.DSN = "private-password-value" },
	} {
		// Config 按值复制，每次只改一个条件，避免前一条用例的错误污染下一条。
		invalid := config
		change(&invalid)
		if _, err := prepare(invalid); err == nil || strings.Contains(err.Error(), "private-password-value") {
			t.Fatalf("错误不安全：%v", err)
		}
	}
}
