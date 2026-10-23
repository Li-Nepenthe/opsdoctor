// 本文件：用真实 Redis 验证幂等占位与原子更新。
//
// 测试固定使用 Redis 的 15 号数据库。每次生成独立键，结束时只删除自己创建的键，不清空整个库。
//
// 主要内容：
//   - integrationStore：显式开启测试，并创建有时间上限的 Redis 客户端。
//   - TestIntegrationReservationCompetition：同时占位只能有一个处理者，完成后可以回放。
//   - TestIntegrationReservationOwnership：旧所有者不能改写或删除新记录。
//   - TestIntegrationExpiredProcessing：超出处理预算的记录转成待核对，不重新执行业务。
//
// 阅读建议：先看测试如何隔离数据，再看 Lua 脚本在真实 Redis 中返回的比较结果。

package order

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// integrationStore 只有设置 TEST_REDIS_ADDR 才连接，地址不打印，测试键也不会写到业务数据库 0。
func integrationStore(t *testing.T) (*RedisIdempotency, string) {
	t.Helper()
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("设置 TEST_REDIS_ADDR 后运行 Redis 幂等测试，固定使用数据库 15")
	}
	client := redis.NewClient(&redis.Options{Addr: address, DB: 15, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, ContextTimeoutEnabled: true})
	key := "test-" + rand.Text()
	t.Cleanup(func() {
		// t.Context 在 Cleanup 开始前会取消，清理另用一秒预算，不让连接故障卡住测试退出。
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := client.Del(ctx, "idempotency:"+key).Err(); err != nil {
			t.Error(err)
		}
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	store, err := NewRedisIdempotency(client, time.Hour, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return store, key
}

// reserveResult 把并发占位的三个返回值一起送回主测试，不在 goroutine 中调用 Fatal。
type reserveResult struct {
	reservation Reservation // 成功拿到的记录。
	owned       bool        // 是否拿到执行权。
	err         error       // 连接或参数错误。
}

// TestIntegrationReservationCompetition 验证 SET NX 的竞争结果，再检查写回结果是否保留原 TTL。
func TestIntegrationReservationCompetition(t *testing.T) {
	store, key := integrationStore(t)
	request := CreateRequest{UserID: 1, SKUID: 2, Quantity: 3}
	results := make(chan reserveResult, 20)
	start := make(chan struct{})
	for range 20 {
		go func() {
			<-start
			value, owned, err := store.Reserve(t.Context(), key, request)
			results <- reserveResult{value, owned, err}
		}()
	}
	close(start)
	owners := 0
	var owner Reservation
	for range 20 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.owned {
			owners++
			owner = result.reservation
		}
	}
	if owners != 1 {
		t.Fatalf("同时占位出现 %d 个执行者", owners)
	}
	// 完成后保存同一订单结果。不能把最初的一小时保存期重新延长。
	before := store.client.PTTL(t.Context(), "idempotency:"+key).Val()
	want := Created{OrderID: 123, AmountCents: 300, Status: "created"}
	if err := store.Complete(t.Context(), owner, want); err != nil {
		t.Fatal(err)
	}
	value, owned, err := store.Reserve(t.Context(), key, request)
	if err != nil || owned || value.State != stateCompleted || value.Result == nil || *value.Result != want {
		t.Fatalf("无法回放：%+v %t %v", value, owned, err)
	}
	after := store.client.PTTL(t.Context(), "idempotency:"+key).Val()
	if after <= 0 || after > before {
		t.Fatalf("写回后 TTL 被延长或丢失：%v -> %v", before, after)
	}
	request.Quantity++
	if _, _, err := store.Reserve(t.Context(), key, request); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("同一键改变参数未被拒绝")
	}
	t.Log("真实 Redis 并发占位 20 次：只有 1 个执行者，完成结果可回放，TTL 保持原窗口")
}

// TestIntegrationReservationOwnership 删除安全失败的占位后，新请求得到新的所有者，旧请求不能再修改它。
func TestIntegrationReservationOwnership(t *testing.T) {
	store, key := integrationStore(t)
	request := CreateRequest{UserID: 1, SKUID: 2, Quantity: 1}
	old, _, err := store.Reserve(t.Context(), key, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Abandon(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	current, owned, err := store.Reserve(t.Context(), key, request)
	if err != nil || !owned || current.Owner == old.Owner {
		t.Fatal("没有生成新的占位所有者")
	}
	// 即使旧请求很晚才回来，也不能覆盖或删除新一轮占位。
	if err := store.Complete(t.Context(), old, Created{OrderID: 99}); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("旧请求覆盖新结果：%v", err)
	}
	if err := store.Abandon(t.Context(), old); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("旧请求删除新占位：%v", err)
	}
	if err := store.Review(t.Context(), current, 88); err != nil {
		t.Fatal(err)
	}
	value, owned, err := store.Reserve(t.Context(), key, request)
	if err != nil || owned || value.State != stateNeedsReview || value.OrderID != 88 {
		t.Fatalf("待核对记录丢失：%+v %v", value, err)
	}
}

// TestIntegrationExpiredProcessing 模拟进程退出留下的占位，不用真实等待十秒。
func TestIntegrationExpiredProcessing(t *testing.T) {
	store, key := integrationStore(t)
	request := CreateRequest{UserID: 1, SKUID: 2, Quantity: 1}
	value, _, err := store.Reserve(t.Context(), key, request)
	if err != nil {
		t.Fatal(err)
	}
	value.ReviewAfter = time.Now().Add(-time.Second).UnixMilli()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// 只在测试专用键上改变处理截止时间，生产代码没有“模拟故障”的开关。
	if err := store.client.Set(t.Context(), "idempotency:"+key, data, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	first, owned, err := store.Reserve(t.Context(), key, request)
	if err != nil || owned || first.State != stateNeedsReview || !first.EnteredReview {
		t.Fatalf("旧占位未转待核对：%+v %v", first, err)
	}
	second, owned, err := store.Reserve(t.Context(), key, request)
	if err != nil || owned || second.EnteredReview || second.State != stateNeedsReview {
		t.Fatalf("重复读取又被当成状态转换：%+v %v", second, err)
	}
}
