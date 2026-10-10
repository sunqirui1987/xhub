package regression

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sunqirui1987/xhub/cmd/regression/providerconfig"
	"github.com/sunqirui1987/xhub/internal/config"
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
// 合成凭据写入隔离存储，避免供应商设置接口的生产地址默认值。
// 参数 t 为测试上下文，sharedBase 控制是否共用地址，scenarios 为部署场景；返回夹具，harness 清理存储与上游。
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

// assertAllocationDraws 用可注入抽样值逐项验证每个正权重区间都归属正确部署。
// 参数 public 是公开模型名，weights 是稳定部署 ID 到相对权重的映射；返回值为空。
// 本函数由权重回归在真实请求前调用，不访问供应商、不产生账单，并确认零权重部署不会进入候选池。
func (f *supplierFixture) assertAllocationDraws(t *testing.T, public string, weights map[string]float64) {
	t.Helper()
	total := 0.0
	for _, weight := range weights {
		if weight > 0 {
			total += weight
		}
	}
	if total <= 0 {
		t.Fatal("确定性 allocations 抽样要求至少一个正权重部署")
	}
	for target, weight := range weights {
		if weight <= 0 {
			continue
		}
		picked := router.Pick(f.entries, public, "traffic-split", router.State{
			Allocations: weights,
			Draw: func() float64 {
				before := 0.0
				for _, entry := range f.entries {
					id := router.DeploymentID(entry)
					if weights[id] <= 0 {
						continue
					}
					if id == target {
						return (before + weights[id]/2) / total
					}
					before += weights[id]
				}
				return 0
			},
		})
		if picked == nil || router.DeploymentID(*picked) != target {
			t.Fatalf("确定性 allocations 抽样选中=%v，期望部署=%s", picked, target)
		}
	}
}

// statisticalSampleSize 为真实随机分流选择样本数。
// 参数 requested 是场景原始请求数，weights 是相对权重；返回值至少为 requested。
// 两个及以上正权重部署使用至少一百次请求形成代表性样本；排他场景保留原请求量，避免无意义放大副作用。
func statisticalSampleSize(requested int, weights map[string]float64) int {
	positive := 0
	for _, weight := range weights {
		if weight > 0 {
			positive++
		}
	}
	if positive > 1 && requested < 100 {
		return 100
	}
	return requested
}

// assertWeightedDistribution 验证真实随机分流的排他性和代表性。
// 参数 label 标识场景，counts 是观测计数，weights 是相对权重，requests 是成功请求总数；返回值为空。
// 零权重必须严格为零；单一正权重必须独占；多部署按期望值的五个标准差且至少三次的容差校验，以覆盖多场景重复运行的随机尾部并避免误判成精确周期。
func assertWeightedDistribution(t *testing.T, label string, counts map[string]int, weights map[string]float64, requests int) {
	t.Helper()
	totalWeight := 0.0
	positive := 0
	for _, weight := range weights {
		if weight > 0 {
			totalWeight += weight
			positive++
		}
	}
	counted := 0
	for id, count := range counts {
		counted += count
		if weight, exists := weights[id]; !exists || weight <= 0 {
			t.Errorf("%s 选中了无资格或零权重部署 %s：counts=%v weights=%v", label, id, counts, weights)
		}
	}
	if counted != requests {
		t.Errorf("%s 只归属了 %d/%d 次成功请求：%v", label, counted, requests, counts)
	}
	for id, weight := range weights {
		if weight <= 0 {
			if counts[id] != 0 {
				t.Errorf("%s 零权重部署 %s 收到 %d 次请求", label, id, counts[id])
			}
			continue
		}
		expected := float64(requests) * weight / totalWeight
		if positive == 1 {
			if counts[id] != requests {
				t.Errorf("%s 唯一正权重部署 %s 收到 %d/%d 次请求", label, id, counts[id], requests)
			}
			continue
		}
		p := weight / totalWeight
		tolerance := math.Max(3, 5*math.Sqrt(float64(requests)*p*(1-p)))
		if math.Abs(float64(counts[id])-expected) > tolerance {
			t.Errorf("%s 部署 %s 的随机分布超出容差：got=%d expected=%.1f tolerance=%.1f counts=%v", label, id, counts[id], expected, tolerance, counts)
		}
	}
}

