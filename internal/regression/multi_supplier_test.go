package regression

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/providerconfig"
	"github.com/sunqirui1987/xhub/internal/router"
)

// supplierFixture observes the supplier at the HTTP boundary, independently of
// the upstream model string. Both suppliers intentionally serve the same model.
// sharedBase also exercises relays sharing an endpoint with separate credentials.
// Bases and credentials are replaced: no external host or key_env is accessed.
type supplierFixture struct {
	h        *harness
	entries  []config.ModelEntry
	mu       sync.Mutex
	calls    []supplierAttempt
	status   map[string]int
	sequence int
}
type supplierAttempt struct {
	Supplier, Model string
	Status          int
}

func supplierConfig(t *testing.T) providerconfig.Config {
	t.Helper()
	file, err := os.Open("testdata/config_provider.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cfg, err := providerconfig.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// newSupplierFixture installs YAML deployment rows and actual named credentials.
// Duplicate connections are allowed; each row must have its own stable ID.
// Credentials are seeded in the actual store, like seedCredential, since the
// provider setup API uses production host defaults. All calls use the real gateway.
func newSupplierFixture(t *testing.T, sharedBase bool, scenarios ...providerconfig.Scenario) *supplierFixture {
	t.Helper()
	cfg := supplierConfig(t)
	f := &supplierFixture{status: map[string]int{}}
	bases := map[string]string{}
	tokens := map[string]string{}
	for _, p := range cfg.Providers {
		tokens["Bearer sk-fake-"+p.ID] = p.ID
	}
	handler := func(expected string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			supplier := tokens[r.Header.Get("Authorization")]
			if supplier == "" || (expected != "" && supplier != expected) {
				http.Error(w, "incorrect supplier credential", http.StatusUnauthorized)
				return
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || r.URL.Path != "/v1/chat/completions" {
				http.Error(w, "unexpected supplier request", http.StatusBadRequest)
				return
			}
			model := stringField(body, "model")
			f.mu.Lock()
			status := f.status[supplier]
			f.sequence++
			responseID := fmt.Sprintf("chatcmpl-%s-%d", supplier, f.sequence)
			f.calls = append(f.calls, supplierAttempt{supplier, model, status})
			f.mu.Unlock()
			if status >= 400 {
				w.WriteHeader(status)
				writeJSON(w, map[string]any{"error": map[string]any{"message": "injected supplier failure", "type": "api_error"}})
				return
			}
			if stream, _ := body["stream"].(bool); stream {
				f.h.serveStream(w, model, false, nil)
				return
			}
			answer := chatAnswer(model, nil)
			answer["id"] = responseID
			answer["choices"] = []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": supplier}}}
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, answer)
		}
	}
	if sharedBase {
		server := httptest.NewServer(handler(""))
		t.Cleanup(server.Close)
		for _, p := range cfg.Providers {
			bases[p.ID] = server.URL + "/v1"
		}
	} else {
		for _, p := range cfg.Providers {
			server := httptest.NewServer(handler(p.ID))
			t.Cleanup(server.Close)
			bases[p.ID] = server.URL + "/v1"
		}
	}
	providers := map[string]providerconfig.Provider{}
	for _, p := range cfg.Providers {
		providers[p.ID] = p
	}
	for _, s := range scenarios {
		for _, d := range s.Deployments {
			p := providers[d.Provider]
			multiplier := 1.0
			if d.Provider == "SUPPLIER_B" {
				multiplier = 2
			}
			f.entries = append(f.entries, deployment(s.ModelName, d.Model, map[string]any{
				"deployment_id": d.ID, "api_base": bases[d.Provider], "litellm_credential_name": p.CredentialName,
				"input_cost_per_token": testInputRate * multiplier, "output_cost_per_token": testOutputRate * multiplier,
			}))
		}
	}
	f.h = newHarness(t, f.entries...)
	for _, p := range cfg.Providers {
		raw := mustJSON(map[string]any{"credential_name": p.CredentialName,
			"credential_info":   map[string]any{"custom_llm_provider": "openai"},
			"credential_values": map[string]any{"api_key": "sk-fake-" + p.ID, "api_base": bases[p.ID]},
		})
		if err := f.h.store.PutKV("credentials", p.CredentialName, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func (f *supplierFixture) attempts() []supplierAttempt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]supplierAttempt(nil), f.calls...)
}
func (f *supplierFixture) fail(supplier string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[supplier] = status
}
func (f *supplierFixture) reset() { f.mu.Lock(); defer f.mu.Unlock(); f.calls = nil }

// selected checks committed affinity against the supplier's response. Model
// strings cannot distinguish suppliers; supplier names alone cannot distinguish
// two deployment rows sharing a connection. Response IDs are unique per call.
func (f *supplierFixture) selected(t *testing.T, key, public string, r reply) config.ModelEntry {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	principal := f.h.gw.RequireLLMPrincipal(httptest.NewRecorder(), req)
	if principal == nil {
		t.Fatal("cannot resolve caller")
	}
	id := stringField(r.json(), "id")
	if id == "" {
		t.Fatal("successful response has no unique id")
	}
	plan := f.h.gw.PlanRoute(req, public, map[string]any{"previous_response_id": id}, principal)
	for _, entry := range f.entries {
		if entry.ModelName == public && router.CooldownID(entry) == plan.Pinned {
			choices := listField(r.json(), "choices")
			if len(choices) != 1 {
				t.Fatal("supplier response has no choice")
			}
			message, _ := choices[0]["message"].(map[string]any)
			want := "SUPPLIER_A"
			if entry.ParamString("litellm_credential_name", "") == "regression-supplier-b" {
				want = "SUPPLIER_B"
			}
			if stringField(message, "content") != want {
				t.Fatalf("affinity and supplier response disagree: %v", plan)
			}
			return entry
		}
	}
	t.Fatalf("no stable deployment attribution: %s", plan.Pinned)
	return config.ModelEntry{}
}

// checkBill independently calculates charges from the selected row's rates.
// Suppliers have different prices despite identical upstream model names.
// Check the durable snapshot, call ID, public alias and final token quantities.
func (f *supplierFixture) checkBill(t *testing.T, admin, public string, r reply, entry config.ModelEntry) float64 {
	t.Helper()
	cost := parseFloatOrZero(r.header("x-litellm-response-cost"))
	expected := 11*numberOrZero(entry.LiteLLMParams["input_cost_per_token"]) + 5*numberOrZero(entry.LiteLLMParams["output_cost_per_token"])
	if cost <= 0 || !nearlyEqual(cost, expected) {
		t.Fatalf("supplier price: got %g want %g", cost, expected)
	}
	row := logDetail(t, f.h, admin, r.header("x-litellm-call-id"))
	if row["model"] != public || !nearlyEqual(numberOrZero(row["spend"]), cost) || numberOrZero(row["prompt_tokens"]) != 11 || numberOrZero(row["completion_tokens"]) != 5 {
		t.Fatalf("supplier bill/usage differs from response: %v", row)
	}
	bill := breakdownOf(t, f.h, admin, r.header("x-litellm-call-id"))
	if bill["source"] != "snapshot" {
		t.Fatal("missing durable rate snapshot")
	}
	sum := 0.0
	for _, rate := range listField(bill, "applied") {
		sum += numberOrZero(rate["quantity"]) * numberOrZero(rate["usd"])
	}
	if !nearlyEqual(sum, expected) {
		t.Fatalf("snapshot charge %g want %g", sum, expected)
	}
	return cost
}

// runCounts observes actual attempts, committed row IDs and five spend counters.
// Each prompt is unique to avoid response-cache and prompt-affinity effects.
func (f *supplierFixture) runCounts(t *testing.T, admin string, c chained, public, label string, requests int, want map[string]int) {
	t.Helper()
	before := f.h.moneyOf(t, c)
	mark := len(f.attempts())
	counts := map[string]int{}
	total := 0.0
	for i := 0; i < requests; i++ {
		r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, fmt.Sprintf("%s request %d", label, i)))
		entry := f.selected(t, c.key, public, r)
		counts[entry.ParamString("deployment_id", "")]++
		total += f.checkBill(t, admin, public, r, entry)
	}
	for id, expected := range want {
		if counts[id] != expected {
			t.Errorf("%s deployment %s: got %d want %d; counts=%v", label, id, counts[id], expected, counts)
		}
		delete(counts, id)
	}
	if len(counts) != 0 {
		t.Errorf("unexpected deployments: %v", counts)
	}
	calls := f.attempts()[mark:]
	if len(calls) != requests {
		t.Fatalf("got %d supplier attempts for %d healthy requests", len(calls), requests)
	}
	for _, call := range calls {
		if call.Model != public {
			t.Fatalf("supplier model %s want identical %s", call.Model, public)
		}
	}
	if !f.h.moneyOf(t, c).grewBy(before, total) {
		t.Fatal("five-layer spend differs from supplier charges")
	}
}

