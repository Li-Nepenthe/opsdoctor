// 本文件：逐个模拟下单步骤失败，核对库存补偿与幂等记录。
//
// 测试使用内存替身，不连接外部服务。断言不只看错误码，还检查有没有重复扣减或错误删除占位。
//
// 主要内容：
//   - fakeRepository、fakePrice、fakeStock、fakePublisher：记录业务步骤执行次数。
//   - memoryIdempotency：模拟有所有者的幂等记录。
//   - TestCreateFailureBoundaries：验证每个失败位置的补偿和待核对规则。
//   - TestConcurrentIdempotency：重复请求并发进入，只有一个实际创建订单。
//   - TestCompensationSurvivesCancellation：上游取消后仍给补偿有限预算。
//   - TestPublishSurvivesCancellation：订单提交后客户断开，仍发送消息并保留上下文里的值。
//   - TestValidateCreate：无效参数在扣库存前被拒绝。
//
// 阅读建议：先读失败表格旁的解释，再对照 Create 中对应的分支。

package order

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// fakeRepository 记录创建次数，错误也会返回一个编号，模拟 INSERT 已获得主键的情况。
type fakeRepository struct {
	calls    atomic.Int32 // 并发测试用它确认只创建一次。
	err      error        // 构造后不修改，模拟事务不同失败位置。
	onCreate func()       // 取消测试在这里触发上游取消。
}

// Create 保留 service 算出的金额，测试才能检查有没有整数计算错误。
func (repository *fakeRepository) Create(_ context.Context, value Order) (Order, error) {
	repository.calls.Add(1)
	value.OrderID = 100
	if repository.onCreate != nil {
		repository.onCreate()
	}
	return value, repository.err
}

// Get 在这些流程测试里不使用，固定返回不存在。
func (*fakeRepository) Get(context.Context, int64) (Order, error) { return Order{}, ErrNotFound }

// List 保留空数组的契约，流程测试不需要另建查询数据。
func (*fakeRepository) List(_ context.Context, _ int64, page, size int) (Page, error) {
	return Page{Items: []Order{}, Page: page, PageSize: size}, nil
}

// fakePrice 固定返回测试价格或读取错误。
type fakePrice struct {
	price int64 // 可以设为 MaxInt64，验证金额溢出保护。
	err   error // 非空时模拟商品不存在或数据库故障。
}

// Get 不读取库存，证明下单不会依赖商品详情中的降级结果。
func (price *fakePrice) Get(context.Context, int64) (product.Product, error) {
	return product.Product{ID: 1, PriceCents: price.price}, price.err
}

// fakeStock 把扣减和补偿分别计数，防止“返回了错误但仍多扣一次”被漏掉。
type fakeStock struct {
	deductCalls  atomic.Int32          // 真正进入扣减的次数。
	releaseCalls atomic.Int32          // 真正进入补偿的次数。
	deductErr    error                 // 模拟明确拒绝或结果不明。
	releaseErr   error                 // 模拟补偿失败。
	onRelease    func(context.Context) // 用于检查补偿的独立时间预算。
}

// Deduct 不自行重试，测试关注的是 service 有没有重复调用。
func (stock *fakeStock) Deduct(context.Context, int64, int32) error {
	stock.deductCalls.Add(1)
	return stock.deductErr
}

// Release 在回调里检查 context 后，再返回预设结果。
func (stock *fakeStock) Release(ctx context.Context, _ int64, _ int32) error {
	stock.releaseCalls.Add(1)
	if stock.onRelease != nil {
		stock.onRelease(ctx)
	}
	return stock.releaseErr
}

// fakePublisher 记录发送次数，发送失败不应让订单再创建一次。
type fakePublisher struct {
	calls     atomic.Int32          // 用于断言重复请求不会再次发布。
	err       error                 // 模拟 Kafka 暂时不可用。
	onPublish func(context.Context) // 在发送时检查请求取消是否错误地传到了收尾操作。
}

// Publish 不处理消息内容，因为字段编码由 events 包的测试负责。
func (publisher *fakePublisher) Publish(ctx context.Context, _ events.OrderCreated) error {
	publisher.calls.Add(1)
	if publisher.onPublish != nil {
		publisher.onPublish(ctx)
	}
	return publisher.err
}

