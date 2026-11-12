// 本文件：医生服务在监听前由 loadConfig 一次检查所有必填项。
// 报错可包含变量名，初始登录密码只保留摘要。

package app

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/go-sql-driver/mysql"
)

// settings 不能用于结构化日志；database 和 jwtSecret 都包含敏感配置。
type settings struct {
	address      string
	database     string // DOCTOR_MYSQL_DSN，只允许 doctor 库的独立账号。
	targets      string
	jwtSecret    string
	email        string // 首次启动账号，已有邮箱不会被覆盖。
	passwordHash string // 已转换为 bcrypt 摘要；不保留明文初始密码。
}

// loadConfig 一次收集所有缺项；密码错误只解释长度要求，不输出输入。
func loadConfig() (settings, error) {
	reader := config.New()
	var problems []error
	read := func(name, fallback string) string {
		value, err := reader.String(name, fallback)
		problems = append(problems, err)
		return value
	}
	value := settings{address: read("DOCTOR_ADDR", ":8090"), database: read("DOCTOR_MYSQL_DSN", ""), targets: read("TARGETS_DIR", "deploy/doctor/targets"), jwtSecret: read("JWT_SECRET", "")}
	email, err := auth.NormalizeEmail(read("DOCTOR_ADMIN_EMAIL", ""))
	problems = append(problems, err)
	value.email = email
	hash, err := auth.HashPassword(read("DOCTOR_ADMIN_PASSWORD", ""))
	problems = append(problems, err)
	value.passwordHash = hash
	_, err = auth.NewTokens(value.jwtSecret, time.Hour)
	problems = append(problems, err)
	if _, _, err := net.SplitHostPort(value.address); err != nil {
		problems = append(problems, fmt.Errorf("DOCTOR_ADDR 必须是主机:端口"))
	}
	// 防止操作者把靶场 DSN 误放进医生自己的配置；两套数据不能共用写权限。
	parsed, err := mysql.ParseDSN(value.database)
	if err != nil || parsed.DBName != "doctor" || parsed.User != "doctor" || parsed.Passwd == "" {
		problems = append(problems, fmt.Errorf("DOCTOR_MYSQL_DSN 必须使用 doctor 库的 doctor 账号"))
	}
	return value, errors.Join(problems...)
}
