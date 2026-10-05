// Package hooks counts a key's in-flight calls. Budget belongs to
// internal/authz, which checks the whole hierarchy before the data plane is
// reached; this package only bounds concurrency inside one process.
package hooks

import (
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Engine counts in-flight calls per key id. The count is per process, so it
// bounds one gateway instance rather than a key across a cluster.
type Engine struct {
	mu       sync.Mutex
	inflight map[string]int
}

var logTraceOnceHooks sync.Once

// New returns an empty gate. In-flight calls are counted by key id.
func New() *Engine {
	logTraceOnceHooks.Do(func() { logx.Trace("enter hooks.New") })

	return &Engine{inflight: map[string]int{}}
}

// Begin reserves one in-flight slot and returns the release function. An empty
// key id is not counted, so a caller without a key still gets a release.
func (e *Engine) Begin(keyID string) func() {
	if keyID == "" {
		return func() {}
	}
	e.mu.Lock()
	e.inflight[keyID]++
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		e.inflight[keyID]--
		if e.inflight[keyID] <= 0 {
			delete(e.inflight, keyID)
		}
		e.mu.Unlock()
	}
}