// TestPublishSurvivesCancellation 模拟数据库刚提交，客户端就断开连接的时刻。
func TestPublishSurvivesCancellation(t *testing.T) {
	value := newFixture()
	type requestKey struct{} // 自己的键类型不会和其他包保存的上下文值混在一起。
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "trace-value"))
	defer cancel()
	value.repository.onCreate = cancel
	value.publisher.onPublish = func(publishCtx context.Context) {
		// 必须真的发生过取消，否则这个测试无法重现旧代码的失败条件。
		if ctx.Err() == nil || publishCtx.Err() != nil {
			t.Fatalf("请求应已取消，消息应仍可发送：request=%v publish=%v", ctx.Err(), publishCtx.Err())
		}
		deadline, ok := publishCtx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > value.service.cleanupTimeout {
			t.Fatal("消息发送必须有独立且有限的预算")
		}
		if publishCtx.Value(requestKey{}) != "trace-value" {
			t.Fatal("消息上下文丢失了请求保存的值")
		}
	}
	_, err := value.service.Create(ctx, CreateRequest{UserID: 1, SKUID: 1, Quantity: 1}, "after-commit")
	if err != nil || value.publisher.calls.Load() != 1 || value.store.value.State != stateCompleted {
		t.Fatalf("提交后的消息与幂等收尾未完成：err=%v publish=%d state=%s", err, value.publisher.calls.Load(), value.store.value.State)
	}
}

// fakeReviews 统计进入待核对的次数，重复读取同一状态不应重复计数。
type fakeReviews struct{ calls atomic.Int32 } // 调用由并发测试安全计数。

// RecordReview 只记录次数，真实日志字段与指标在 app 层测试。
func (reviews *fakeReviews) RecordReview(context.Context, int64, int64, string, string) {
	reviews.calls.Add(1)
}

// memoryIdempotency 用锁模拟 Redis 的原子占位，状态与服务的公开接口保持一致。
type memoryIdempotency struct {
	mu          sync.Mutex  // 保护 present 与 value。
	present     bool        // false 模拟 key 不存在。
	value       Reservation // 测试只处理一个幂等键。
	reserveErr  error       // 占位失败时业务不能继续。
	completeErr error       // 模拟保存成功结果失败。
}

// Reserve 让首次请求拿到处理权，其他请求只返回已有状态。
func (store *memoryIdempotency) Reserve(_ context.Context, key string, request CreateRequest) (Reservation, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.reserveErr != nil {
		return Reservation{}, false, store.reserveErr
	}
	if store.present {
		if store.value.Fingerprint != requestFingerprint(request) {
			return Reservation{}, false, ErrInvalidArgument
		}
		return store.value, false, nil
	}
	store.present = true
	store.value = Reservation{Key: key, SKUID: request.SKUID, Fingerprint: requestFingerprint(request), State: stateProcessing, Owner: "owner"}
	return store.value, true, nil
}

// Complete 保存结果，后面的相同请求直接回放。
func (store *memoryIdempotency) Complete(_ context.Context, _ Reservation, result Created) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.completeErr != nil {
		return store.completeErr
	}
	store.value.State, store.value.Result = stateCompleted, &result
	return nil
}

// Abandon 模拟安全失败后的删除，后续请求因此可以重新占位。
func (store *memoryIdempotency) Abandon(context.Context, Reservation) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.present = false
	return nil
}

// Review 保留记录，只把状态切成不能重新执行的待核对。
func (store *memoryIdempotency) Review(_ context.Context, _ Reservation, id int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.value.State, store.value.OrderID = stateNeedsReview, id
	return nil
}

// fixture 把同一组替身放在一起，让每个测试只修改与当前失败位置有关的字段。
type fixture struct {
	service    *Service           // 被测业务流程。
	repository *fakeRepository    // 可观察订单创建次数。
	price      *fakePrice         // 可改变价格或查询错误。
	stock      *fakeStock         // 可观察扣减与补偿次数。
	publisher  *fakePublisher     // 可观察发送次数。
	store      *memoryIdempotency // 可检查最终占位状态。
	reviews    *fakeReviews       // 可检查告警计数调用次数。
}

