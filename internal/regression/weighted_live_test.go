package regression

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/providerconfig"
	"github.com/sunqirui1987/xhub/internal/router"
)

var invalidProviderMetadata = errors.New("invalid provider metadata")

// Observe only attempt outcomes. Never persist URLs, headers, bodies or raw
// errors: any of them may contain a supplier credential.
type weightedAttempt struct {
	Request int    `json:"request"`
	Status  int    `json:"status"`
	Failure string `json:"failure,omitempty"`
}

type weightedTransport struct {
	next     http.RoundTripper
	mu       sync.Mutex
	request  int
	attempts []weightedAttempt
}

func (o *weightedTransport) begin(request int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.request = request
}

func (o *weightedTransport) snapshot() []weightedAttempt {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]weightedAttempt(nil), o.attempts...)
}

func (o *weightedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	o.mu.Lock()
	index := o.request
	o.mu.Unlock()
	resp, err := o.next.RoundTrip(req)
	a := weightedAttempt{Request: index}
	if resp != nil {
		a.Status = resp.StatusCode
	}
	if err != nil {
		a.Failure = "transport_error"
	}
	o.mu.Lock()
	o.attempts = append(o.attempts, a)
	o.mu.Unlock()
	return resp, err
}

// An exact healthy cycle requires one successful upstream attempt per request.
// A final HTTP 200 after fallback does not satisfy that precondition.
func healthyWeightedCycle(attempts []weightedAttempt, requests int) bool {
	if len(attempts) != requests {
		return false
	}
	seen := make(map[int]bool, requests)
	for _, a := range attempts {
		if a.Request < 1 || a.Request > requests || seen[a.Request] || a.Failure != "" || a.Status < 200 || a.Status >= 300 {
			return false
		}
		seen[a.Request] = true
	}
	return true
}

// decodeProviderMetadata applies the complete shared schema to metadata passed
// by the live wrapper. Errors omit input text because it is adjacent to secrets.
func decodeProviderMetadata(raw string) (providerconfig.Config, error) {
	var cfg providerconfig.Config
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return providerconfig.Config{}, invalidProviderMetadata
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return providerconfig.Config{}, invalidProviderMetadata
	}
	if err := providerconfig.Validate(cfg); err != nil {
		return providerconfig.Config{}, fmt.Errorf("%w: %v", invalidProviderMetadata, err)
	}
	return cfg, nil
}

// expectedWeightedCounts uses reduced ratios so adding large weights cannot
// overflow an int before exact expected counts are calculated.
func expectedWeightedCounts(s providerconfig.Scenario) map[string]int {
	var divisor uint64
	for _, deployment := range s.Deployments {
		divisor = weightedGCD(divisor, uint64(deployment.Weight))
	}
	var reducedCycle uint64
	for _, deployment := range s.Deployments {
		reducedCycle += uint64(deployment.Weight) / divisor
	}
	cycles := uint64(s.Requests) / reducedCycle
	expected := make(map[string]int, len(s.Deployments))
	for _, deployment := range s.Deployments {
		expected[deployment.ID] = int(cycles * (uint64(deployment.Weight) / divisor))
	}
	return expected
}

