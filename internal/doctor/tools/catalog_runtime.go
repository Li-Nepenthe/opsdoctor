// 本文件：登记 Redis 运行信息和 goroutine 堆栈摘要。
//
// 工具只捕获传入的 source 接口，调用时不能更换后端地址或账号。
//
// 主要内容：addRedis 登记 INFO/慢命令摘要；addProfiles 登记按服务汇总的堆栈。
// 阅读建议：返回的是运行状态；Redis key 内容与完整 pprof 入口没有暴露。

package tools

import (
	"context"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// addRedis 只登记查看分区，调用者不能借运行状态查询执行 Redis 命令。
func (registry *Registry) addRedis(backend source.RedisInspector) {
	// 分区白名单同时用于 Schema 和实际校验，不开放任意 Redis 命令。
	section := textProperty("INFO 分区；slowlog 返回删去 key/value 的慢命令摘要", 1, 16)
	section.Enum = []string{"server", "clients", "memory", "stats", "replication", "cpu", "keyspace", "commandstats", "slowlog"}
	section.Default = "stats"
	registry.add("redis_info", "查看 Redis 运行负担，不读取 key 内容。", map[string]Property{"section": section}, nil, func(ctx context.Context, args arguments) (output, error) {
		values, err := backend.Info(ctx, args.text("section"))
		return output{data: values, summary: fmt.Sprintf("Redis %s 分区，%d 个字段", args.text("section"), len(values))}, err
	})
}

// addProfiles 只允许选择配置里已有的服务，并限制输出的堆栈组数。
func (registry *Registry) addProfiles(backend source.ProfileSource) {
	// 服务名只用于查已配置的映射，不能被当作网络地址。
	registry.add("goroutine_summary", "按相同函数栈汇总任务，只访问已经配置的服务管理端口。", map[string]Property{"service": nameProperty("服务名"), "limit": integerProperty("最多显示的堆栈组数", 1, 20, 5)}, []string{"service"}, func(ctx context.Context, args arguments) (output, error) {
		groups, err := backend.Goroutines(ctx, args.text("service"))
		total := 0
		for _, group := range groups {
			total += group.Count
		}
		return output{data: groups[:min(len(groups), args.integer("limit"))], summary: fmt.Sprintf("共 %d 个任务、%d 组堆栈，显示数量最多的前 %d 组", total, len(groups), min(len(groups), args.integer("limit")))}, err
	})
}
