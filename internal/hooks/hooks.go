// 请求开始前的预算和并发闸门。超限时数据面不会发上游。
package hooks

import (
	"sync"

	"github.com/sunqirui1987/xhub/internal/store"
)

// 请求级闸门，限制单密钥预算和并发。
// PROXY_HOOKS: max_budget_limiter, parallel_request_limiter
type Engine struct {
	mu       sync.Mutex
	inflight map[string]int
}

// 创建空的闸门。并发表按密钥哈希计数。
func New() *Engine {
	return &Engine{inflight: map[string]int{}}
}

// 占用一个并发名额。预算已满返回 budget，并发已满返回 parallel，此时不要调用返回的释放函数。
func (e *Engine) Begin(k *store.Key) (func(), string) {
	if k == nil {
		return func() {}, ""
	}
	if k.MaxBudget.Valid && k.Spend >= k.MaxBudget.Float64 {
		return nil, "budget"
	}
	e.mu.Lock()
	n := e.inflight[k.TokenHash]
	if k.MaxParallel.Valid && int64(n) >= k.MaxParallel.Int64 && k.MaxParallel.Int64 > 0 {
		e.mu.Unlock()
		return nil, "parallel"
	}
	e.inflight[k.TokenHash] = n + 1
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		e.inflight[k.TokenHash]--
		e.mu.Unlock()
	}, ""
}
