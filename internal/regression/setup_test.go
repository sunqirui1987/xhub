package regression

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

// 一次测试调用被计费的两个单价。它们写在部署上，而不是从生成的价格目录里取：
// 目录是从供应商价目表刷新的，真实价格一变，回归不该跟着红。单位是每 token。
const (
	testInputRate  = 0.000002 // 输入每 token $2.00 / 百万
	testOutputRate = 0.000008 // 输出每 token $8.00 / 百万
)

// chatDeployment 是一个指向假供应商的对话模型。存的模型名就是网关转发给上游的
// 名字；部署自带单价，所以计费是确定的。
// 参数 name（string）：对外模型名。
// 返回 config.ModelEntry（config.ModelEntry）：可装进模型表的部署。
func chatDeployment(name string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"model":                 name,
			"api_key":               "sk-fake",
			"custom_llm_provider":   "openai",
			"input_cost_per_token":  testInputRate,
			"output_cost_per_token": testOutputRate,
		},
		ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
	}
}

// expectedCost 是一次假补全必须被扣的钱：假供应商报的 token 数乘部署上的单价。
// 计费断言拿它做基准，所以它必须和上面两个常量同源。
//
// 金额走和网关同一段计费，而不是在这里把单价乘一遍。两段各自用 float64 乘，
// 编译器对其中一段做常量折叠时会差一个最小单位。额度判定是精确的大于等于，
// 差这一点，上限刚好等于一次调用的用例就会放行下一次。
// 参数：无。
// 返回 float64（float64）：这一次调用的应付金额。
func expectedCost() float64 {
	charge, ok := catalog.CostFromFlatOrRates(func(field string) (float64, bool) {
		switch field {
		case "input_cost_per_token":
			return testInputRate, true
		case "output_cost_per_token":
			return testOutputRate, true
		default:
			return 0, false
		}
	}, catalog.Usage{
		PromptTokens:     defaultReply.PromptTokens,
		CompletionTokens: defaultReply.CompletionTokens,
	}, time.Date(2026, 1, 15, 3, 0, 0, 0, time.UTC))
	if !ok {
		return float64(defaultReply.PromptTokens)*testInputRate + float64(defaultReply.CompletionTokens)*testOutputRate
	}
	return charge.Total
}

// TestStartupDoesNotInstallRelayCapabilities 验证旧环境开关不再安装固定供应商凭据。
// 参数 t：测试上下文；返回：无。前置隔离 PostgreSQL，验证凭据为空、无上游请求、旧入口契约；协议传输由独立目录测试覆盖，harness 清理 schema。
func TestStartupDoesNotInstallRelayCapabilities(t *testing.T) {
	t.Setenv("XHUB_BUILTIN_PROVIDERS", "1")
	t.Setenv("FENNOAI_API_KEY", "ignored-key")
	t.Setenv("QINIU_API_KEY", "ignored-key")
	h := newHarness(t)
	admin := h.adminSession()
	rows := rowsOf(h.ok(http.MethodGet, "/credentials", admin, nil), "credentials")
	if len(rows) != 0 {
		t.Fatalf("启动自动生成了供应商凭据: %v", rows)
	}
	if len(h.upstreamCalls()) != 0 {
		t.Fatal("启动不应发现供应商模型")
	}
	for _, provider := range []string{"fennoai", "qiniu", "unknown"} {
		r := h.do(http.MethodPost, "/model/builtin/models", admin, map[string]any{"provider": provider})
		if r.status != http.StatusBadRequest {
			t.Fatalf("未保存凭据仍可发现模型: %s", r.describe())
		}
	}
	r := h.do(http.MethodPost, "/model/builtin/add", admin, map[string]any{"provider": "qiniu", "model_ids": []string{"removed-model"}})
	if r.status != http.StatusGone || !strings.Contains(errorMessage(r), "/model/new") {
		t.Fatalf("旧添加入口未退役: %s", r.describe())
	}
}

// TestScopedKeyCallsInference 证明给租户发出来的那把密钥真的能调推理。
// 所有额度用例都建立在这一点上：密钥本来就调不动的话，额度断言就没意义了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestScopedKeyCallsInference(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-chat"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "scoped")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-chat", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if got := stringField(r.json(), "id"); got == "" {
		t.Fatalf("completion had no id: %s", r.describe())
	}
}

// TestModelAllowListIsEnforced 证明限定模型的密钥在别的模型上会被拒。
// 有了它，额度用例才不可能因为"这把密钥本来什么都调不了"而假通过。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelAllowListIsEnforced(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-chat"), chatDeployment("regression-other"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "allowlist")
	restricted := h.modelKey(t, tn.session, tn.teamID, "restricted-key", "regression-chat")

	// 名单内的模型放行。
	h.ok(http.MethodPost, "/v1/chat/completions", restricted, map[string]any{
		"model": "regression-chat", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})

	// 名单外的模型拒绝，而且拒绝理由要说明是名单。
	r := h.do(http.MethodPost, "/v1/chat/completions", restricted, map[string]any{
		"model": "regression-other", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("a model outside the allow-list was not refused: %s", r.describe())
	}
	if got := stringField(r.json()["error"].(map[string]any), "message"); !strings.Contains(got, "allowed model list") {
		t.Fatalf("refusal did not name the allow-list: %s", r.describe())
	}
}
