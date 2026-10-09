package router

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"strings"
	"testing"
)

// TestGroupContract 验证严格字段、名称边界、成员占用和部署归属；纯内存目录，无外部数据清理。
func TestGroupContract(t *testing.T) {
	list := []config.ModelEntry{deployment("a", "a1", 0), deployment("b", "b1", 0)}
	base := Group{Name: "chat-group", Models: []string{"a", "b"}, Strategy: "traffic-split", Args: &GroupArgs{Allocations: []Allocation{{"a1", 3}, {"b1", 7}}}}
	if err := ValidateGroups([]Group{base}, list); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseGroup(map[string]any{"group_name": "group", "models": []string{"a"}, "routing_strategy": "random", "ttl": 3}); err == nil {
		t.Fatal("未知字段没有拒绝")
	}
	for _, name := range []string{"", "default", "white space", "unicode\u2003space", "wild*", strings.Repeat("字", 65)} {
		bad := base
		bad.Name = name
		if bad.Validate() == nil {
			t.Fatalf("非法名称被接受: %q", name)
		}
	}
	boundary := base
	boundary.Name = strings.Repeat("字", 64)
	if boundary.Validate() != nil {
		t.Fatal("64字符边界被拒绝")
	}
	cases := [][]Group{{{Name: "a", Models: []string{"a"}, Strategy: "random"}}, {{Name: "g", Models: []string{"missing"}, Strategy: "random"}}, {{Name: "g", Models: []string{"a", "a"}, Strategy: "random"}}, {base, {Name: "other", Models: []string{"a"}, Strategy: "random"}}, {{Name: "g", Models: []string{"a"}, Strategy: "traffic-split", Args: &GroupArgs{Allocations: []Allocation{{"b1", 1}}}}}, {{Name: "g", Models: []string{"a"}, Strategy: "random", Args: &GroupArgs{}}}, {{Name: "g", Models: []string{"a"}, Strategy: "traffic-split", Args: &GroupArgs{Allocations: []Allocation{{"a1", 0}}}}}}
	for i, groups := range cases {
		if ValidateGroups(groups, list) == nil {
			t.Fatalf("非法组配置 %d 被接受", i)
		}
	}
}

// TestGroupScheduleIdentity 验证组展开保留真实成员身份、零权重、粘性及普通成员范围；无外部状态和清理。
func TestGroupScheduleIdentity(t *testing.T) {
	a, b, c := deployment("a", "a1", 0), deployment("b", "b1", 0), deployment("other", "c1", 0)
	list := []config.ModelEntry{a, b, c}
	state := State{ModelNames: []string{"a", "b"}, Allocations: map[string]float64{"a1": 0, "b1": 7}, Draw: func() float64 { return 0 }}
	got := Schedule(list, "chat-group", "traffic-split", state, CooldownID(a))
	if len(got) != 1 || got[0].ModelName != "b" {
		t.Fatalf("组身份或零权重失效: %v", got)
	}
	state.Allocations["a1"] = 3
	got = Schedule(list, "chat-group", "traffic-split", state, CooldownID(b))
	if len(got) != 2 || got[0].ModelName != "b" {
		t.Fatalf("组粘性失效: %v", got)
	}
	state.Cooldown = map[string]bool{CooldownID(b): true}
	got = Schedule(list, "chat-group", "traffic-split", state, "")
	if len(got) != 1 || got[0].ModelName != "a" {
		t.Fatalf("组冷却失效: %v", got)
	}
	got = Schedule(list, "a", "random", State{}, "")
	if len(got) != 1 || got[0].ModelName != "a" {
		t.Fatalf("成员调用跨模型: %v", got)
	}
	b.ModelInfo = map[string]any{"disabled": true}
	if len(GroupCandidates([]config.ModelEntry{a, b, c}, []string{"a", "b"})) != 1 {
		t.Fatal("暂停部署进入组候选")
	}
}
