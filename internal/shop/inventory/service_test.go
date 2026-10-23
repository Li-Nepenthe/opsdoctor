// 本文件：离线验证参数规则、取消和仓储错误传递。
//
// fakeRepository 用调用次数证明无效请求没有触碰数据层。
// 先读 TestServiceValidation 的边界案例，再读 TestServiceCancellationAndError；并发防超卖由真实数据库测试承担，假仓储不冒充锁实现。

package inventory

import (
	"context"
	"errors"
	"testing"
)

// fakeRepository 保存单次测试的调用次数；本文件串行使用它，不拿普通 int 做并发计数。
type fakeRepository struct {
	calls int   // calls 证明非法参数不会进入数据层。
	err   error // err 是由测试注入的领域或底层错误。
}

// GetStock 返回固定快照，把业务层测试与 MySQL 分离。
func (repository *fakeRepository) GetStock(_ context.Context, skuID int64) (Stock, error) {
	repository.calls++
	return Stock{SKUID: skuID, Available: 50}, repository.err
}

// Deduct 只统计调用，用来验证参数校验发生在仓储之前。
func (repository *fakeRepository) Deduct(context.Context, int64, int32) error {
	repository.calls++
	return repository.err
}

// Release 与 Deduct 使用相同的假错误来源，验证补偿入口同样遵守业务规则。
func (repository *fakeRepository) Release(context.Context, int64, int32) error {
	repository.calls++
	return repository.err
}

// TestServiceValidation 检查三种入口的边界，并证明仓储错误不会被业务层吞掉。
func TestServiceValidation(t *testing.T) {
	cases := []struct {
		name      string // name 直接说明这个用例要阻止哪种非法调用。
		operation string // operation 选择业务入口，而不是绕过 service 直接调用假仓储。
		sku       int64  // sku 可为零或负数，覆盖编号边界。
		quantity  int32  // quantity 对查询无意义，对写操作必须为正。
		want      Error  // want 为空表示合法输入。
	}{
		{"查询合法商品", "get", 1, 1, ""},                    // 正常查询应进入仓储。
		{"查询零编号", "get", 0, 1, ErrInvalidArgument},     // protobuf 缺字段会得到 0，不能继续查库。
		{"扣减负编号", "deduct", -1, 1, ErrInvalidArgument}, // 负 ID 不属于商品域。
		{"扣减零件", "deduct", 1, 0, ErrInvalidArgument},   // 不能用空更新冒充成功扣减。
		{"扣减负数", "deduct", 1, -1, ErrInvalidArgument},  // 防止负数扣减变成加库存。
		{"合法扣减", "deduct", 1, 1, ""},                   // 正常写入转交原子仓储实现。
		{"归还负数", "release", 1, -1, ErrInvalidArgument}, // 归还不能绕过扣减规则。
		{"合法归还", "release", 1, 1, ""},                  // 补偿入口也能正常调用仓储。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			repository := &fakeRepository{}
			service := NewService(repository)
			err := callService(service, t.Context(), testcase.operation, testcase.sku, testcase.quantity)
			if testcase.want == "" {
				if err != nil || repository.calls != 1 {
					t.Fatalf("合法调用：calls=%d err=%v", repository.calls, err)
				}
			} else if !errors.Is(err, testcase.want) || repository.calls != 0 {
				t.Fatalf("非法调用：calls=%d err=%v", repository.calls, err)
			}
		})
	}
}

// callService 让同一组边界案例覆盖三个公开方法，保持测试断言集中。
func callService(service *Service, ctx context.Context, operation string, sku int64, quantity int32) error {
	switch operation {
	case "get":
		_, err := service.GetStock(ctx, sku)
		return err
	case "deduct":
		return service.Deduct(ctx, sku, quantity)
	default:
		return service.Release(ctx, sku, quantity)
	}
}

// TestServiceCancellationAndError 确认取消不访问仓储、仓储错误保持可识别，不被重新制造成普通字符串。
func TestServiceCancellationAndError(t *testing.T) {
	for _, operation := range []string{"get", "deduct", "release"} {
		repository := &fakeRepository{err: ErrUnavailable}
		service := NewService(repository)
		// t.Context 随测试结束而取消；派生 cancel 用来模拟调用方主动放弃。
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := callService(service, ctx, operation, 1, 1); !errors.Is(err, context.Canceled) || repository.calls != 0 {
			t.Fatalf("%s 取消后仍访问仓储：%v", operation, err)
		}
		if err := callService(service, t.Context(), operation, 1, 1); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s 丢失仓储错误：%v", operation, err)
		}
	}
}
