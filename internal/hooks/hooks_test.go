package hooks

import "testing"

func TestReleaseIsIdempotentWithOtherCallsInFlight(t *testing.T) {
	e := New()
	a, b := e.Begin("key"), e.Begin("key")
	a()
	a()
	if e.inflight["key"] != 1 {
		t.Fatalf("double release changed another call: %#v", e.inflight)
	}
	b()
	if len(e.inflight) != 0 {
		t.Fatalf("leaked call: %#v", e.inflight)
	}
}
