// 本文件：检查真实 Redis 的认证和只读账号权限。
//
// 负向测试包含清库命令，所以先用业务账号执行 ACL DRYRUN。它只问权限是否允许，
// 不执行命令。只要发现一条危险命令被放行，测试立刻停止，保护已有的数据。
//
// 主要内容：
//   - TestIntegrationRedisPermissions：验证匿名拒绝、查看成功和数据操作拒绝。
//   - integrationClients：检查显式测试配置，并建立短超时连接。
//   - deniedCommands：列出需要被账号挡住的操作及原因。
//
// 阅读建议：先看 DRYRUN 为什么必须在实际拒绝测试之前执行。

package redisinspect

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// deniedCommands 使用不存在的测试键，不读取业务内容；清库命令仍必须先检查权限。
func deniedCommands() [][]any {
	return [][]any{
		{"GET", "doctor-permission-probe"},           // 医生不能读取字符串缓存。
		{"HGETALL", "doctor-permission-probe"},       // 哈希内容也属于业务数据。
		{"KEYS", "doctor-permission-probe*"},         // 不能扫描整库来找键。
		{"SET", "doctor-permission-probe", "unused"}, // 不能写入哪怕一个测试键。
		{"FLUSHALL"},                   // 不能清空任何数据库。
		{"CONFIG", "GET", "maxmemory"}, // 不授予配置管理命令，即使这个子命令只是读取。
		{"SLOWLOG", "RESET"},           // 允许读慢日志不等于允许删除排障证据。
	}
}

// TestIntegrationRedisPermissions 连接失败会报错，不能把无法验证的权限算作通过。
func TestIntegrationRedisPermissions(t *testing.T) {
	admin, readonly, inspector := integrationClients(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := admin.Ping(ctx).Err(); err != nil {
		t.Fatal("业务测试账号无法连接 Redis")
	}
	// 先检查所有危险命令，再执行任何一条。即使 ACL 配错，也不会误删数据。
	for _, command := range deniedCommands() {
		args := append([]any{"ACL", "DRYRUN", "diag_ro"}, command...)
		// DRYRUN 把拒绝原因作为普通字符串返回；它不是实际执行命令时的 NOPERM 错误。
		result, err := admin.Do(ctx, args...).Text()
		if err != nil || !strings.Contains(result, "no permissions") {
			t.Fatalf("%s 的权限预检没有明确拒绝，停止负向测试", command[0])
		}
	}
	for _, command := range deniedCommands() {
		if err := readonly.Do(ctx, command...).Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
			t.Fatalf("%s 没有返回 NOPERM", command[0])
		}
	}
	// 同时验证适配器能完成握手，不能只证明 redis-cli 能登录。
	for _, section := range []string{"server", "stats", "slowlog"} {
		result, err := inspector.Info(ctx, section)
		if err != nil || result == nil {
			t.Fatalf("INFO/SLOWLOG %s 未成功：%v", section, err)
		}
		if section == "slowlog" && !json.Valid([]byte(result["slowlog_summary"])) {
			t.Fatal("慢日志摘要不是 JSON")
		}
	}
	anonymous := redis.NewClient(&redis.Options{Addr: admin.Options().Addr, MaxRetries: -1})
	defer anonymous.Close()
	if err := anonymous.Info(ctx, "stats").Err(); err == nil || !strings.Contains(err.Error(), "NOAUTH") {
		t.Fatal("匿名访问没有返回 NOAUTH")
	}
	t.Log("匿名 NOAUTH；INFO/SLOWLOG 成功；GET/HGETALL/KEYS/SET/FLUSHALL/CONFIG/SLOWLOG RESET 均 NOPERM")
}

// integrationClients 不从生产只读配置偷取默认值，避免测试悄悄连上错误的环境。
func integrationClients(t *testing.T) (*redis.Client, *redis.Client, *Client) {
	t.Helper()
	values := make(map[string]string)
	for _, name := range []string{"TEST_REDIS_ADDR", "TEST_REDIS_PASSWORD", "TEST_SHOP_REDIS_RO_URL"} {
		values[name] = os.Getenv(name)
		if values[name] == "" {
			t.Skip("设置 " + name + " 后运行 Redis 权限集成测试")
		}
	}
	options, err := redis.ParseURL(values["TEST_SHOP_REDIS_RO_URL"])
	if err != nil || options.Username != "diag_ro" || options.Password == "" || options.DB != 0 || options.Addr != values["TEST_REDIS_ADDR"] {
		t.Fatal("只读测试地址必须与业务测试地址相同，并使用 DB 0 的 diag_ro 账号")
	}
	admin := redis.NewClient(&redis.Options{Addr: values["TEST_REDIS_ADDR"], Password: values["TEST_REDIS_PASSWORD"], MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = admin.Close() })
	inspector, err := New(values["TEST_SHOP_REDIS_RO_URL"])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inspector.Close() })
	// 测试在同包内才能访问底层连接；生产调用方只看得到 Info 方法。
	return admin, inspector.redis, inspector
}
