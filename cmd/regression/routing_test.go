package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestRoutingStrategyChain 每一种策略都走完整链路：写入策略，调用成功，上游模型是策略选出的那条，日志金额和响应头一致。
func TestRoutingStrategyChain(t *testing.T) { runSimulated(t, routingStrategySimulated) }

func routingStrategySimulated(t *testing.T) {
	cases := []struct {
		name, strategy, want string
		a, b                 map[string]any
		allocations          map[string]float64
	}{
		{
			name: "weight", strategy: "traffic-split", want: "heavy",
			a: map[string]any{}, b: map[string]any{},
			allocations: map[string]float64{"heavy": 1, "light": 0},
		},
		{
			name: "cost", strategy: "cost-based-routing", want: "cheap",
			a: map[string]any{"input_cost_per_token": 0.01}, b: map[string]any{"input_cost_per_token": 0.0000001},
		},
		{
			name: "latency", strategy: "latency-based-routing", want: "quick",
			a: map[string]any{"latency_ms": 80}, b: map[string]any{"latency_ms": 5},
		},
		{
			name: "usage", strategy: "usage-based-routing", want: "idle",
			a: map[string]any{"tpm": 100}, b: map[string]any{"tpm": 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			public := "routed-" + tc.name
			first, second := tc.want, "other-"+tc.name
			if tc.name == "weight" {
				first, second = "heavy", "light"
			}
			if tc.name == "cost" {
				first, second = "dear", "cheap"
			}
			if tc.name == "latency" {
				first, second = "slow", "quick"
			}
			if tc.name == "usage" {
				first, second = "busy", "idle"
			}
			h := newHarness(t,
				deployment(public, "openai/"+first, tc.a),
				deployment(public, "openai/"+second, tc.b),
			)
			admin := h.adminSession()
			c := h.openScope(t, admin, "route-"+tc.name)
			var allocations []any
			if tc.allocations != nil {
				allocations = routeAllocations(tc.allocations)
			}
			h.modelRouteTemplate(t, admin, c, "route "+tc.name, public, tc.strategy, allocations, 1, 0, 0)
			h.assertBilled(t, c, admin, public, "strategy "+tc.strategy, []string{tc.want})
		})
	}
}

// TestLeastBusyChain 第一条请求占住部署时，并发的下一条打到空闲的那条。
func TestLeastBusyChain(t *testing.T) { runSimulated(t, leastBusySimulated) }

func leastBusySimulated(t *testing.T) {
	const public = "routed-busy"
	h := newHarness(t,
		deployment(public, "openai/held", nil),
		deployment(public, "openai/free", nil),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "least-busy")
	h.modelRouteTemplate(t, admin, c, "least busy route", public, "least-busy", nil, 1, 0, 0)
	release := h.holdUpstream("held")
	defer release()
	done := make(chan reply, 1)
	go func() {
		done <- h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "holds the first deployment"))
	}()
	h.waitUpstream(t, "held")
	mark := len(h.upstreamCalls())
	second := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "takes the idle deployment"))
	if got := h.upstreamSince(mark); len(got) != 1 || got[0] != "free" {
		t.Fatalf("the second call reached %v, want only free", got)
	}
	if parseFloatOrZero(second.header("x-litellm-response-cost")) <= 0 {
		t.Fatalf("the idle deployment was not billed: %s", second.describe())
	}
	release()
	first := <-done
	if first.status != http.StatusOK {
		t.Fatalf("the held call answered %d: %s", first.status, first.text())
	}
}

// TestUnknownStrategyChain 不认识的策略是 400，不会被悄悄当成 simple-shuffle，也不会拨上游。
func TestUnknownStrategyChain(t *testing.T) { runSimulated(t, unknownStrategySimulated) }