// scenarioWeights 提取供应商场景的部署权重。
// 参数 scenario 来自严格校验后的本地 YAML；返回稳定部署 ID 到权重的新映射，无副作用。
// 调用方用它同时构造接口请求、确定性边界和真实随机统计断言。
func scenarioWeights(scenario providerconfig.Scenario) map[string]float64 {
	weights := make(map[string]float64, len(scenario.Deployments))
	for _, deployment := range scenario.Deployments {
		weights[deployment.ID] = float64(deployment.Weight)
	}
	return weights
}

// runCounts 观察真实供应商调用、已提交部署 ID、随机权重分布和五层花费。
// 参数包含鉴权、公开模型、场景标签、最小请求数及部署权重；返回实际样本数，供调用方核对日志。
// 每个提示唯一；函数会产生真实本地网关调用和账单测试数据，但不会访问外部供应商。
func (f *supplierFixture) runCounts(t *testing.T, admin string, c chained, public, label string, requests int, weights map[string]float64) int {
	t.Helper()
	f.assertAllocationDraws(t, public, weights)
	requests = statisticalSampleSize(requests, weights)
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
	assertWeightedDistribution(t, label, counts, weights, requests)
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
	return requests
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
				f.h.ok(http.MethodPut, "/model/default", admin, map[string]any{"model_name": scenario.ModelName, "weights": map[string]any{"allocations": weights}})
				samples := f.runCounts(t, admin, c, scenario.ModelName, scenario.Name, scenario.Requests, scenarioWeights(scenario))
				if len(f.h.successRows(t, admin, scenario.ModelName)) != samples {
					t.Fatal("successful bill count differs from client calls")
				}
			})
		}
	}
}