func TestMultiSupplierConfiguredWeights(t *testing.T) {
	cfg := supplierConfig(t)
	for _, shared := range []bool{false, true} {
		for _, scenario := range cfg.WeightedScenarios {
			t.Run(fmt.Sprintf("shared-base-%t/%s", shared, scenario.Name), func(t *testing.T) {
				f := newSupplierFixture(t, shared, scenario)
				admin := f.h.adminSession()
				c := f.h.openScope(t, admin, "suppliers")
				weights := []any{}
				for _, dep := range scenario.Deployments {
					weights = append(weights, map[string]any{"deployment_id": dep.ID, "weight": dep.Weight})
				}
				id := routeTemplate(t, f.h, admin, "configured-weights", map[string]any{"routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": weights}, "allowed_fails": 0})
				bindTemplate(t, f.h, admin, "team", c.teamID, id)
				want := expectedWeightedCounts(scenario)
				f.runCounts(t, admin, c, scenario.ModelName, scenario.Name, scenario.Requests, want)
				if len(f.h.successRows(t, admin, scenario.ModelName)) != scenario.Requests {
					t.Fatal("successful bill count differs from client calls")
				}
			})
		}
	}
}

func TestMultiSupplierTemplateWeights(t *testing.T) {
	for _, form := range []string{"list", "map", "partial", "default-one"} {
		t.Run(form, func(t *testing.T) {
			s := supplierConfig(t).WeightedScenarios[0]
			if form == "default-one" {
				s.Deployments[1].Weight = 1
			}
			f := newSupplierFixture(t, true, s)
			if form == "default-one" {
				// 未列出的部署统一使用模板默认权重 1。
			}
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "template-suppliers")
			var weights any = []any{map[string]any{"deployment_id": "sol-a", "weight": 3}, map[string]any{"deployment_id": "sol-b", "weight": 7}}
			requests := 10
			want := map[string]int{"sol-a": 3, "sol-b": 7}
			if form == "map" {
				weights = map[string]any{"deployment:sol-a": 3, "deployment:sol-b": 7}
			}
			if form == "partial" || form == "default-one" {
				weights = []any{map[string]any{"deployment_id": "sol-a", "weight": 1}}
				requests = 8
				want = map[string]int{"sol-a": 4, "sol-b": 4}
				if form == "default-one" {
					want = map[string]int{"sol-a": 4, "sol-b": 4}
				}
			}
			id := routeTemplate(t, f.h, admin, form, map[string]any{"routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": weights}, "allowed_fails": 0})
			bindTemplate(t, f.h, admin, "team", c.teamID, id)
			f.runCounts(t, admin, c, s.ModelName, form, requests, want)
			if _, exists := f.entries[0].LiteLLMParams["weight"]; exists {
				t.Fatal("部署不能保存路由权重")
			}
		})
	}
}

