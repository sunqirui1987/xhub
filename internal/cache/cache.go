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
func Key(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns a copy of the cached bytes. Changing the slice does not change the cache. A miss returns ok false.
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
func (c *DualCache) Set(key string, value []byte) {
	out := make([]byte, len(value))
	copy(out, value)
	c.c.Add(key, out)
}

// Flush removes every cached response. It does not write spend or talk to Redis.
func (c *DualCache) Flush() {
	c.c.Purge()
}