// newFixture 默认全部成功，失败用例只覆盖一个环节。
func newFixture() fixture {
	value := fixture{repository: &fakeRepository{}, price: &fakePrice{price: 100}, stock: &fakeStock{}, publisher: &fakePublisher{}, store: &memoryIdempotency{}, reviews: &fakeReviews{}}
	value.service = NewService(value.repository, value.price, value.stock, value.publisher, value.store, value.reviews, time.Second, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return value
}

// TestCreateFailureBoundaries 检查失败之后系统留下的状态，而不只检查函数返回值。
func TestCreateFailureBoundaries(t *testing.T) {
	failed := errors.New("预期失败")
	for _, testcase := range []struct {
		name                           string         // 描述失败发生在哪一步。
		setup                          func(*fixture) // 只改变当前用例需要的依赖。
		want                           error          // 最后返回的业务错误。
		deduct, save, publish, release int32          // 四个有业务效果的步骤各执行几次。
		state                          string         // 空字符串表示安全删除，其他值表示保留的状态。
	}{
		{"正常创建", func(*fixture) {}, nil, 1, 1, 1, 0, stateCompleted},                                                                                                     // 成功后保存回放结果。
		{"价格不存在", func(f *fixture) { f.price.err = product.ErrNotFound }, product.ErrNotFound, 0, 0, 0, 0, ""},                                                           // 尚未动库存，可以删除占位。
		{"扣减明确拒绝", func(f *fixture) { f.stock.deductErr = &PersistenceError{Cause: failed} }, failed, 1, 0, 0, 0, ""},                                                    // 明确没有生效，允许重试。
		{"扣减结果不明", func(f *fixture) { f.stock.deductErr = &PersistenceError{Cause: failed, MayHaveCommitted: true} }, ErrOutcomeUnknown, 1, 0, 0, 0, stateNeedsReview},   // 不知道扣没扣，不能重扣或盲目归还。
		{"订单未提交", func(f *fixture) { f.repository.err = &PersistenceError{Cause: failed} }, failed, 1, 1, 0, 1, ""},                                                      // 归还成功后才删除占位。
		{"订单提交结果不明", func(f *fixture) { f.repository.err = &PersistenceError{Cause: failed, MayHaveCommitted: true} }, ErrOutcomeUnknown, 1, 1, 0, 0, stateNeedsReview},  // 订单可能存在，此时不能归还库存。
		{"补偿失败", func(f *fixture) { f.repository.err = &PersistenceError{Cause: failed}; f.stock.releaseErr = failed }, ErrOutcomeUnknown, 1, 1, 0, 1, stateNeedsReview}, // 保留记录供人工核对。
		{"消息发送失败", func(f *fixture) { f.publisher.err = failed }, nil, 1, 1, 1, 0, stateCompleted},                                                                       // 订单已经创建，仍保存成功结果。
		{"回放结果保存失败", func(f *fixture) { f.store.completeErr = failed }, ErrOutcomeUnknown, 1, 1, 1, 0, stateNeedsReview},                                                 // 不能因 Redis 写失败而重复下单。
		{"Redis 占位失败", func(f *fixture) { f.store.reserveErr = ErrIdempotencyUnavailable }, ErrIdempotencyUnavailable, 0, 0, 0, 0, ""},                                   // 无保护时不访问库存或数据库。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			f := newFixture()
			testcase.setup(&f)
			result, err := f.service.Create(t.Context(), CreateRequest{UserID: 1, SKUID: 1, Quantity: 2}, "request-1")
			if !errors.Is(err, testcase.want) {
				t.Fatalf("错误=%v，期望 %v", err, testcase.want)
			}
			if f.stock.deductCalls.Load() != testcase.deduct || f.repository.calls.Load() != testcase.save || f.publisher.calls.Load() != testcase.publish || f.stock.releaseCalls.Load() != testcase.release {
				t.Fatalf("扣减/订单/发送/归还=%d/%d/%d/%d", f.stock.deductCalls.Load(), f.repository.calls.Load(), f.publisher.calls.Load(), f.stock.releaseCalls.Load())
			}
			state := ""
			if f.store.present {
				state = f.store.value.State
			}
			if state != testcase.state {
				t.Fatalf("最终状态=%q，期望 %q", state, testcase.state)
			}
			if err == nil && (result.OrderID != 100 || result.AmountCents != 200) {
				t.Fatalf("金额或订单号不对：%+v", result)
			}
			if state == stateNeedsReview {
				if f.reviews.calls.Load() != 1 {
					t.Fatal("进入待核对时没有记录一次告警")
				}
				_, repeatErr := f.service.Create(t.Context(), CreateRequest{UserID: 1, SKUID: 1, Quantity: 2}, "request-1")
				if !errors.Is(repeatErr, ErrOutcomeUnknown) || f.reviews.calls.Load() != 1 {
					t.Fatal("重复请求未保留待核对状态，或重复计入状态转换")
				}
			}
		})
	}
}