// One team template covers two public aliases. Rules only affect their alias;
// other models use the document default. Key overrides select a whole document.
func TestMultiSupplierModelRulesAndScopeIsolation(t *testing.T) {
	sol := supplierConfig(t).WeightedScenarios[0]
	terra := sol
	terra.ModelName = "gpt-5.6-terra"
	terra.Deployments = append([]providerconfig.Deployment(nil), sol.Deployments...)
	for i := range terra.Deployments {
		terra.Deployments[i].ID = fmt.Sprintf("terra-%d", i)
		terra.Deployments[i].Model = terra.ModelName
	}
	f := newSupplierFixture(t, true, sol, terra)
	admin := f.h.adminSession()
	a := f.h.openScope(t, admin, "team-a")
	b := f.h.openScope(t, admin, "team-b")
	body := func(wa, wb int) map[string]any {
		return map[string]any{"routing_strategy": "simple-shuffle", "allowed_fails": 0, "model_routing": []any{map[string]any{
			"model_name": sol.ModelName, "routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": map[string]any{"deployment:sol-a": wa, "deployment:sol-b": wb}},
		}}}
	}
	team := routeTemplate(t, f.h, admin, "team rules", body(3, 7))
	bindTemplate(t, f.h, admin, "team", a.teamID, team)
	f.runCounts(t, admin, a, sol.ModelName, "team split", 10, map[string]int{"sol-a": 3, "sol-b": 7})
	f.runCounts(t, admin, a, terra.ModelName, "other model default", 4, map[string]int{"terra-0": 4, "terra-1": 0})
	f.runCounts(t, admin, b, sol.ModelName, "unbound tenant", 4, map[string]int{"sol-a": 4, "sol-b": 0})
	key := routeTemplate(t, f.h, admin, "key rules", body(0, 1))
	bindTemplate(t, f.h, admin, "key", a.keyID, key)
	f.runCounts(t, admin, a, sol.ModelName, "key overrides team", 4, map[string]int{"sol-a": 0, "sol-b": 4})
	if e := effectiveTemplate(t, f.h, admin, "key", a.keyID); e["scope_type"] != "key" {
		t.Fatal("wrong effective scope")
	}
	bindTemplate(t, f.h, admin, "key", a.keyID, "")
	f.h.ok(http.MethodPost, "/route_template/"+team+"/update", admin, map[string]any{"body": body(8, 2)})
	f.runCounts(t, admin, a, sol.ModelName, "edited team", 10, map[string]int{"sol-a": 8, "sol-b": 2})
	f.runCounts(t, admin, b, sol.ModelName, "edit tenant isolation", 4, map[string]int{"sol-a": 4, "sol-b": 0})
	bindTemplate(t, f.h, admin, "team", a.teamID, "")
	f.runCounts(t, admin, a, sol.ModelName, "clear to platform", 4, map[string]int{"sol-a": 4, "sol-b": 0})
}

