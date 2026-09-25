// 进程内响应缓存。容量固定，读写都复制字节，避免调用方改到缓存内容。
package cache

import (
	"crypto/sha256"
	"encoding/hex"

	lru "github.com/hashicorp/golang-lru/v2"
)

// DualCache 是进程内的响应缓存。
//
// 容量交给 hashicorp/golang-lru，避免自己用 map 一直涨到把内存吃完。
// 库本身保证并发安全。取出和写入都复制字节，调用方之后改自己的切片不会改到缓存。
const cacheEntries = 8192

// 进程内 LRU 响应缓存。容量固定为 8192，并发安全。
type DualCache struct {
	c *lru.Cache[string, []byte]
}

// 创建容量为 8192 的进程内缓存。容量常量不合法时直接 panic，因为那是编程错误。
func New() *DualCache {
	c, err := lru.New[string, []byte](cacheEntries)
	if err != nil {
		// 容量小于 1 才会失败。常量不是这个情况，失败就说明库的用法错了。
		panic(err)
	}
	return &DualCache{c: c}
}

// Key 把一次调用的租户、操作、模型和请求体收成缓存键。
// 字段之间用 0 字节隔开，避免 "ab"+"c" 和 "a"+"bc" 撞成同一个键。
func Key(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// 按键取出缓存。返回的切片是副本，调用方修改不会改到缓存。未命中时 ok 为 false。
func (c *DualCache) Get(key string) ([]byte, bool) {
	value, ok := c.c.Get(key)
	if !ok {
		return nil, false
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out, true
}

// 写入缓存。保存的是副本，之后修改入参不会影响已缓存的字节。
func (c *DualCache) Set(key string, value []byte) {
	out := make([]byte, len(value))
	copy(out, value)
	c.c.Add(key, out)
}

// 把队列中的花费日志写入 PostgreSQL。事务成功后才确认 Redis。确认失败时两边都保留。
func (c *DualCache) Flush() {
	c.c.Purge()
}