func TestMultiSupplierTemplateWeights(t *testing.T) {
	for _, form := range []string{"complete", "partial"} {
		t.Run(form, func(t *testing.T) {
			s := supplierConfig(t).WeightedScenarios[0]
			f := newSupplierFixture(t, true, s)
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "template-suppliers")
			var weights any = []any{map[string]any{"deployment_id": "sol-a", "weight": 3}, map[string]any{"deployment_id": "sol-b", "weight": 7}}
			requests := 10
			want := map[string]float64{"sol-a": 3, "sol-b": 7}
			if form == "partial" {
				weights = []any{map[string]any{"deployment_id": "sol-a", "weight": 1}}
				requests = 8
				want = map[string]float64{"sol-a": 1, "sol-b": 1}
			}
			id := routeTemplate(t, f.h, admin, form, routeTemplateBody([]any{map[string]any{"model": s.ModelName, "strategy": "traffic-split", "allocations": weights}}, 1, 60, 0, 0))
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
		return routeTemplateBody([]any{map[string]any{
			"model": sol.ModelName, "strategy": "traffic-split", "allocations": []any{
				map[string]any{"deployment_id": "sol-a", "weight": wa},
				map[string]any{"deployment_id": "sol-b", "weight": wb},
			},
		}}, 1, 60, 0, 0)
	}
	f.h.setModelDefault(t, admin, sol.ModelName, map[string]float64{"sol-a": 1, "sol-b": 0})
	f.h.setModelDefault(t, admin, terra.ModelName, map[string]float64{"terra-0": 1, "terra-1": 0})
	team := routeTemplate(t, f.h, admin, "team rules", body(3, 7))
	bindTemplate(t, f.h, admin, "team", a.teamID, team)
	f.runCounts(t, admin, a, sol.ModelName, "team split", 10, map[string]float64{"sol-a": 3, "sol-b": 7})
	f.runCounts(t, admin, a, terra.ModelName, "other model default", 4, map[string]float64{"terra-0": 1, "terra-1": 0})
	f.runCounts(t, admin, b, sol.ModelName, "unbound tenant", 4, map[string]float64{"sol-a": 1, "sol-b": 0})
	key := routeTemplate(t, f.h, admin, "key rules", body(0, 1))
	bindTemplate(t, f.h, admin, "key", a.keyID, key)
	f.runCounts(t, admin, a, sol.ModelName, "key overrides team", 4, map[string]float64{"sol-a": 0, "sol-b": 1})
	if e := effectiveTemplate(t, f.h, admin, "key", a.keyID); e["scope_type"] != "key" {
		t.Fatal("wrong effective scope")
	}
	bindTemplate(t, f.h, admin, "key", a.keyID, "")
	f.h.ok(http.MethodPost, "/route_template/"+team+"/update", admin, map[string]any{"body": body(8, 2)})
	f.runCounts(t, admin, a, sol.ModelName, "edited team", 10, map[string]float64{"sol-a": 8, "sol-b": 2})
	f.runCounts(t, admin, b, sol.ModelName, "edit tenant isolation", 4, map[string]float64{"sol-a": 1, "sol-b": 0})
	bindTemplate(t, f.h, admin, "team", a.teamID, "")
	f.runCounts(t, admin, a, sol.ModelName, "clear to platform", 4, map[string]float64{"sol-a": 1, "sol-b": 0})
}

func TestMultiSupplierRetryAndBilling(t *testing.T) {
	for _, status := range []int{500, 429, 400} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := supplierConfig(t).WeightedScenarios[0]
			f := newSupplierFixture(t, true, s)
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "retry-suppliers")
			id := routeTemplate(t, f.h, admin, "retry", routeTemplateBody([]any{map[string]any{
				"model": s.ModelName, "strategy": "cost-based-routing",
			}}, 2, 60, 0, 0))
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
	f.h.setModelDefault(t, admin, s.ModelName, map[string]float64{
		s.Deployments[0].ID: 1,
		s.Deployments[1].ID: 0,
	})
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
	invalid := routeTemplateBody([]any{map[string]any{"model": s.ModelName, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"sol-a": 0, "sol-b": 0})}}, 1, 60, 0, 0)
	r := f.h.do(http.MethodPost, "/route_template/new", admin, map[string]any{"name": "no traffic", "body": invalid})
	if r.status != http.StatusBadRequest || len(f.attempts()) != 0 || !f.h.moneyOf(t, c).same(scopeMoney{}) {
		t.Fatalf("全零 allocations 未在写入时拒绝: %s %v", r.describe(), f.attempts())
	}
}

func TestMultiSupplierResponseAffinity(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "affinity-suppliers")
	f.h.setModelDefault(t, admin, s.ModelName, map[string]float64{"sol-a": 7, "sol-b": 3})
	r := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "first unique turn"), map[string]string{"X-Session-Id": "supplier-affinity"})
	if r.status != http.StatusOK {
		t.Fatalf("首次会话请求失败: %s", r.describe())
	}
	first := f.selected(t, c.key, s.ModelName, r)
	// Other traffic advances the scheduler; the continuation must remain pinned.
	f.runCounts(t, admin, c, s.ModelName, "advance scheduler", 10, map[string]float64{"sol-a": 7, "sol-b": 3})
	body := chatRequest(s.ModelName, "continuation with different prompt")
	next := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, map[string]string{"X-Session-Id": "supplier-affinity"})
	if next.status != http.StatusOK {
		t.Fatalf("同会话续接失败: %s", next.describe())
	}
	if router.CooldownID(f.selected(t, c.key, s.ModelName, next)) != router.CooldownID(first) {
		t.Fatal("continuation changed identical-model supplier")
	}
}

