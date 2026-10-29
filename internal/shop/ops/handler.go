// 本文件：只接收调整连接池和预热缓存这两类运维请求。
//
// 请求不能指定 SQL、缓存键或后端地址。连接池修改还要带上审批时看到的进程编号
// 和旧值，避免审批期间服务重启或被别人修改后，执行器仍照旧操作。
//
// 主要内容：New 绑定本进程资源；ServeHTTP 鉴权与分流；pool 条件修改；warm 逐个补缓存。
// 阅读建议：PoolState 的 Initial 来自启动配置，供实验工具发现运行时改动。

package ops

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// Pool 只暴露当前上限与修改方法，测试可以用小型替身而不连接 MySQL。
type Pool interface {
	Stats() sql.DBStats
	SetMaxOpenConns(int)
}

// PoolState 把进程身份与旧值一起交给审批，重启后的同一个数值不代表同一份状态。
type PoolState struct {
	Instance      string `json:"instance"`       // 每次进程启动重新生成。
	Initial       int    `json:"initial"`        // 配置文件给的起始上限。
	Current       int    `json:"current"`        // 此刻真实连接池上限。
	Revision      int    `json:"revision"`       // 每次有效修改增加一次。
	LastOperation string `json:"last_operation"` // 最近成功执行的审批编号。
}

// Handler 的锁只保护池状态；预热按最多一百个 ID 顺序处理，不无限开 goroutine。
type Handler struct {
	mu         sync.Mutex
	token      [32]byte
	poolSource Pool
	state      PoolState
	products   product.Repository
	cache      product.Cache
}

// New 不给短令牌补默认值；只有显式配置了身份才开放运维入口。
func New(token string, pool Pool, products product.Repository, cache product.Cache) (*Handler, error) {
	if len(token) < 32 || pool == nil || (products == nil) != (cache == nil) {
		return nil, fmt.Errorf("运维令牌或资源配置不完整")
	}
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	initial := pool.Stats().MaxOpenConnections
	return &Handler{token: sha256.Sum256([]byte(token)), poolSource: pool, products: products, cache: cache, state: PoolState{Instance: hex.EncodeToString(nonce[:]), Initial: initial, Current: initial}}, nil
}

// ServeHTTP 先鉴权再解析，错误响应不带令牌、数据库错误或商品内容。
func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 保存的是令牌摘要，所以这里只移除精确前缀；没有前缀直接拒绝。
	value := r.Header.Get("Authorization")
	if len(value) < 7 || value[:7] != "Bearer " {
		http.Error(w, "运维鉴权失败", http.StatusUnauthorized)
		return
	}
	candidate := sha256.Sum256([]byte(value[7:]))
	if subtle.ConstantTimeCompare(candidate[:], handler.token[:]) != 1 {
		http.Error(w, "运维鉴权失败", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	switch {
	case r.URL.Path == "/ops/pool" && (r.Method == "GET" || r.Method == "POST"):
		handler.pool(w, r)
	case r.URL.Path == "/ops/cache/warm" && r.Method == "POST":
		handler.warm(w, r)
	default:
		http.NotFound(w, r)
	}
}

// pool 的比较与修改在同一把锁中，两个审批不会同时依据同一个旧值执行。
func (handler *Handler) pool(w http.ResponseWriter, r *http.Request) {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	handler.state.Current = handler.poolSource.Stats().MaxOpenConnections
	if r.Method == "GET" {
		respond(w, handler.state)
		return
	}
	var input struct {
		Operation string `json:"operation"`
		Instance  string `json:"instance"`
		Revision  int    `json:"revision"`
		Before    int    `json:"before"`
		Size      int    `json:"size"`
	}
	if decode(w, r, &input) != nil || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(input.Operation) || input.Size < 5 || input.Size > 200 {
		http.Error(w, "连接池参数不合法，允许范围为 5–200", http.StatusBadRequest)
		return
	}
	if input.Operation == handler.state.LastOperation {
		// 回复丢失后重试同一审批，不再次改动池，也不增加修订号。
		if input.Instance != handler.state.Instance || input.Size != handler.state.Current {
			http.Error(w, "操作编号已使用且状态不符", http.StatusConflict)
			return
		}
		respond(w, handler.state)
		return
	}
	if input.Instance != handler.state.Instance || input.Revision != handler.state.Revision || input.Before != handler.state.Current {
		http.Error(w, "审批后连接池状态已变化，请重新申请", http.StatusConflict)
		return
	}
	handler.poolSource.SetMaxOpenConns(input.Size)
	handler.state.Current = input.Size
	handler.state.Revision++
	handler.state.LastOperation = input.Operation
	respond(w, handler.state)
}

// warm 只回源缺失项；崩溃后重试不会改业务数据，已经存在的缓存也不被刷新 TTL。
func (handler *Handler) warm(w http.ResponseWriter, r *http.Request) {
	if handler.products == nil {
		http.Error(w, "该服务没有商品缓存", http.StatusNotFound)
		return
	}
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if decode(w, r, &input) != nil || len(input.IDs) < 1 || len(input.IDs) > 100 {
		http.Error(w, "每次必须提供 1–100 个商品 ID", http.StatusBadRequest)
		return
	}
	seen := map[int64]bool{}
	for _, id := range input.IDs {
		if id <= 0 || seen[id] {
			http.Error(w, "商品 ID 必须为正整数且不重复", http.StatusBadRequest)
			return
		}
		seen[id] = true
	}
	// 一次中断可能已经写入前几个商品。逐项先看缓存，恢复时只补还缺的部分。
	loaded, existing := 0, 0
	for _, id := range input.IDs {
		_, hit, err := handler.cache.Get(r.Context(), id)
		if hit && (err == nil || errors.Is(err, product.ErrNotFound)) {
			existing++
			continue
		}
		if err != nil {
			http.Error(w, "读取缓存未完成", http.StatusServiceUnavailable)
			return
		}
		value, err := handler.products.Get(r.Context(), id)
		missing := errors.Is(err, product.ErrNotFound)
		if err != nil && !missing {
			http.Error(w, "预热查询未完成", http.StatusServiceUnavailable)
			return
		}
		if err = handler.cache.Set(r.Context(), id, value, missing); err != nil {
			http.Error(w, "保存缓存未完成", http.StatusServiceUnavailable)
			return
		}
		loaded++
	}
	respond(w, map[string]int{"loaded": loaded, "already_present": existing})
}

// decode 拒绝重复字段和未知字段，不能让审批展示的参数与执行器理解的不一样。
func decode(w http.ResponseWriter, r *http.Request, value any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		return err
	}
	// 标准解码器允许重复键，先独立扫描一次，再用结构体拒绝未知字段。
	keys := json.NewDecoder(bytes.NewReader(raw))
	first, err := keys.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("需要对象")
	}
	seen := map[string]bool{}
	for keys.More() {
		key, err := keys.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return fmt.Errorf("参数重复")
		}
		seen[name] = true
		var ignore json.RawMessage
		if err = keys.Decode(&ignore); err != nil {
			return err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("只接受一个对象")
	}
	return nil
}

// respond 的内容都由服务生成，接口不会返回缓存里的商品资料。
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// Modified 记录是否曾在本次进程生命周期里改过池；即使手工改回同一个值，也要求重启恢复基线。
func (handler *Handler) Modified() bool {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.state.Revision > 0 || handler.poolSource.Stats().MaxOpenConnections != handler.state.Initial
}
