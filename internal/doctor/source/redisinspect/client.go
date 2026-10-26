// 本文件：通过独立 Redis ACL 账号读取运行信息。
//
// ACL 是 Redis 按账号限制命令的规则。这里的客户端也只暴露 INFO，或经过删减的
// SLOWLOG GET 摘要；两层都不提供 GET、HGETALL、KEYS 这类业务内容读取。
//
// 主要内容：
//   - New：设置固定连接预算，关闭不需要的握手附加命令。
//   - Info：按允许的分区读 INFO，或只返回慢命令的名称、耗时和参数数量。
//
// 阅读建议：先看分区白名单，再看慢日志为什么不返回 Args 原文。

package redisinspect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

// Client 的底层 Redis 客户端不导出，调用者无法借它执行任意命令。
type Client struct{ redis *redis.Client }

// New 只接受带独立 ACL 用户的 URL，凭据始终留在配置里，不出现在错误中。
func New(address string) (*Client, error) {
	options, err := redis.ParseURL(address)
	if err != nil || options.Username == "" || options.Username == "default" || options.Password == "" || options.DB != 0 {
		return nil, fmt.Errorf("缓存后端 Redis 只读配置必须使用独立账号，且 DB 为 0")
	}
	options.Protocol = 2
	options.DisableIdentity = true
	options.MaintNotificationsConfig = &maintnotifications.Config{Mode: maintnotifications.ModeDisabled}
	options.PoolSize = 3
	options.MaxActiveConns = 3
	options.MinIdleConns = 0
	options.MaxRetries = -1 // 一个诊断工具的失败不触发额外重试，避免在故障时继续放大压力。
	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 10 * time.Second
	options.WriteTimeout = 5 * time.Second
	options.PoolTimeout = 10 * time.Second
	options.ContextTimeoutEnabled = true
	return &Client{redis: redis.NewClient(options)}, nil
}

// Close 由系统装配统一调用，单次工具执行只归还连接。
func (client *Client) Close() error { return client.redis.Close() }

// Info 不把 section 拼成命令文本；未知分区在联网前被拒绝。
func (client *Client) Info(ctx context.Context, section string) (map[string]string, error) {
	switch section {
	case "server", "clients", "memory", "stats", "replication", "cpu", "keyspace", "commandstats", "slowlog":
	default:
		return nil, fmt.Errorf("不支持这个 Redis 信息分区")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if section == "slowlog" {
		return client.slowlog(bounded)
	}
	text, err := client.redis.Info(bounded, section).Result()
	if err != nil {
		return nil, readError(bounded)
	}
	return parseInfo(text)
}

// slowSummary 只保留排查耗时需要的字段，类型里没有业务参数的位置。
type slowSummary struct {
	ID         int64     `json:"id"`          // Redis 分配的慢日志编号。
	Time       time.Time `json:"time"`        // 命令执行时间。
	DurationUS int64     `json:"duration_us"` // 服务端执行耗时。
	Command    string    `json:"command"`     // 只有命令名，不含 key 或 value。
	Arguments  int       `json:"arguments"`   // 参数个数帮助区分操作规模。
}

// slowlog 在结果进入通用工具层之前就删掉参数，不能依赖最后的 4KB 截断来保密。
func (client *Client) slowlog(ctx context.Context) (map[string]string, error) {
	entries, err := client.redis.SlowLogGet(ctx, 10).Result()
	if err != nil {
		return nil, readError(ctx)
	}
	summaries := make([]slowSummary, 0, len(entries))
	for _, entry := range entries {
		// Args 第一项是命令名。其余项即使很短，也不能交给医生。
		command := ""
		if len(entry.Args) > 0 {
			command = entry.Args[0]
		}
		summaries = append(summaries, slowSummary{entry.ID, entry.Time, entry.Duration.Microseconds(), command, max(0, len(entry.Args)-1)})
	}
	data, err := json.Marshal(summaries)
	if err != nil {
		return nil, fmt.Errorf("无法生成 Redis 慢日志摘要")
	}
	return map[string]string{"slowlog_summary": string(data)}, nil
}

// parseInfo 把每行 key:value 转成字段，保留值中其余的冒号（例如复制节点地址）。
func parseInfo(text string) (map[string]string, error) {
	if len(text) > 2<<20 {
		return nil, fmt.Errorf("缓存后端 Redis INFO 超过读取预算")
	}
	result := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// INFO 用井号行区分章节，空行也不是数据。
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || key == "" {
			return nil, fmt.Errorf("缓存后端 Redis INFO 格式不合法")
		}
		result[key] = value
	}
	return result, nil
}

// readError 不回显 Redis 的命令错误原文，避免把被拒绝的参数或账号细节带给调用者。
func readError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("缓存后端 Redis 只读查询未完成，请检查账号权限或连接")
}