func TestMultiSupplierStreamingAndConcurrentWeights(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream-%t", stream), func(t *testing.T) {
			const requests = 100
			s := supplierConfig(t).WeightedScenarios[0]
			f := newSupplierFixture(t, true, s)
			admin := f.h.adminSession()
			c := f.h.openScope(t, admin, "parallel-suppliers")
			f.h.setModelDefault(t, admin, s.ModelName, map[string]float64{"sol-a": 7, "sol-b": 3})
			before := f.h.moneyOf(t, c)
			type result struct {
				r   reply
				err error
			}
			results := make(chan result, requests)
			limit := make(chan struct{}, 5)
			for i := 0; i < requests; i++ {
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
			for i := 0; i < requests; i++ {
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
			f.assertAllocationDraws(t, s.ModelName, map[string]float64{"sol-a": 7, "sol-b": 3})
			assertWeightedDistribution(t, "并发供应商分流", suppliers, map[string]float64{"SUPPLIER_A": 7, "SUPPLIER_B": 3}, requests)
			if len(f.attempts()) != requests {
				t.Fatalf("parallel split selected an unknown supplier: %v", suppliers)
			}
			if !stream {
				assertWeightedDistribution(t, "并发部署归属", counts, map[string]float64{"sol-a": 7, "sol-b": 3}, requests)
			}
			baseCost := 11*testInputRate + 5*testOutputRate
			expectedCost := float64(suppliers["SUPPLIER_A"]+2*suppliers["SUPPLIER_B"]) * baseCost
			if !nearlyEqual(total, expectedCost) {
				t.Fatalf("parallel/stream charge: got=%g want=%g", total, expectedCost)
			}
			after := f.h.moneyOf(t, c)
			if !after.grewBy(before, total) {
				t.Fatalf("parallel/stream scopes: before=%+v after=%+v charge=%g", before, after, total)
			}
			if rows := f.h.successRows(t, admin, s.ModelName); len(rows) != requests {
				t.Fatalf("parallel/stream successful logs: got=%d want=%d", len(rows), requests)
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
	id := routeTemplate(t, f.h, admin, "database-weights", routeTemplateBody([]any{map[string]any{
		"model": public, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"db-sol-a": 7, "db-sol-b": 3}),
	}}, 1, 60, 0, 0))
	bindTemplate(t, f.h, admin, "team", c.teamID, id)
	f.assertAllocationDraws(t, public, map[string]float64{"db-sol-a": 7, "db-sol-b": 3})
	before := f.h.moneyOf(t, c)
	counts := map[string]int{}
	total := 0.0
	const weightedRequests = 100
	for i := 0; i < weightedRequests; i++ {
		r := f.h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, fmt.Sprintf("database duplicate %d", i)))
		entry := f.selected(t, c.key, public, r)
		id := stringField(entry.ModelInfo, "id")
		counts[id]++
		total += f.checkBill(t, admin, public, r, entry)
	}
	assertWeightedDistribution(t, "数据库重复部署分流", counts, map[string]float64{"db-sol-a": 7, "db-sol-b": 3}, weightedRequests)
	if len(f.attempts()) != weightedRequests {
		t.Fatalf("database duplicate split=%v attempts=%d", counts, len(f.attempts()))
	}
	if !f.h.moneyOf(t, c).grewBy(before, total) || len(f.h.successRows(t, admin, public)) != weightedRequests {
		t.Fatal("database duplicate billing scopes or logs differ")
	}
	secondOnly := routeTemplateBody([]any{map[string]any{
		"model": public, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"db-sol-a": 0, "db-sol-b": 1}),
	}}, 1, 60, 0, 0)
	f.h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": secondOnly})
	firstPinned := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "pin second database row"), map[string]string{"X-Session-Id": "database-second"})
	if firstPinned.status != http.StatusOK || stringField(f.selected(t, c.key, public, firstPinned).ModelInfo, "id") != "db-sol-b" {
		t.Fatalf("未能把会话钉到第二条数据库部署: %s", firstPinned.describe())
	}
	f.h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": routeTemplateBody([]any{map[string]any{
		"model": public, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"db-sol-a": 7, "db-sol-b": 3}),
	}}, 1, 60, 0, 0)})
	body := chatRequest(public, "continue specifically on second database row")
	pinned := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, map[string]string{"X-Session-Id": "database-second"})
	if pinned.status != http.StatusOK {
		t.Fatalf("数据库部署会话续接失败: %s", pinned.describe())
	}
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