// TestConcurrentIdempotency 二十个相同请求只能创建一笔订单，其他请求得到回放或处理中。
func TestConcurrentIdempotency(t *testing.T) {
	f := newFixture()
	results := make(chan error, 20)
	start := make(chan struct{})
	for range 20 {
		go func() {
			<-start
			_, err := f.service.Create(t.Context(), CreateRequest{UserID: 1, SKUID: 1, Quantity: 1}, "same")
			results <- err
		}()
	}
	close(start)
	// 全部结束后再断言次数，避免把仍在处理的请求漏掉。
	for range 20 {
		if err := <-results; err != nil && !errors.Is(err, ErrInProgress) {
			t.Fatal(err)
		}
	}
	if f.stock.deductCalls.Load() != 1 || f.repository.calls.Load() != 1 || f.publisher.calls.Load() != 1 {
		t.Fatal("相同幂等键重复产生了业务操作")
	}
	_, err := f.service.Create(t.Context(), CreateRequest{UserID: 2, SKUID: 1, Quantity: 1}, "same")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("同一键换了参数仍被接受")
	}
}

// TestCompensationSurvivesCancellation 请求取消不应阻止已经扣下的库存被归还。
func TestCompensationSurvivesCancellation(t *testing.T) {
	// 在订单写入时取消上游，专门观察随后补偿拿到的是不是独立 context。
	f := newFixture()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.repository.onCreate = cancel
	f.repository.err = &PersistenceError{Cause: context.Canceled}
	f.stock.onRelease = func(cleanup context.Context) {
		if cleanup.Err() != nil {
			t.Error("补偿继承了已取消请求")
		}
		if _, ok := cleanup.Deadline(); !ok {
			t.Error("补偿没有时间上限")
		}
	}
	_, err := f.service.Create(ctx, CreateRequest{UserID: 1, SKUID: 1, Quantity: 1}, "cancel")
	if !errors.Is(err, context.Canceled) || f.stock.releaseCalls.Load() != 1 || f.store.present {
		t.Fatalf("取消后的补偿：%v", err)
	}
}

// TestValidateCreate 检查整数上界和键格式，所有拒绝都发生在扣库存之前。
func TestValidateCreate(t *testing.T) {
	for _, testcase := range []struct {
		name    string        // 说明参数为何不合法。
		request CreateRequest // 进入业务层的已解析请求。
		key     string        // 测试键字符范围。
		price   int64         // 用于金额乘法上界测试。
	}{
		{"零数量", CreateRequest{UserID: 1, SKUID: 1}, "key", 100},                         // 不能购买零件。
		{"非法键", CreateRequest{UserID: 1, SKUID: 1, Quantity: 1}, "有空格 key", 100},        // 键只允许规定的 ASCII 字符。
		{"金额溢出", CreateRequest{UserID: 1, SKUID: 1, Quantity: 2}, "key", math.MaxInt64}, // 先除再比较，不能先做溢出的乘法。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			f := newFixture()
			f.price.price = testcase.price
			_, err := f.service.Create(t.Context(), testcase.request, testcase.key)
			if !errors.Is(err, ErrInvalidArgument) || f.stock.deductCalls.Load() != 0 {
				t.Fatalf("无效请求产生了扣减：%v", err)
			}
		})
	}
}

// TestPublishHasIndependentBudget 保证调整补偿预算不会顺带缩短 Kafka 的确认等待。
func TestPublishHasIndependentBudget(t *testing.T) {
	f := newFixture()
	f.service.cleanupTimeout = time.Millisecond
	f.service.publishTimeout = time.Second
	f.publisher.onPublish = func(ctx context.Context) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= f.service.cleanupTimeout {
			t.Error("发送仍然用了短的补偿预算")
		}
	}
	if _, err := f.service.Create(t.Context(), CreateRequest{UserID: 1, SKUID: 1, Quantity: 1}, "publish-budget"); err != nil {
		t.Fatal(err)
	}
}
