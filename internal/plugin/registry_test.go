package plugin

import "testing"

type orderExt struct {
	name   string
	log    *[]string
	refuse bool
}

func (e orderExt) Name() string { return e.name }

func (e orderExt) BeforeUpstream(Call) Decision {
	*e.log = append(*e.log, e.name)
	return Decision{Refuse: e.refuse, Message: e.name}
}

func TestRegistryListsAndStopsOnRefusal(t *testing.T) {
	reg := New()
	var log []string
	if err := reg.Register(orderExt{name: "first", log: &log}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(orderExt{name: "second", log: &log, refuse: true}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(orderExt{name: "third", log: &log}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(orderExt{name: "second", log: &log}); err == nil {
		t.Fatal("duplicate name registered")
	}
	got := reg.Names()
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("names %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names %v", got)
		}
	}
	d := reg.Run(Call{Op: "chat", Model: "gpt-4o-mini", Path: "/v1/chat/completions"})
	if !d.Refuse || d.Message != "second" {
		t.Fatalf("decision %+v", d)
	}
	if len(log) != 2 || log[0] != "first" || log[1] != "second" {
		t.Fatalf("ran %v", log)
	}
	one, err := reg.Invoke("second", Call{Model: "gpt-4o-mini"})
	if err != nil || !one.Refuse {
		t.Fatalf("invoke %+v %v", one, err)
	}
	if _, err := reg.Invoke("missing", Call{}); err == nil {
		t.Fatal("missing name invoked")
	}
}
