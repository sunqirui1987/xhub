package plugin

import (
	"reflect"
	"testing"
)

type testExtension struct {
	name   string
	calls  int
	decide func(Call) Decision
}

func (e *testExtension) Name() string {
	e.calls++
	return e.name
}

func (e *testExtension) BeforeUpstream(call Call) Decision {
	if e.decide == nil {
		return Decision{}
	}
	return e.decide(call)
}

func TestRegistryZeroValueRegistersWithStableName(t *testing.T) {
	var registry Registry
	ext := &testExtension{name: "policy"}
	if err := registry.Register(ext); err != nil {
		t.Fatalf("register zero-value registry: %v", err)
	}
	if ext.calls != 1 {
		t.Fatalf("Name called %d times during registration, want 1", ext.calls)
	}
	ext.name = "changed-after-registration"
	if got := registry.Names(); !reflect.DeepEqual(got, []string{"policy"}) {
		t.Fatalf("names = %v, want [policy]", got)
	}
	if err := registry.Register(&testExtension{name: "policy"}); err == nil {
		t.Fatal("duplicate registration succeeded")
	}
	if got := registry.Names(); !reflect.DeepEqual(got, []string{"policy"}) {
		t.Fatalf("duplicate changed order: %v", got)
	}
}

func TestRegistryRunMergesHeadersAndStopsAtRefusal(t *testing.T) {
	registry := New()
	first := &testExtension{name: "first", decide: func(Call) Decision {
		return Decision{Header: map[string]string{"X-First": "yes", "X-Shared": "first"}}
	}}
	second := &testExtension{name: "second", decide: func(Call) Decision {
		return Decision{Refuse: true, Status: 403, Code: "denied", Header: map[string]string{"X-Shared": "second"}}
	}}
	thirdRan := false
	third := &testExtension{name: "third", decide: func(Call) Decision {
		thirdRan = true
		return Decision{}
	}}
	for _, ext := range []Extension{first, second, third} {
		if err := registry.Register(ext); err != nil {
			t.Fatalf("register %q: %v", ext.Name(), err)
		}
	}

	got := registry.Run(Call{Op: "chat", Model: "public"})
	if !got.Refuse || got.Status != 403 || got.Code != "denied" {
		t.Fatalf("decision = %#v", got)
	}
	if thirdRan {
		t.Fatal("extension after refusal ran")
	}
	wantHeaders := map[string]string{"X-First": "yes", "X-Shared": "second"}
	if !reflect.DeepEqual(got.Header, wantHeaders) {
		t.Fatalf("headers = %v, want %v", got.Header, wantHeaders)
	}
}
