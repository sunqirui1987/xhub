package cache

import "testing"

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
