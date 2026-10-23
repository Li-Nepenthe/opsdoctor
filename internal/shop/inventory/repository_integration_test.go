// 本文件：真实 MySQL 的并发扣减与 deadline 实验。
//
// 测试只接受专用库，避免准备数据时碰到 shop。
// 先读 integrationDatabase，再读 100 个 goroutine 的同步起跑与结果收集。slowRepository 仅在测试中延迟查询，生产代码没有故障开关。
//
// 主要内容：
//   - integrationDatabase：只允许专用测试库。每个测试串行清理 stock，不可能清空 shop 业务库。
//   - TestIntegrationConcurrentDeduct：让 100 个 goroutine 同时争抢 50 件。真实 InnoDB 锁与条件更新决定胜负。
//   - TestIntegrationStockBoundaries：检查不存在、归还和整数上界，覆盖单条 UPDATE 的零影响行数分支。
//   - GetStock：用 uk_stock_sku 找行后人为等待五秒。普通 SELECT 不持有排他行锁，ctx 应提前结束驱动等待。
//   - TestIntegrationDeadline：验证真实 gRPC deadline 穿过 service，令驱动查询等待和服务端 handler 提前返回。

package inventory

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// integrationDatabase 只允许专用测试库。每个测试串行清理 stock，不可能清空 shop 业务库。
func integrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_INVENTORY_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_INVENTORY_MYSQL_DSN 为 opsdoctor_inventory_test 专用库以开启真实 MySQL 测试，见 README")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_inventory_test" {
		t.Fatal("测试只允许 opsdoctor_inventory_test，未连接或清理任何库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 20, MaxIdleConns: 10, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrations.Apply(t.Context(), database); err != nil {
		t.Fatal(err)
	}
	// DELETE 全表只发生在被白名单保护的测试库。清理会锁住库存测试行，不与其他测试并行运行。
	if _, err := database.ExecContext(t.Context(), "DELETE FROM stock"); err != nil {
		t.Fatal(err)
	}
	// uk_stock_sku 保证唯一。准备 50 件正好对应计划书的并发实验。
	if _, err := database.ExecContext(t.Context(), "INSERT INTO stock (sku_id, available, updated_at) VALUES (1, 50, UTC_TIMESTAMP(6))"); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestIntegrationConcurrentDeduct 让 100 个 goroutine 同时争抢 50 件。真实 InnoDB 锁与条件更新决定胜负。
func TestIntegrationConcurrentDeduct(t *testing.T) {
	database := integrationDatabase(t)
	service := NewService(NewMySQLStockRepository(database))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second) // 避免数据库异常导致整个并发测试无限等待。
	defer cancel()
	start := make(chan struct{})     // 主测试唯一关闭，100 个 worker 只接收；关闭相当于同时放行。
	results := make(chan error, 100) // 每个 worker 写一次，主测试收齐；缓冲装得下全部结果，Wait 不会死锁。
	var workers sync.WaitGroup       // WaitGroup 只负责等待完成，不用它保护业务数据。
	for range 100 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- service.Deduct(ctx, 1, 1)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	success, insufficient := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrInsufficientStock) {
			insufficient++
		} else {
			t.Fatalf("非预期失败：%v", err)
		}
	}
	stock, err := service.GetStock(ctx, 1)
	if err != nil || success != 50 || insufficient != 50 || stock.Available != 0 {
		t.Fatalf("success=%d insufficient=%d stock=%+v err=%v", success, insufficient, stock, err)
	}
	t.Logf("100 次并发扣减：成功=%d，不足=%d，最终库存=%d", success, insufficient, stock.Available)
}

// TestIntegrationStockBoundaries 检查不存在、归还和整数上界，覆盖单条 UPDATE 的零影响行数分支。
func TestIntegrationStockBoundaries(t *testing.T) {
	repository := NewMySQLStockRepository(integrationDatabase(t))
	if err := repository.Deduct(t.Context(), 99, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的扣减：%v", err)
	}
	if err := repository.Release(t.Context(), 99, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的归还：%v", err)
	}
	if err := repository.Release(t.Context(), 1, 2); err != nil {
		t.Fatal(err)
	}
	stock, err := repository.GetStock(t.Context(), 1)
	if err != nil || stock.Available != 52 {
		t.Fatalf("归还结果：%+v %v", stock, err)
	}
	// 归还 int32 最大值会越界，条件 UPDATE 应拒绝，并保持原库存。
	if err := repository.Release(t.Context(), 1, 2147483647); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("上界：%v", err)
	}
	stock, err = repository.GetStock(t.Context(), 1)
	if err != nil || stock.Available != 52 {
		t.Fatalf("越界后库存：%+v %v", stock, err)
	}
}

// slowRepository 仅用于集成测试：保留正常扣减/归还，用带 SLEEP 的真实查询延迟 GetStock。
type slowRepository struct {
	*MySQLStockRepository            // 嵌入正常仓储，复用其他方法；生产实现没有故障开关。
	finished              chan error // 查询 goroutine 写一次，测试读一次；容量 1 保证测试提前失败也不阻塞写入。
}

// GetStock 用 uk_stock_sku 找行后人为等待五秒。普通 SELECT 不持有排他行锁，ctx 应提前结束驱动等待。
func (repository *slowRepository) GetStock(ctx context.Context, skuID int64) (Stock, error) {
	const slowQuery = `SELECT sku_id, available FROM stock WHERE sku_id = ? AND SLEEP(5) = 0`
	var stock Stock
	err := repository.database.QueryRowContext(ctx, slowQuery, skuID).Scan(&stock.SKUID, &stock.Available)
	err = classifyDatabaseError(ctx, err)
	repository.finished <- err
	return stock, err
}

// TestIntegrationDeadline 验证真实 gRPC deadline 穿过 service，令驱动查询等待和服务端 handler 提前返回。
func TestIntegrationDeadline(t *testing.T) {
	repository := &slowRepository{MySQLStockRepository: NewMySQLStockRepository(integrationDatabase(t)), finished: make(chan error, 1)}
	client := testClient(t, repository)
	// 先完成一次正常 RPC，让 100ms 实验集中测查询取消，不把首次建立内存连接的时间算进去。
	if _, err := client.Deduct(t.Context(), &inventoryv1.DeductRequest{SkuId: 1, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.GetStock(ctx, &inventoryv1.GetStockRequest{SkuId: 1})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("客户端状态：%v", err)
	}
	select {
	case serverErr := <-repository.finished:
		if !errors.Is(serverErr, context.DeadlineExceeded) && !errors.Is(serverErr, context.Canceled) {
			t.Fatalf("服务端未感知取消：%v", serverErr)
		}
		t.Logf("100ms deadline：客户端=%s，服务端=%v，总等待=%s（原查询 5s）", status.Code(err), serverErr, time.Since(started))
	case <-time.After(2 * time.Second):
		// 测试预算远小于 SLEEP(5)，可以区分“真正结束等待”和“等慢 SQL 自己做完”。
		t.Fatal("服务端在两秒内没有停止查询等待")
	}
}
