// 本文件：用 Redis 记录哪些下单请求已经取得处理权，重复请求应该看到什么结果。
// Reserve 只有在键不存在时才能占位。后续的保存和删除都要核对完整的旧记录，避免过期请求改写新订单。

package order

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// replaceReservationLua 在 Redis 内原子比较完整旧值，避免读与写之间被另一个请求插入。
// KEEPTTL 保留原占位的有效期，重复请求不能不断把二十四小时往后延长。
const replaceReservationLua = `if redis.call('GET', KEYS[1]) == ARGV[1] then
	redis.call('SET', KEYS[1], ARGV[2], 'XX', 'KEEPTTL');
	return 1
end;
return 0`

// deleteReservationLua 只删除自己看到的记录，旧请求不能删除过期后新建的同名占位。
const deleteReservationLua = `if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('DEL', KEYS[1])
end;
return 0`

// RedisIdempotency 保存固定策略。真实状态只保存在 Redis，不依靠单个进程的内存锁。
type RedisIdempotency struct {
	client           *redis.Client // app 负责创建与关闭。
	ttl              time.Duration // 幂等保证的保存窗口，默认二十四小时。
	processingBudget time.Duration // 超过业务和收尾总上限后，把旧处理中状态转为待核对。
}

// NewRedisIdempotency 要求记录寿命长于处理预算，避免请求尚未结束 key 就消失。
func NewRedisIdempotency(client *redis.Client, ttl, processingBudget time.Duration) (*RedisIdempotency, error) {
	if client == nil || processingBudget <= 0 || ttl <= processingBudget || ttl%time.Second != 0 {
		return nil, fmt.Errorf("幂等保存时长须为整秒且大于处理预算")
	}
	return &RedisIdempotency{client: client, ttl: ttl, processingBudget: processingBudget}, nil
}

// requestFingerprint 只比较三个已解析的整数，JSON 空格或字段顺序不会改变同一请求的身份。
func requestFingerprint(request CreateRequest) string {
	data, _ := json.Marshal(request) // 结构只有整数，标准 JSON 编码不会失败。
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Reserve 只有 SET NX 成功才授予执行权。Redis 故障时直接拒绝，不退回无保护下单。
func (store *RedisIdempotency) Reserve(ctx context.Context, key string, request CreateRequest) (Reservation, bool, error) {
	reservation := Reservation{
		Key: key, Owner: rand.Text(), Fingerprint: requestFingerprint(request),
		State: stateProcessing, SKUID: request.SKUID,
		ReviewAfter: time.Now().Add(store.processingBudget).UnixMilli(),
	}
	data, err := json.Marshal(reservation)
	if err != nil {
		return Reservation{}, false, ErrIdempotencyUnavailable
	}
	reservation.raw = string(data)
	owned, err := store.client.SetNX(ctx, "idempotency:"+key, data, store.ttl).Result()
	if err != nil {
		return Reservation{}, false, ErrIdempotencyUnavailable
	}
	if owned {
		return reservation, true, nil
	}
	// SET NX 失败只说明有人先占位，不能因此重复执行，必须读取它留下的状态。
	existing, err := store.existing(ctx, key, reservation.Fingerprint)
	return existing, false, err
}

// existing 在总处理预算已过时把遗留占位转为待核对，覆盖进程退出或 Redis 写状态失败的情况。
func (store *RedisIdempotency) existing(ctx context.Context, key, fingerprint string) (Reservation, error) {
	// 比较失败可能是原请求刚刚完成，重新读才能看到它的结果。一次占位请求最多读三次。
	for range 3 {
		raw, err := store.client.Get(ctx, "idempotency:"+key).Result()
		if err != nil {
			return Reservation{}, ErrIdempotencyUnavailable
		}
		var reservation Reservation
		if err := json.Unmarshal([]byte(raw), &reservation); err != nil {
			return Reservation{}, ErrIdempotencyUnavailable
		}
		reservation.Key, reservation.raw = key, raw
		if reservation.Fingerprint != fingerprint {
			return Reservation{}, ErrInvalidArgument
		}
		switch reservation.State {
		case stateCompleted, stateNeedsReview:
			return reservation, nil
		case stateProcessing:
			updated, err := store.reviewIfExpired(ctx, reservation)
			if errors.Is(err, ErrOutcomeUnknown) {
				continue
			}
			return updated, err
		default:
			return Reservation{}, ErrIdempotencyUnavailable
		}
	}
	return Reservation{}, ErrIdempotencyUnavailable
}

func (store *RedisIdempotency) reviewIfExpired(ctx context.Context, reservation Reservation) (Reservation, error) {
	if time.Now().UnixMilli() < reservation.ReviewAfter {
		return reservation, nil
	}
	updated := reservation
	updated.State = stateNeedsReview
	if err := store.replace(ctx, reservation, updated); err != nil {
		return Reservation{}, err
	}
	updated.EnteredReview = true
	return updated, nil
}

// Complete 只把自己的处理中记录替换成完成结果，已经转成待核对的记录不能偷偷改回成功。
func (store *RedisIdempotency) Complete(ctx context.Context, reservation Reservation, result Created) error {
	updated := reservation
	updated.State, updated.OrderID, updated.Result = stateCompleted, result.OrderID, &result
	return store.replace(ctx, reservation, updated)
}

// Review 保留原有占位期限，避免不确定请求在窗口内重新执行业务。
func (store *RedisIdempotency) Review(ctx context.Context, reservation Reservation, orderID int64) error {
	updated := reservation
	updated.State, updated.OrderID = stateNeedsReview, orderID
	return store.replace(ctx, reservation, updated)
}

// replace 不把订单编号交给 Lua 的浮点数运算，完整 JSON 由 Go 编码后原样替换。
func (store *RedisIdempotency) replace(ctx context.Context, previous, next Reservation) error {
	data, err := json.Marshal(next)
	if err != nil {
		return ErrIdempotencyUnavailable
	}
	changed, err := store.client.Eval(ctx, replaceReservationLua, []string{"idempotency:" + previous.Key}, previous.raw, string(data)).Int()
	if err != nil {
		return ErrIdempotencyUnavailable
	}
	if changed != 1 {
		return ErrOutcomeUnknown
	}
	return nil
}

// Abandon 只用于确定没有留下业务变更的路径。删除失败时不能声称客户端可以安全重试。
func (store *RedisIdempotency) Abandon(ctx context.Context, reservation Reservation) error {
	deleted, err := store.client.Eval(ctx, deleteReservationLua, []string{"idempotency:" + reservation.Key}, reservation.raw).Int()
	if err != nil {
		return ErrIdempotencyUnavailable
	}
	if deleted != 1 {
		return ErrOutcomeUnknown
	}
	return nil
}