func weightedGCD(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// This calls real suppliers through the real gateway. PlanRoute reads the
// committed response affinity to observe the successful deployment without
// inferring routing from identical upstream model names or exposing credentials.
func TestLiveConfiguredWeightedRouting(t *testing.T) {
	raw := os.Getenv("E2E_PROVIDER_METADATA")
	if raw == "" {
		t.Skip("use config_provider.yaml through scripts/with-live-vendors.py")
	}
	cfg, err := decodeProviderMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WeightedScenarios) == 0 {
		t.Skip("no weighted scenarios selected")
	}
	// Validate metadata before credentials are loaded or a paid harness exists.
	vendors := liveCredentials(t)
	byVendor := map[string]liveVendor{}
	for _, v := range vendors {
		byVendor[v.ID] = v
	}
	for _, s := range cfg.WeightedScenarios {
		t.Run(s.Name, func(t *testing.T) {
			evidence := map[string]any{"scenario": s.Name, "model": s.ModelName, "requests": s.Requests, "status": "failed", "calls": []any{}, "expected": expectedWeightedCounts(s)}
			calls := []map[string]any{}
			var observer *weightedTransport
			defer func() {
				evidence["calls"] = calls
				if observer != nil {
					attempts := observer.snapshot()
					evidence["attempts"] = attempts
					evidence["healthy_cycle"] = healthyWeightedCycle(attempts, s.Requests)
					if !healthyWeightedCycle(attempts, s.Requests) && evidence["failure_reason"] == nil {
						evidence["failure_reason"] = "healthy weighted cycle incomplete; inspect attempt outcomes and backend assertion"
					}
				}
				if dir := os.Getenv("E2E_EVIDENCE_DIR"); dir != "" {
					_ = os.MkdirAll(dir, 0700)
					data, _ := json.MarshalIndent(evidence, "", "  ")
					if err := os.WriteFile(filepath.Join(dir, "weighted-"+slugOf(s.Name)+".json"), data, 0600); err != nil {
						t.Errorf("write routing evidence: %v", err)
					}
				}
			}()
			var entries []config.ModelEntry
			for _, d := range s.Deployments {
				v, ok := byVendor[d.Provider]
				if !ok {
					t.Fatalf("missing selected provider %s", d.Provider)
				}
				entry := liveModelDeployment(v, d.Model)
				entry.ModelName = s.ModelName
				// Stable IDs distinguish otherwise identical connections, including cooldown
				// and affinity identity. Unique per run so shared split cursors start clean.
				entry.LiteLLMParams["deployment_id"] = d.ID + "-" + fmt.Sprint(time.Now().UnixNano())
				entry.LiteLLMParams["weight"] = d.Weight
				entries = append(entries, entry)
			}
			h := newHarness(t, entries...)
			h.live = true
			next := h.gw.Client.Transport
			if next == nil {
				next = http.DefaultTransport
			}
			observer = &weightedTransport{next: next}
			h.gw.Client.Transport = observer
			admin := h.adminSession()
			c := h.openScope(t, admin, "weighted-live")
			result := h.setRouter(admin, map[string]any{"routing_strategy": "weighted-split", "num_retries": 0, "timeout": 90})
			if result.status != 200 {
				t.Fatalf("configure split: %s", result.describe())
			}
			// Resolve the same caller principal used by the inference request.
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Header.Set("Authorization", "Bearer "+c.key)
			principal := h.gw.RequireLLMPrincipal(httptest.NewRecorder(), req)
			if principal == nil {
				t.Fatal("cannot resolve inference principal")
			}
			counts := map[string]int{}
			bills := map[string]float64{}
			charged := 0.0
			before := h.moneyOf(t, c)
			for i := 0; i < s.Requests; i++ {
				observer.begin(i + 1)
				prompt := fmt.Sprintf("Request %d. Reply only ok.", i)
				r := h.liveCallFor(t, c.key, s.ModelName, "openai", prompt)
				id := stringField(r.json(), "id")
				if id == "" {
					t.Fatal("real response has no id")
				}
				plan := h.gw.PlanRoute(req, s.ModelName, map[string]any{"previous_response_id": id}, principal)
				selected := ""
				provider := ""
				for index, entry := range entries {
					if router.CooldownID(entry) == plan.Pinned {
						selected = s.Deployments[index].ID
						provider = s.Deployments[index].Provider
					}
				}
				if selected == "" {
					t.Fatal("successful request has no committed deployment attribution")
				}
				cost := parseFloatOrZero(r.header("x-litellm-response-cost"))
				if cost <= 0 {
					t.Fatal("real weighted call has no positive charge")
				}
				callID := r.header("x-litellm-call-id")
				row := logDetail(t, h, admin, callID)
				if row["model"] != s.ModelName || !nearlyEqual(numberOrZero(row["spend"]), cost) {
					t.Fatal("weighted bill does not match public alias and response charge")
				}
				usage, _ := r.json()["usage"].(map[string]any)
				pt, ct := liveCounts(usage)
				if pt <= 0 || ct <= 0 || numberOrZero(row["prompt_tokens"]) != float64(pt) || numberOrZero(row["completion_tokens"]) != float64(ct) {
					t.Fatal("weighted usage does not match durable token counts")
				}
				bill := breakdownOf(t, h, admin, callID)
				sum := 0.0
				if bill["source"] != "snapshot" {
					t.Fatal("weighted bill has no durable rate snapshot")
				}
				rates, _ := bill["applied"].([]any)
				for _, item := range rates {
					rate, _ := item.(map[string]any)
					sum += numberOrZero(rate["quantity"]) * numberOrZero(rate["usd"])
				}
				if !nearlyEqual(sum, cost) {
					t.Fatal("weighted bill quantities times rates do not equal charge")
				}
				counts[selected]++
				bills[selected] += cost
				charged += cost
				calls = append(calls, map[string]any{"call_id": callID, "deployment": selected, "provider": provider, "prompt_tokens": pt, "completion_tokens": ct, "cost_usd": cost})
				t.Logf("weighted %s request=%d/%d deployment=%s supplier=%s cost=%g", s.Name, i+1, s.Requests, selected, provider, cost)
			}
			evidence["counts"] = counts
			evidence["cost_by_deployment"] = bills
			expected := expectedWeightedCounts(s)
			for _, d := range s.Deployments {
				if counts[d.ID] != expected[d.ID] {
					t.Errorf("deployment %s received %d; want exactly %d in complete weighted cycles", d.ID, counts[d.ID], expected[d.ID])
				}
			}
			evidence["expected"] = expected
			if !healthyWeightedCycle(observer.snapshot(), s.Requests) {
				reason := "upstream failure or fallback invalidated the healthy weighted cycle; successful response counts cannot alone prove routing weights (num_retries=0 still permits deployment fallback)"
				evidence["failure_reason"] = reason
				t.Error(reason)
			}
			if rows := h.successRows(t, admin, s.ModelName); len(rows) != s.Requests {
				t.Errorf("got %d successful bills, want %d", len(rows), s.Requests)
			}
			if !h.moneyOf(t, c).grewBy(before, charged) {
				t.Error("user/key/project/team/organization spend does not equal total weighted charges")
			}
			if !t.Failed() {
				evidence["status"] = "passed"
			}
		})
	}
}