func TestMultiSupplierRetryAndBilling(t *testing.T) {
	for _, status := range []int{500, 429, 400} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := supplierConfig(t).WeightedScenarios[0]
			f := newSupplierFixture(t, true, s)
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "retry-suppliers")
			id := routeTemplate(t, f.h, admin, "retry", map[string]any{"routing_strategy": "weighted-split", "num_retries": 2, "allowed_fails": 0})
			bindTemplate(t, f.h, admin, "team", c.teamID, id)
			f.fail("SUPPLIER_A", status)
			before := f.h.moneyOf(t, c)
			r := f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "fault injection"))
			want := []supplierAttempt{{"SUPPLIER_A", s.ModelName, status}, {"SUPPLIER_A", s.ModelName, status}, {"SUPPLIER_B", s.ModelName, 0}}
			if status == 400 {
				want = want[:1]
				if r.status != 400 || !f.h.moneyOf(t, c).same(before) || len(f.h.successRows(t, admin, s.ModelName)) != 0 {
					t.Fatalf("terminal 400 billed or retried: %s", r.describe())
				}
			} else {
				if r.status != 200 {
					t.Fatalf("failover: %s", r.describe())
				}
				entry := f.selected(t, c.key, s.ModelName, r)
				cost := f.checkBill(t, admin, s.ModelName, r, entry)
				if !f.h.moneyOf(t, c).grewBy(before, cost) || len(f.h.successRows(t, admin, s.ModelName)) != 1 {
					t.Fatal("failover charged multiple attempts")
				}
			}
			if !reflect.DeepEqual(f.attempts(), want) {
				t.Fatalf("supplier retry order %v want %v", f.attempts(), want)
			}
			f.reset()
			f.fail("SUPPLIER_A", 500)
			f.fail("SUPPLIER_B", 500)
			before = f.h.moneyOf(t, c)
			r = f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "all suppliers fail"))
			if r.status != 502 || len(f.attempts()) != 4 || !f.h.moneyOf(t, c).same(before) {
				t.Fatalf("all failed: %s attempts=%v", r.describe(), f.attempts())
			}
			row := logDetail(t, f.h, admin, r.header("x-litellm-call-id"))
			if row["status"] != "error" || numberOrZero(row["spend"]) != 0 {
				t.Fatalf("failure log charged: %v", row)
			}
		})
	}
}

