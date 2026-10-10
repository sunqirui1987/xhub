package cache

import "testing"

// TestFlush 验证空缓存、重复清理及多条响应全部失效，清理后可重新写入。
// 前置独立内存缓存；无非法键限制，空键也必须清理；不访问外部资源，无需清理。
func TestFlush(t *testing.T) {
	c := New()
	c.Flush()
	for _, key := range []string{"", "a", "b"} {
		c.Set(key, []byte("response"))
	}
	c.Flush()
	c.Flush()
	for _, key := range []string{"", "a", "b"} {
		if _, ok := c.Get(key); ok {
			t.Fatalf("清理后缓存仍命中 %q", key)
		}
	}
	c.Set("a", []byte("new"))
	if value, ok := c.Get("a"); !ok || string(value) != "new" {
		t.Fatal("清理后无法保存新响应")
	}
}

func TestKeySeparatesPartsAndCacheCopiesBytes(t *testing.T) {
	if Key("ab", "c") == Key("a", "bc") {
		t.Fatal("cache key aliases different part boundaries")
	}
	c := New()
	source := []byte("response")
	c.Set("key", source)
	source[0] = 'X'
	first, ok := c.Get("key")
	if !ok || string(first) != "response" {
		t.Fatalf("stored value changed: %q, ok=%v", first, ok)
	}
	first[0] = 'Y'
	second, _ := c.Get("key")
	if string(second) != "response" {
		t.Fatalf("returned value aliases cache storage: %q", second)
	}
}
