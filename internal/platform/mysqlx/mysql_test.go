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
		func(value *Config) { value.MaxOpenConns = 0 },
		func(value *Config) { value.MaxIdleConns = 21 },
		func(value *Config) { value.ConnMaxLifetime = 0 },
		func(value *Config) { value.DSN = "private-password-value" },
	} {
		invalid := config
		change(&invalid)
		if _, err := prepare(invalid); err == nil || strings.Contains(err.Error(), "private-password-value") {
			t.Fatalf("错误不安全：%v", err)
		}
	}
}