func TestMultiSupplierZeroWeightExcludesFailover(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[3]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "zero-supplier")
	f.h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"routing_strategy": "weighted-split", "num_retries": 1, "allowed_fails": 0}})
	f.fail("SUPPLIER_A", 500)
	r := f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "zero cannot rescue failed supplier"))
	if r.status != 502 || !reflect.DeepEqual(f.attempts(), []supplierAttempt{{"SUPPLIER_A", s.ModelName, 500}}) || !f.h.moneyOf(t, c).same(scopeMoney{}) {
		t.Fatalf("zero-weight supplier received retry or charge: %s %v", r.describe(), f.attempts())
	}
}

func TestMultiSupplierAllZeroRejects(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "all-zero")
	id := routeTemplate(t, f.h, admin, "no traffic", map[string]any{"routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": map[string]any{"deployment:sol-a": 0, "deployment:sol-b": 0}}})
	bindTemplate(t, f.h, admin, "team", c.teamID, id)
	r := f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "no eligible deployment"))
	if r.status < 400 || len(f.attempts()) != 0 || !f.h.moneyOf(t, c).same(scopeMoney{}) {
		t.Fatalf("all-zero weights dialed a supplier: %s %v", r.describe(), f.attempts())
	}
}

func TestMultiSupplierResponseAffinity(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "affinity-suppliers")
	f.h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"routing_strategy": "weighted-split", "allowed_fails": 0}})
	r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "first unique turn"))
	first := f.selected(t, c.key, s.ModelName, r)
	// Other traffic advances the scheduler; the continuation must remain pinned.
	f.runCounts(t, admin, c, s.ModelName, "advance scheduler", 10, map[string]int{"sol-a": 7, "sol-b": 3})
	body := chatRequest(s.ModelName, "continuation with different prompt")
	body["previous_response_id"] = stringField(r.json(), "id")
	next := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, body)
	if router.CooldownID(f.selected(t, c.key, s.ModelName, next)) != router.CooldownID(first) {
		t.Fatal("continuation changed identical-model supplier")
	}
}

