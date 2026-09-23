package hooks

import (
	"sync"

	"github.com/sunqirui1987/xhub/internal/store"
)

// PROXY_HOOKS: max_budget_limiter, parallel_request_limiter
type Engine struct {
	mu   sync.Mutex
	inflight map[string]int
}

func New() *Engine {
	return &Engine{inflight: map[string]int{}}
}

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
