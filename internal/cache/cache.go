package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

type DualCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func New() *DualCache {
	return &DualCache{m: map[string][]byte{}}
}

func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *DualCache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true
}

func (c *DualCache) Set(k string, v []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, len(v))
	copy(out, v)
	c.m[k] = out
}

func (c *DualCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string][]byte{}
}
