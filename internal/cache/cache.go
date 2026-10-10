// Package cache is the in-process response cache. Capacity is fixed, and reads and writes copy bytes so a caller cannot mutate a stored value.
package cache

import (
	"crypto/sha256"
	"encoding/hex"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// cacheEntries is the fixed LRU capacity. The hashicorp LRU keeps the map from growing without a limit, and the library is safe for concurrent use.
const cacheEntries = 8192

// DualCache is the in-process LRU response cache. It holds 8192 entries and is safe for concurrent use. Get and Set copy bytes.
type DualCache struct {
	c *lru.Cache[string, []byte]
}

var logTraceOnceCache sync.Once

// New builds an in-process cache of 8192 entries. It panics if that capacity is illegal, which would be a programming error.
// 参数：无。
// 调用：authz/authz.go、authz/decide.go、dataplane/serve.go、gateway/engine.go
// 测试：activity_http_test.go、authz_test.go、builtin_providers_test.go
// 返回：容量 8192 的进程内缓存。容量非法才会 panic，正常调用不会返回 nil。
func New() *DualCache {
	logTraceOnceCache.Do(func() { logx.Trace("enter cache.New") })

	c, err := lru.New[string, []byte](cacheEntries)
	if err != nil {
		// New fails only when the capacity is below 1. The constant is not that case, so a failure means the library was called incorrectly.
		panic(err)
	}
	return &DualCache{c: c}
}

// Key hashes the tenant, operation, model, and body into a cache key. Parts are separated by a zero byte so "ab"+"c" and "a"+"bc" do not collide.
// 调用：dataplane/serve.go
// 测试：failure_log_test.go
// 参数 parts（...string）：拼缓存键的片段，顺序敏感。
// 返回：sha256 十六进制。parts 用 0 字节隔开，所以 "ab"+"c" 和 "a"+"bc" 不会撞键。
func Key(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns a copy of the cached bytes. Changing the slice does not change the cache. A miss returns ok false.
// 参数 key（string）：Key 拼出的缓存键，不是模型字段名。
// 返回：缓存正文的副本，以及是否命中。未命中时正文为 nil、ok 为 false。改返回的切片不会改缓存。
// 调用：Serve 在扩展通过之后。
// 测试：failure_log_test.go TestServeLogsCacheHitAndStreamMetrics。
func (c *DualCache) Get(key string) ([]byte, bool) {
	value, ok := c.c.Get(key)
	if !ok {
		return nil, false
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out, true
}

// Set stores a copy of value. Later changes to the caller's slice do not change the cached bytes.
// 调用：dataplane/official.go、dataplane/serve.go、dataplane/stream.go、gateway/engine.go
// 测试：access_log_test.go、affinity_test.go、builtin_providers_test.go
// 参数 key（string）：缓存键。value（[]byte）：要保存的响应正文，函数会复一份，调用方之后改自己的切片不影响缓存。
// 返回：无。不写 HTTP 响应。
func (c *DualCache) Set(key string, value []byte) {
	out := make([]byte, len(value))
	copy(out, value)
	c.c.Add(key, out)
}

// Flush removes every cached response. It does not write spend or talk to Redis.
// 参数：无。
// 调用：gateway/access.go 的管理端清空缓存接口。
// 测试：cache_test.go TestFlush；regression/consistency_test.go。
// 返回：无。进程内的响应缓存已清空。不写用量，也不访问 Redis。
func (c *DualCache) Flush() {
	c.c.Purge()
}
