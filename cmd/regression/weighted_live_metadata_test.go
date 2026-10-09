package regression

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/providerconfig"
)

// validWeightedMetadata 构造不含秘密值的最小合法权重元数据。
// 参数 t（*testing.T）：序列化失败时承载断言；返回 JSON 字符串，供严格解码边界测试使用，无外部副作用。
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

// TestWeightedObserverRejectsSuccessfulFallback 验证一次 500 后切换部署得到的 200 不能算健康权重样本。
// 前置为本地双供应商场景，并通过模型默认分配让故障供应商优先；检查观察器保留两次尝试，harness 负责清理。
// 参数 t（*testing.T）：当前回归测试；返回：无。
func TestWeightedObserverRejectsSuccessfulFallback(t *testing.T) {
	s := supplierConfig(t).WeightedScenarios[0]
	f := newSupplierFixture(t, true, s)
	admin := f.h.adminSession()
	c := f.h.openScope(t, admin, "observed-fallback")
	allocations := make([]any, 0, len(s.Deployments))
	for _, dep := range s.Deployments {
		weight := 1
		if dep.Provider == "SUPPLIER_A" {
			weight = 1000
		}
		allocations = append(allocations, map[string]any{"deployment_id": dep.ID, "weight": weight})
	}
	f.h.ok(http.MethodPut, "/model/default", admin, map[string]any{"model_name": s.ModelName, "weights": map[string]any{"allocations": allocations}})
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

// TestHealthyWeightedCycleRequiresEveryRequest 验证健康周期要求每个请求恰有一次成功尝试。
// 前置为纯内存观察记录；覆盖缺失、重复、传输失败、HTTP 失败和越界编号，无资源需要清理。
// 参数 t（*testing.T）：当前单元测试；返回：无。
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