func TestMultiSupplierStreamingAndConcurrentWeights(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream-%t", stream), func(t *testing.T) {
			s := supplierConfig(t).WeightedScenarios[0]
			f := newSupplierFixture(t, true, s)
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "parallel-suppliers")
			f.h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"routing_strategy": "weighted-split", "allowed_fails": 0}})
			before := f.h.moneyOf(t, c)
			type result struct {
				r   reply
				err error
			}
			results := make(chan result, 20)
			limit := make(chan struct{}, 5)
			for i := 0; i < 20; i++ {
				go func(index int) {
					limit <- struct{}{}
					defer func() { <-limit }()
					body := chatRequest(s.ModelName, fmt.Sprintf("parallel unique %t %d", stream, index))
					body["stream"] = stream
					req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.h.server.URL+"/v1/chat/completions", bytes.NewReader(mustJSON(body)))
					if err != nil {
						results <- result{err: err}
						return
					}
					req.Header.Set("Authorization", "Bearer "+c.key)
					req.Header.Set("Content-Type", "application/json")
					client := *f.h.server.Client()
					client.Timeout = 30 * time.Second
					res, err := client.Do(req)
					if err != nil {
						results <- result{err: err}
						return
					}
					raw, err := io.ReadAll(res.Body)
					res.Body.Close()
					results <- result{reply{status: res.StatusCode, body: raw, headers: res.Header.Clone()}, err}
				}(i)
			}
			total := 0.0
			counts := map[string]int{}
			for i := 0; i < 20; i++ {
				out := <-results
				if out.err != nil {
					t.Error(out.err)
					continue
				}
				if out.r.status != 200 {
					t.Errorf("parallel request: %s", out.r.describe())
					continue
				}
				if stream {
					if !strings.Contains(out.r.text(), "data: [DONE]") || !strings.Contains(out.r.text(), "regression-ok") {
						t.Error("stream is missing content or completion")
					}
					row := logDetail(t, f.h, admin, out.r.header("x-litellm-call-id"))
					total += numberOrZero(row["spend"])
					if numberOrZero(row["prompt_tokens"]) != 11 || numberOrZero(row["completion_tokens"]) != 5 {
						t.Error("stream final usage lost")
					}
				} else {
					entry := f.selected(t, c.key, s.ModelName, out.r)
					counts[entry.ParamString("deployment_id", "")]++
					total += f.checkBill(t, admin, s.ModelName, out.r, entry)
				}
			}
			suppliers := map[string]int{}
			for _, call := range f.attempts() {
				suppliers[call.Supplier]++
				if call.Model != s.ModelName {
					t.Error("parallel model changed")
				}
			}
			if suppliers["SUPPLIER_A"] != 14 || suppliers["SUPPLIER_B"] != 6 || len(f.attempts()) != 20 {
				t.Fatalf("parallel split: %v", suppliers)
			}
			if !stream && (counts["sol-a"] != 14 || counts["sol-b"] != 6) {
				t.Fatalf("parallel deployment attribution: %v", counts)
			}
			baseCost := 11*testInputRate + 5*testOutputRate
			if !nearlyEqual(total, 26*baseCost) {
				t.Fatalf("parallel/stream charge: got=%g want=%g", total, 26*baseCost)
			}
			after := f.h.moneyOf(t, c)
			if !after.grewBy(before, total) {
				t.Fatalf("parallel/stream scopes: before=%+v after=%+v charge=%g", before, after, total)
			}
			if rows := f.h.successRows(t, admin, s.ModelName); len(rows) != 20 {
				t.Fatalf("parallel/stream successful logs: got=%d want=20", len(rows))
			}
		})
	}
}

// TestMultiSupplierDatabaseDuplicateDeployments uses the same model/new path as
// the console. The two rows intentionally share endpoint, credential and model;
// only model_info.id, weights and manual prices differ. Config-only IDs would
// miss identity collisions in actual database-backed deployments.
func TestMultiSupplierDatabaseDuplicateDeployments(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "database-duplicates")
	const public = "database-gpt-5.6-sol"
	connection := f.entries[0]
	for i, id := range []string{"db-sol-a", "db-sol-b"} {
		multiplier := 1.0
		if i == 1 {
			multiplier = 2
		}
		f.h.addDBModel(t, admin, public, s.ModelName, id, map[string]any{
			"api_base":                connection.ParamString("api_base", ""),
			"litellm_credential_name": connection.ParamString("litellm_credential_name", ""),
			"input_cost_per_token":    testInputRate * multiplier, "output_cost_per_token": testOutputRate * multiplier,
		})
	}
	f.entries = nil
	for _, entry := range f.h.gw.Models() {
		if entry.ModelName == public {
			if entry.ParamString("deployment_id", "") != "" {
				t.Fatal("test must exercise database IDs without injected config IDs")
			}
			f.entries = append(f.entries, entry)
		}
	}
	if len(f.entries) != 2 || router.CooldownID(f.entries[0]) == router.CooldownID(f.entries[1]) {
		t.Fatal("database duplicate rows collapsed into one runtime deployment")
	}
	id := routeTemplate(t, f.h, admin, "database-weights", map[string]any{"routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": []any{map[string]any{"deployment_id": "db-sol-a", "weight": 7}, map[string]any{"deployment_id": "db-sol-b", "weight": 3}}}, "allowed_fails": 0})
	bindTemplate(t, f.h, admin, "team", c.teamID, id)
	before := f.h.moneyOf(t, c)
	counts := map[string]int{}
	total := 0.0
	var second reply
	for i := 0; i < 20; i++ {
		r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, fmt.Sprintf("database duplicate %d", i)))
		entry := f.selected(t, c.key, public, r)
		id := stringField(entry.ModelInfo, "id")
		counts[id]++
		total += f.checkBill(t, admin, public, r, entry)
		if id == "db-sol-b" {
			second = r
		}
	}
	if counts["db-sol-a"] != 14 || counts["db-sol-b"] != 6 || len(f.attempts()) != 20 {
		t.Fatalf("database duplicate split=%v attempts=%d", counts, len(f.attempts()))
	}
	if !f.h.moneyOf(t, c).grewBy(before, total) || len(f.h.successRows(t, admin, public)) != 20 {
		t.Fatal("database duplicate billing scopes or logs differ")
	}
	body := chatRequest(public, "continue specifically on second database row")
	body["previous_response_id"] = stringField(second.json(), "id")
	pinned := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, body)
	entry := f.selected(t, c.key, public, pinned)
	if stringField(entry.ModelInfo, "id") != "db-sol-b" {
		t.Fatal("second database row affinity resolved to the first physical duplicate")
	}
	f.checkBill(t, admin, public, pinned, entry)
	f.h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"id": "db-sol-a"})
	for i := 0; i < 4; i++ {
		r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, fmt.Sprintf("only enabled duplicate %d", i)))
		entry := f.selected(t, c.key, public, r)
		if stringField(entry.ModelInfo, "id") != "db-sol-b" {
			t.Fatal("disabled database duplicate selected")
		}
		f.checkBill(t, admin, public, r, entry)
	}
	f.h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"id": "db-sol-b"})
	mark := len(f.attempts())
	before = f.h.moneyOf(t, c)
	rejected := f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "all database duplicates disabled"))
	if rejected.status != http.StatusBadRequest || !strings.Contains(errorMessage(rejected), "disabled") || len(f.attempts()) != mark || !f.h.moneyOf(t, c).same(before) {
		t.Fatal("all disabled duplicates must reject without upstream calls or charges")
	}
}

