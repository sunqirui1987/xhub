// Package hooks gates a request on budget and concurrency before the data plane contacts an upstream.
package hooks

import (
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Engine limits one key's budget and in-flight calls. It covers the max-budget and parallel-request checks that run before an upstream call.
type Engine struct {
	mu       sync.Mutex
	inflight map[string]int
}

var logTraceOnceHooks sync.Once

// New returns an empty gate. In-flight calls are counted by key hash.
func New() *Engine {
	logTraceOnceHooks.Do(func() { logx.Trace("enter hooks.New") })

	return &Engine{inflight: map[string]int{}}
}

// Begin reserves one in-flight slot. It returns "budget" or "parallel" when that limit is already full, and the release function is nil in those cases.
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
