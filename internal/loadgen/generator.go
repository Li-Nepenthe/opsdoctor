// 本文件：按目标速率分配订单服务的请求，到固定上限就记录本次未发出。
// 普通请求与成批商品查询共用工作者。随机抽样和下单幂等键都由一个调度循环生成，工作者只负责执行。

package loadgen

import (
	"context"
	"crypto/rand"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"log/slog"
	mathrand "math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// requestKind 的连续编号也用于统计数组下标，不接受外部输入直接设置。
type requestKind uint8

// 请求比例在 nextJob 中定义，这里只给三种类型起名字。
const (
	productRequest requestKind = iota // 查商品，占约七成。
	orderRequest                      // 下单，占约两成。
	listRequest                       // 查用户订单列表，占约一成。
)

// 调度和退出都采用有限预算，运行越久也不会积累更多工作者。
const (
	scheduleInterval  = 5 * time.Millisecond // 默认 200 QPS 平均每五毫秒安排一次，避免一秒一大批。
	requestAttempts   = 2                    // 首次下单加一次同键超时重试，其他请求只发一次。
	workerDrainMargin = 2 * time.Second      // 在两次请求预算之外留出调度与退出余量。
	zipfShape         = 1.1                  // 大于一，让少数商品比长尾更常被访问。
	zipfOffset        = 100                  // 分散最热的一组商品，避免一次基准就耗尽首个商品的库存。
)

// job 是一次逻辑操作。超时重试继续使用同一份字段。
type job struct {
	kind   requestKind
	skuID  int64       // 商品查询与下单使用同一套热门分布。
	userID int64       // 在已造数的常见用户范围内选择。
	key    string      // 每笔逻辑下单独立生成，重试不更换。
	burst  *burstBatch // 非 nil 时只查商品，并等待同批工作者一起出发。
}

// generator 把调度数据和发送依赖放在一起，随机源只允许 schedule 所在 goroutine 使用。
type generator struct {
	settings Config
	control  *controlState      // 控制接口修改速率与热点，内部用锁保护。
	bursts   *burstState        // 有上限的单次突发也交给同一组工作者。
	client   *http.Client       // 复用连接，所有工作者共享。
	metrics  *telemetry.Metrics // app 注入指标，与日志统计同时记下每次尝试。
	stats    *statistics
	logger   *slog.Logger
	random   *mathrand.Rand
	zipf     *mathrand.Zipf
	runID    string // 每次启动换一个前缀，重启后也不会沿用旧幂等键。
	sequence uint64 // 仅调度循环增加，和 runID 一起组成键。
}

// 默认把热门请求分散到一组商品，避免短时间内耗尽单个商品的库存。指定热点时仍然只选该商品。
func newGenerator(settings Config, control *controlState, client *http.Client, logger *slog.Logger, metrics *telemetry.Metrics) *generator {
	random := mathrand.New(mathrand.NewPCG(mathrand.Uint64(), mathrand.Uint64()))
	return &generator{metrics: metrics, settings: settings, control: control, client: client, logger: logger, random: random,
		bursts: &burstState{queue: make(chan *burstBatch, 1), workers: settings.Workers, productCount: settings.ProductCount},
		zipf:   mathrand.NewZipf(random, zipfShape, zipfOffset, uint64(settings.ProductCount-1)), runID: rand.Text(), stats: newStatistics(time.Now())}
}

// nextJob 用一次百分位抽签选择接口，因此长期接近 70/20/10，不保证每十个请求都恰好凑齐。
func (generator *generator) nextJob(hotSKU int64) job {
	value := job{kind: productRequest, skuID: int64(generator.zipf.Uint64()) + 1, userID: int64(generator.random.IntN(1000)) + 1}
	if hotSKU != 0 {
		value.skuID = hotSKU
	}
	choice := generator.random.IntN(100)
	if choice >= 90 {
		value.kind = listRequest
	} else if choice >= 70 {
		value.kind = orderRequest
	}
	if value.kind == orderRequest {
		generator.sequence++
		value.key = generator.runID + "-" + strconv.FormatUint(generator.sequence, 10)
	}
	return value
}

// run 停止时先停调度，再让正在发送的请求结束。工作者数量固定，不会随 QPS 增长。
func (generator *generator) run(ctx context.Context) {
	workContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	jobs := make(chan job) // 不积压队列，所有工作者忙时由调度器记录 dropped。
	var workers sync.WaitGroup
	for range generator.settings.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			generator.work(workContext, jobs)
		}()
	}
	generator.schedule(ctx, jobs)
	close(jobs) // 只有调度者关闭，工作者只接收，避免向已关闭通道发送。
	finished := make(chan struct{})
	go func() { workers.Wait(); close(finished) }()
	// 一笔下单最多两次尝试，给两次请求预算再留两秒退出余量。
	timer := time.NewTimer(requestAttempts*generator.settings.RequestTimeout + workerDrainMargin)
	defer timer.Stop()
	select {
	case <-finished:
	case <-timer.C:
		cancel()
		<-finished
	}
}

func (generator *generator) work(ctx context.Context, jobs <-chan job) {
	for value := range jobs {
		if value.burst != nil {
			// 调度者把这一批交给工作者后统一放行，避免先领到任务的人已经查完缓存。
			<-value.burst.start
		}
		failed := generator.execute(ctx, value)
		if value.burst != nil {
			generator.bursts.finish(value.burst.id, failed, 0)
		}
	}
}

// schedule 每五毫秒积累“应发送多少次”的额度，实际完成 QPS 由 statistics 独立计算。
func (generator *generator) schedule(ctx context.Context, jobs chan<- job) {
	ticker := time.NewTicker(scheduleInterval)
	defer ticker.Stop()
	last := time.Now()
	credit := 0.0
	for {
		select {
		case <-ctx.Done():
			return
		case batch := <-generator.bursts.queue:
			generator.dispatchBurst(ctx, jobs, batch)
			// 突发占用了调度时间，不把这段时间积累成下一轮突然补发的普通流量。
			last, credit = time.Now(), 0
		case now := <-ticker.C:
			settings := generator.control.snapshot()
			elapsed := max(0, now.Sub(last).Seconds()) // ticker 可能留下突发之前的一次通知。
			last = now
			if settings.QPS == 0 {
				credit = 0
				continue
			}
			credit += elapsed * float64(settings.QPS)
			var running bool
			credit, running = generator.dispatchRegular(ctx, jobs, settings.HotSKUID, credit)
			if !running {
				return
			}
		}
	}
}

// 整数额度逐个交给已有工作者，剩下的小数留到下次计时。
func (generator *generator) dispatchRegular(ctx context.Context, jobs chan<- job, hotSKU int64, credit float64) (float64, bool) {
	for credit >= 1 {
		credit--
		value := generator.nextJob(hotSKU)
		select {
		case <-ctx.Done():
			return credit, false
		case jobs <- value:
		default:
			// 没有空闲工作者时不排长队，报告 dropped 才能看出生成器自身的容量限制。
			generator.stats.drop()
		}
	}
	return credit, true
}
