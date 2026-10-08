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
// 参数：无。
// 返回 *Engine（*Engine）：空的进程内并发计数器，按密钥 id 计数。不会返回 nil。
// 调用：gateway/server.go 在组装进程时。
// 测试：dataplane/bypass_logic_test.go、dataplane/failure_log_test.go。
func New() *Engine {
	logTraceOnceHooks.Do(func() { logx.Trace("enter hooks.New") })

	return &Engine{inflight: map[string]int{}}
}

// Begin reserves one in-flight slot and returns the release function. An empty key id is not counted, so a caller without a key still gets a release.
// 参数 keyID（string）：密钥 id。空串表示没有指定密钥。
// 返回 func（func）：请求结束时要调用的释放函数。不需要释放时可能为 nil。
// 调用：dataplane/serve.go、iam/db.go
// 测试：无直接单测
func (e *Engine) Begin(keyID string) func() {
	if keyID == "" {
		return func() {}
	}
	e.mu.Lock()
	e.inflight[keyID]++
	e.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Lock()
			e.inflight[keyID]--
			if e.inflight[keyID] <= 0 {
				delete(e.inflight, keyID)
			}
			e.mu.Unlock()
		})
	}
}
