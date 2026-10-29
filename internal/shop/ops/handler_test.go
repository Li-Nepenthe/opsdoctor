// 本文件：验证业务端运维接口自己的限制，不依赖医生端先过滤参数。
//
// 测试用假的连接池和缓存，不能改本机实验。越界、身份错误与审批期间状态变化
// 都必须在实际操作之前被挡住。
//
// 主要内容：TestPoolBoundary 检查令牌、范围、条件更新与重试；TestWarmBoundary 检查商品范围。
// 阅读建议：不仅看 HTTP 状态，还看替身记录了几次真正的修改。

package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// fakePool 记录实际修改次数，重复请求返回成功不应再改第二次。
type fakePool struct{ size, changes int }

// Stats 提供当前池上限。
func (pool *fakePool) Stats() sql.DBStats { return sql.DBStats{MaxOpenConnections: pool.size} }

// SetMaxOpenConns 同时计数，发现重复执行。
func (pool *fakePool) SetMaxOpenConns(size int) { pool.size = size; pool.changes++ }

// TestPoolBoundary 一张审批两次发送也只改一次，另一个过时审批不能覆盖它。
func TestPoolBoundary(t *testing.T) {
	pool := &fakePool{size: 20}
	token := strings.Repeat("x", 32)
	handler, _ := New(token, pool, nil, nil)
	call := func(body, auth string) int {
		r := httptest.NewRequest("POST", "/ops/pool", strings.NewReader(body))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	good := fmt.Sprintf(`{"operation":"%s","instance":"%s","revision":0,"before":20,"size":30}`, strings.Repeat("a", 32), handler.state.Instance)
	// 不同用例分别验证匿名、越界、重复字段与任意主机不能进运维操作。
	for _, test := range []struct {
		name, body, auth string
		status           int
	}{
		{"匿名", good, "", 401},
		{"错误令牌", good, "Bearer " + strings.Repeat("y", 32), 401},
		{"低于范围", strings.Replace(good, `"size":30`, `"size":4`, 1), "Bearer " + token, 400},
		{"高于范围", strings.Replace(good, `"size":30`, `"size":201`, 1), "Bearer " + token, 400},
		{"重复字段", strings.Replace(good, `"size":30`, `"size":30,"size":40`, 1), "Bearer " + token, 400},
		{"任意地址", strings.Replace(good, `"size":30`, `"size":30,"url":"http://other"`, 1), "Bearer " + token, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := call(test.body, test.auth); got != test.status || pool.changes != 0 {
				t.Fatalf("status=%d changes=%d", got, pool.changes)
			}
		})
	}
	for range 2 {
		if got := call(good, "Bearer "+token); got != 200 {
			t.Fatal(got)
		}
	}
	if pool.changes != 1 || pool.size != 30 {
		t.Fatal("重复执行")
	}
	stale := strings.Replace(good, strings.Repeat("a", 32), strings.Repeat("b", 32), 1)
	if call(stale, "Bearer "+token) != 409 || pool.changes != 1 {
		t.Fatal("过时参数被执行")
	}
}

// fakeProducts 只支持按编号查一个商品，避免预热偷偷获得其他业务能力。
type fakeProducts struct{}

// Get 生成一条商品，避免依赖数据库。
func (fakeProducts) Get(_ context.Context, id int64) (product.Product, error) {
	return product.Product{ID: id}, nil
}

// fakeCache 保存预热结果，第二次同 ID 应直接复用。
type fakeCache struct {
	ids    map[int64]bool
	writes int
}

// Get 让第二次预热能看到第一次已保存的值。
func (cache *fakeCache) Get(_ context.Context, id int64) (product.Product, bool, error) {
	return product.Product{ID: id}, cache.ids[id], nil
}

// Set 记录写入数量，用来检查重复执行。
func (cache *fakeCache) Set(_ context.Context, id int64, _ product.Product, _ bool) error {
	cache.ids[id] = true
	cache.writes++
	return nil
}

// TestWarmBoundary 证明最多一百项，并且部分成功后的重试不会重复补已经存在的缓存。
func TestWarmBoundary(t *testing.T) {
	token := strings.Repeat("x", 32)
	cache := &fakeCache{ids: map[int64]bool{}}
	handler, _ := New(token, &fakePool{size: 20}, fakeProducts{}, cache)
	call := func(raw string) int {
		r := httptest.NewRequest("POST", "/ops/cache/warm", strings.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	// 空列表、重复编号和负数都在读取缓存之前拒绝。
	for _, raw := range []string{`{"ids":[]}`, `{"ids":[1,1]}`, `{"ids":[-1]}`} {
		if call(raw) != 400 {
			t.Fatal(raw)
		}
	}
	ids := make([]int64, 101)
	for index := range ids {
		ids[index] = int64(index + 1)
	}
	raw, _ := json.Marshal(map[string]any{"ids": ids})
	if call(string(raw)) != 400 || cache.writes != 0 {
		t.Fatal("超限预热")
	}
	for range 2 {
		if call(`{"ids":[1,2]}`) != 200 {
			t.Fatal("合法预热失败")
		}
	}
	if cache.writes != 2 {
		t.Fatal("重复刷新缓存")
	}
}
