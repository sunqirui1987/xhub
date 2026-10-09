package regression

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/providerconfig"
)

func validWeightedMetadata(t *testing.T) string {
	t.Helper()
	cfg := providerconfig.Config{
		Version: 1,
		Providers: []providerconfig.Provider{
			{ID: "ALPHA", Enabled: true, CredentialName: "alpha", KeyEnv: "ALPHA_KEY", Base: "https://alpha.example/v1", Protocol: "openai", Models: []string{"model"}},
		},
		WeightedScenarios: []providerconfig.Scenario{
			{Name: "split", ModelName: "public", Requests: 10, Deployments: []providerconfig.Deployment{
				{ID: "a", Provider: "ALPHA", Model: "model", Weight: 3},
				{ID: "b", Provider: "ALPHA", Model: "model", Weight: 7},
			}},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(raw)
}

func TestDecodeProviderMetadataRejectsInvalidInput(t *testing.T) {
	const credential = "credential-that-must-not-appear"
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{"invalid JSON", func(string) string { return "{" + credential }},
		{"invalid config", func(raw string) string { return strings.Replace(raw, `"version":1`, `"version":2`, 1) }},
		{"all-zero weights", func(raw string) string {
			raw = strings.Replace(raw, `"weight":3`, `"weight":0`, 1)
			return strings.Replace(raw, `"weight":7`, `"weight":0`, 1)
		}},
		{"incomplete cycles", func(raw string) string { return strings.Replace(raw, `"requests":10`, `"requests":9`, 1) }},
		{"unknown field", func(raw string) string {
			return strings.Replace(raw, `"version":1`, `"version":1,"secret":"`+credential+`"`, 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeProviderMetadata(test.mutate(validWeightedMetadata(t)))
			if err == nil {
				t.Fatal("decodeProviderMetadata succeeded")
			}
			if strings.Contains(err.Error(), credential) {
				t.Fatalf("error exposed metadata contents: %v", err)
			}
		})
	}
}

func TestExpectedWeightedCountsAvoidsIntegerOverflow(t *testing.T) {
	scenario := providerconfig.Scenario{
		Requests: 2,
		Deployments: []providerconfig.Deployment{
			{ID: "a", Weight: math.MaxInt},
			{ID: "b", Weight: math.MaxInt},
		},
	}
	got := expectedWeightedCounts(scenario)
	if got["a"] != 1 || got["b"] != 1 {
		t.Fatalf("expectedWeightedCounts() = %v, want one request each", got)
	}
}

// A real gateway may return 200 after switching deployments even with zero
// retries. Such a request must never be accepted as a healthy weight sample.
func TestWeightedObserverRejectsSuccessfulFallback(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "observed-fallback")
	result := f.h.setRouter(admin, map[string]any{"routing_strategy": "weighted-split", "num_retries": 0, "allowed_fails": 0})
	if result.status != http.StatusOK {
		t.Fatal(result.describe())
	}
	next := f.h.gw.Client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	observer := &weightedTransport{next: next}
	f.h.gw.Client.Transport = observer
	f.fail("SUPPLIER_A", 500)
	observer.begin(1)
	r := f.h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(s.ModelName, "observed fallback"))
	if r.status != http.StatusOK {
		t.Fatal(r.describe())
	}
	attempts := observer.snapshot()
	if len(attempts) != 2 || attempts[0].Status != 500 || attempts[1].Status != 200 || healthyWeightedCycle(attempts, 1) {
		t.Fatalf("successful fallback accepted as healthy: %+v", attempts)
	}
}

func TestHealthyWeightedCycleRequiresEveryRequest(t *testing.T) {
	for _, test := range []struct {
		name     string
		attempts []weightedAttempt
		healthy  bool
	}{
		{"healthy", []weightedAttempt{{Request: 1, Status: 200}, {Request: 2, Status: 200}}, true},
		{"missing", []weightedAttempt{{Request: 1, Status: 200}}, false},
		{"duplicate", []weightedAttempt{{Request: 1, Status: 200}, {Request: 1, Status: 200}}, false},
		{"transport failure", []weightedAttempt{{Request: 1, Failure: "transport_error"}, {Request: 2, Status: 200}}, false},
		{"HTTP failure", []weightedAttempt{{Request: 1, Status: 429}, {Request: 2, Status: 200}}, false},
		{"out of range", []weightedAttempt{{Request: 1, Status: 200}, {Request: 3, Status: 200}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := healthyWeightedCycle(test.attempts, 2); got != test.healthy {
				t.Fatalf("healthy=%v want=%v", got, test.healthy)
			}
		})
	}
}