// TestMultiSupplierCooldownIsolation 验证同地址供应商按稳定部署身份隔离冷却，并在解除冷却后恢复随机分流。
// 前置私有 PostgreSQL、本地供应商和显式 Redis；先把专用会话固定到 A，避免随机首选使失败路径未执行。
// 结果验证 A 失败后回退 B、A 的独立 TTL、冷却期间 B 独占和恢复后权重；专用冷却键与 schema 自动清理。
func TestMultiSupplierCooldownIsolation(t *testing.T) {
	redisURL := os.Getenv("XHUB_REGRESSION_REDIS_URL")
	if redisURL == "" {
		t.Skip("cooldown isolation needs XHUB_REGRESSION_REDIS_URL")
	}
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "cooldown-suppliers")
	id := routeTemplate(t, f.h, admin, "cool one supplier", routeTemplateBody([]any{map[string]any{
		"model": s.ModelName, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"sol-a": 7, "sol-b": 3}),
	}}, 1, 60, 1, 120))
	bindTemplate(t, f.h, admin, "team", c.teamID, id)
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opt)
	keyA := "xhub:cooldown:" + router.CooldownID(f.entries[0])
	keyB := "xhub:cooldown:" + router.CooldownID(f.entries[1])
	defer func() { client.Del(t.Context(), keyA, keyB); client.Close() }()
	if err := client.Del(t.Context(), keyA, keyB).Err(); err != nil {
		t.Fatal(err)
	}
	// 先通过独占 A 的合法模板建立粘性，再恢复双供应商权重；故障请求必定先尝试 A。
	f.h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": routeTemplateBody([]any{map[string]any{
		"model": s.ModelName, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"sol-a": 1, "sol-b": 0}),
	}}, 1, 60, 1, 120)})
	sticky := map[string]string{"X-Session-Id": "cooldown-prime"}
	prime := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "prime supplier A"), sticky)
	if prime.status != http.StatusOK {
		t.Fatalf("建立 A 粘性失败: %s", prime.describe())
	}
	f.checkBill(t, admin, s.ModelName, prime, f.selected(t, c.key, s.ModelName, prime))
	f.h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": routeTemplateBody([]any{map[string]any{
		"model": s.ModelName, "strategy": "traffic-split", "allocations": routeAllocations(map[string]float64{"sol-a": 7, "sol-b": 3}),
	}}, 1, 60, 1, 120)})
	f.reset()
	f.fail("SUPPLIER_A", 500)
	r := f.h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "trip supplier A"), sticky)
	if r.status != http.StatusOK {
		t.Fatalf("A 故障后未恢复到 B: %s", r.describe())
	}
	f.checkBill(t, admin, s.ModelName, r, f.selected(t, c.key, s.ModelName, r))
	if !reflect.DeepEqual(f.attempts(), []supplierAttempt{{"SUPPLIER_A", s.ModelName, 500}, {"SUPPLIER_B", s.ModelName, 0}}) {
		t.Fatalf("trip order %v", f.attempts())
	}
	ttl, err := client.PTTL(t.Context(), keyA).Result()
	if err != nil || ttl <= 0 || ttl > 120*time.Second {
		t.Fatalf("supplier A cooldown TTL=%v err=%v", ttl, err)
	}
	if n, err := client.Exists(t.Context(), keyB).Result(); err != nil || n != 0 {
		t.Fatalf("supplier A cooled supplier B: %d %v", n, err)
	}
	f.fail("SUPPLIER_A", 0)
	f.runCounts(t, admin, c, s.ModelName, "healthy but cooling", 5, map[string]float64{"sol-a": 0, "sol-b": 1})
	if err := client.Del(t.Context(), keyA).Err(); err != nil {
		t.Fatal(err)
	}
	f.runCounts(t, admin, c, s.ModelName, "recovered split", 10, map[string]float64{"sol-a": 7, "sol-b": 3})
}