func unknownStrategySimulated(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-bad-strategy"))
	admin := h.adminSession()
	c := h.openScope(t, admin, "bad-strategy")
	before := h.moneyOf(t, c)
	body := routeTemplateBody([]any{map[string]any{"model": "regression-bad-strategy", "strategy": "not-a-strategy"}}, 1, 60, 0, 0)
	r := h.do(http.MethodPost, "/route_template/new", admin, map[string]any{"name": "invalid strategy", "body": body})
	if r.status != http.StatusBadRequest {
		t.Fatalf("unknown strategy answered %d, want 400: %s", r.status, r.describe())
	}
	if message := errorMessage(r); !strings.Contains(message, "unknown routing strategy") {
		t.Fatalf("the refusal said %q", message)
	}
	if got := len(h.upstreamCalls()); got != 0 {
		t.Fatalf("unknown strategy still dialed the upstream %d times", got)
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("unknown strategy moved spend from %+v to %+v", before, after)
	}
}

// TestExactNameBeatsWildcard 精确名优先。通配只改写没有精确部署的那个名字。
func TestExactNameBeatsWildcard(t *testing.T) { runSimulated(t, exactNameSimulated) }

func exactNameSimulated(t *testing.T) {
	h := newHarness(t,
		deployment("openai/*", "openai/*", nil),
		deployment("openai/exact-model", "openai/exact-upstream", nil),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "wildcard")
	h.assertBilled(t, c, admin, "openai/exact-model", "exact wins", []string{"exact-upstream"})
	// 通配改写之后的模型 id 不在价目上，这次只核对上游被改写成了调用方写的名字。
	mark := len(h.upstreamCalls())
	rewritten := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest("openai/wild-1", "wildcard rewrites"))
	if got := h.upstreamSince(mark); len(got) != 1 || got[0] != "wild-1" {
		t.Fatalf("wildcard upstream %v, want wild-1; body %s", got, rewritten.text())
	}
}

// TestSessionPinOverridesStrategy 会话钉压过后来改掉的策略：同一会话仍打第一台，换一个会话才按新策略走。
func TestSessionPinOverridesStrategy(t *testing.T) { runSimulated(t, sessionPinSimulated) }

func sessionPinSimulated(t *testing.T) {
	const public = "routed-pin"
	h := newHarness(t,
		deployment(public, "openai/pin-a", map[string]any{"weight": 10, "input_cost_per_token": 0.01}),
		deployment(public, "openai/pin-b", map[string]any{"weight": 1, "input_cost_per_token": 0.0000001}),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "pin")
	// 先用确定的流量分配建立会话归属，再切换到最低成本策略。这样验证的是会话钉住
	// 覆盖后来生效的策略，不依赖 simple-shuffle 的随机首选结果。
	h.modelRouteTemplate(t, admin, c, "pinned initial route", public, "traffic-split", routeAllocations(map[string]float64{"pin-a": 1, "pin-b": 0}), 1, 0, 0)
	pinned := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "pinned turn"), map[string]string{"X-Session-Id": "session-one"})
	if pinned.status != http.StatusOK {
		t.Fatalf("first pinned turn: %s", pinned.describe())
	}
	if got := h.upstreamSince(0); len(got) != 1 || got[0] != "pin-a" {
		t.Fatalf("the initial allocation did not select pin-a: %v", got)
	}
	h.modelRouteTemplate(t, admin, c, "pinned cost route", public, "cost-based-routing", nil, 1, 0, 0)
	h.resetUpstream()
	again := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "same session, cheaper exists"), map[string]string{"X-Session-Id": "session-one"})
	if again.status != http.StatusOK {
		t.Fatalf("pinned retry: %s", again.describe())
	}
	if got := h.upstreamSince(0); len(got) != 1 || got[0] != "pin-a" {
		t.Fatalf("the session left its deployment: %v", got)
	}
	h.resetUpstream()
	other := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "a different session"), map[string]string{"X-Session-Id": "session-two"})
	if other.status != http.StatusOK {
		t.Fatalf("second session: %s", other.describe())
	}
	if got := h.upstreamSince(0); len(got) != 1 || got[0] != "pin-b" {
		t.Fatalf("a new session ignored lowest-cost: %v", got)
	}
	row := h.successRows(t, admin, public)
	if len(row) < 3 {
		t.Fatalf("pinned calls were not all logged: %d", len(row))
	}
}