func TestMultiSupplierCooldownIsolation(t *testing.T) {
	redisURL := os.Getenv("XHUB_REGRESSION_REDIS_URL")
	if redisURL == "" {
		t.Skip("cooldown isolation needs XHUB_REGRESSION_REDIS_URL")
	}
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "cooldown-suppliers")
	id := routeTemplate(t, f.h, admin, "cool one supplier", map[string]any{"routing_strategy": "weighted-split", "allowed_fails": 1, "cooldown_time": 120, "num_retries": 1})
	bindTemplate(t, f.h, admin, "team", c.teamID, id)
	f.fail("SUPPLIER_A", 500)
	r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "trip supplier A"))
	f.checkBill(t, admin, s.ModelName, r, f.selected(t, c.key, s.ModelName, r))
	if !reflect.DeepEqual(f.attempts(), []supplierAttempt{{"SUPPLIER_A", s.ModelName, 500}, {"SUPPLIER_B", s.ModelName, 0}}) {
		t.Fatalf("trip order %v", f.attempts())
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opt)
	keyA := "xhub:cooldown:" + router.CooldownID(f.entries[0])
	keyB := "xhub:cooldown:" + router.CooldownID(f.entries[1])
	defer func() { client.Del(t.Context(), keyA, keyB); client.Close() }()
	ttl, err := client.PTTL(t.Context(), keyA).Result()
	if err != nil || ttl <= 0 || ttl > 120*time.Second {
		t.Fatalf("supplier A cooldown TTL=%v err=%v", ttl, err)
	}
	if n, err := client.Exists(t.Context(), keyB).Result(); err != nil || n != 0 {
		t.Fatalf("supplier A cooled supplier B: %d %v", n, err)
	}
	f.fail("SUPPLIER_A", 0)
	f.runCounts(t, admin, c, s.ModelName, "healthy but cooling", 5, map[string]int{"sol-a": 0, "sol-b": 5})
	if err := client.Del(t.Context(), keyA).Err(); err != nil {
		t.Fatal(err)
	}
	f.runCounts(t, admin, c, s.ModelName, "recovered split", 10, map[string]int{"sol-a": 7, "sol-b": 3})
}
