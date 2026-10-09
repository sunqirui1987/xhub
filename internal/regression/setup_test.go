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
			"model":                 "openai/" + name,
			"api_key":               "sk-fake",
			"custom_llm_provider":   "openai",
			"input_cost_per_token":  testInputRate,
			"output_cost_per_token": testOutputRate,
		},
		ModelInfo: map[string]any{"transport": "adapted", "endpoint_types": []string{"chat"}},
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

// TestProviderSetupAddsFennoaiAndQiniu 证明两个内置供应商能配起来、它们的模型目录
// 读得到、并且从目录里加进来的模型真的能调。
//
// 它是后面每一个用例的前提，所以断言的是整条链而不只是状态码：凭据存下了、
// 用这个凭据目录读得到、它加进来的模型真的完成了一次补全。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestProviderSetupAddsFennoaiAndQiniu(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()

	// 目录读得到：假供应商会对 /v1/models 回一个补全形状的正文，所以这一条
	// 走通了读取链路，又不用连真实供应商。
	t.Run("catalog is readable with the operator's key", func(t *testing.T) {
		for _, provider := range []string{"fennoai", "qiniu"} {
			r := h.do(http.MethodPost, "/model/builtin/models", admin, map[string]any{"provider": provider})
			if r.status != http.StatusOK {
				t.Fatalf("%s catalog -> %s", provider, r.describe())
			}
			if got := stringField(r.json(), "provider"); got != provider {
				t.Fatalf("%s catalog answered provider=%q", provider, got)
			}
		}
	})

	// 旧入口已经退役；目录导入后统一走 /price/model 和 /model/new。
	t.Run("the old builtin add endpoint is gone", func(t *testing.T) {
		r := h.do(http.MethodPost, "/model/builtin/add", admin, map[string]any{
			"provider":  "fennoai",
			"model_ids": []string{"regression-fenno-model"},
		})
		if r.status != http.StatusGone {
			t.Fatalf("retired add endpoint -> %s", r.describe())
		}
		message := errorMessage(r)
		if !strings.Contains(message, "/price/model") || !strings.Contains(message, "/model/new") {
			t.Fatalf("retirement does not direct the caller to the unified flow: %q", message)
		}
	})

	// 不认识的供应商要拒绝，否则"供应商"这个字段就没有约束力。
	t.Run("an unknown provider is refused", func(t *testing.T) {
		if r := h.do(http.MethodPost, "/model/builtin/models", admin, map[string]any{"provider": "not-a-provider"}); r.status != http.StatusBadRequest {
			t.Fatalf("unknown provider -> %s", r.describe())
		}
	})
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
