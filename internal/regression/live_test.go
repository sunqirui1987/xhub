package regression

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestLiveFennoaiAndQiniuCallTheRealVendors 是这套测试里需要显式打开的那一半。
//
// 其余用例都跑在本地假供应商上，图的是确定、免费、快。这一条相反：它拿着环境
// 变量里的密钥去连真实供应商，因为假供应商只能证明网关自己的接线对，
// 永远证明不了"网关发出去的请求形状还是供应商认的那个形状"。
//
// 除非 XHUB_REGRESSION_LIVE=1，否则它整体跳过。开了之后还需要：
//
//	XHUB_REGRESSION_FENNO_KEY, XHUB_REGRESSION_QINIU_KEY
//
// 这里挑的是便宜且稳定的模型。即便便宜也是要花钱的，所以只要求它回一个很短的
// 回答。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式时整个用例跳过。
func TestLiveFennoaiAndQiniuCallTheRealVendors(t *testing.T) {
	keys := liveCredentials(t)

	t.Run("fennoai chat", func(t *testing.T) {
		h := newHarness(t, liveChatDeployment("fennoai/gpt-5.6-sol", "fennoai", "openai", liveChatBase("fennoai"), keys.fenno))
		admin := h.adminSession()
		tn := h.provision(t, admin, "live-fenno")

		r := h.liveCall(t, tn.key, "fennoai/gpt-5.6-sol", "Reply with the single word ok.")
		usage, _ := r.json()["usage"].(map[string]any)
		if usage == nil {
			t.Fatalf("the vendor answer carried no usage, so billing has nothing to read: %s", r.describe())
		}
		prompt, _ := floatField(usage, "prompt_tokens")
		if prompt <= 0 {
			t.Fatalf("the vendor reported %v prompt tokens: %s", prompt, r.describe())
		}
		// 网关必须给这次调用定价。这里读出 0，就说明价目表里没有这条供应商
		// 正在服务的模型的费率。
		if cost := parseFloatOrZero(r.header("x-litellm-response-cost")); cost <= 0 {
			t.Fatalf("the call was priced at zero; the catalog has no rate for fennoai/gpt-5.6-sol")
		}
	})

	t.Run("qiniu chat", func(t *testing.T) {
		h := newHarness(t, liveChatDeployment("qiniu/deepseek-v3", "qiniu", "openai", liveChatBase("qiniu"), keys.qiniu))
		admin := h.adminSession()
		tn := h.provision(t, admin, "live-qiniu")

		r := h.liveCall(t, tn.key, "qiniu/deepseek-v3", "Reply with the single word ok.")
		usage, _ := r.json()["usage"].(map[string]any)
		if usage == nil {
			t.Fatalf("the vendor answer carried no usage: %s", r.describe())
		}
		if cost := parseFloatOrZero(r.header("x-litellm-response-cost")); cost <= 0 {
			t.Fatal("the call was priced at zero; the catalog has no rate for qiniu/deepseek-v3")
		}
	})

	// Seedance 这条是自定义 Bypass 对着真实供应商跑：先建任务，再查任务——
	// 这正是这个端点类型存在的意义。
	t.Run("qiniu seedance bypass creates a real task", func(t *testing.T) {
		const model = "qiniu/bytedance/doubao-seedance-2-0-260128"
		h := newHarness(t, liveBypassDeployment(model, "qiniu_contents_generation", liveBypassBase("qiniu"), keys.qiniu))
		admin := h.adminSession()
		tn := h.provision(t, admin, "live-seedance")

		created := h.ok(http.MethodPost, "/v3/contents/generations/tasks", tn.key, map[string]any{
			"model":   model,
			"content": []any{map[string]any{"type": "text", "text": "a red apple on a white table"}},
		})
		taskID := stringField(created.json(), "id")
		if taskID == "" {
			t.Fatalf("the vendor accepted no task: %s", created.describe())
		}
		t.Logf("created vendor task %s", taskID)

		// 查询这一步证明供应商返回的 id 就是网关能再查回去的 id，
		// 也就是调用方接下来要做的事。
		polled := h.ok(http.MethodGet, "/v3/contents/generations/tasks/"+taskID, tn.key, nil)
		if got := stringField(polled.json(), "id"); got != taskID {
			t.Fatalf("polling returned id=%q, want %q: %s", got, taskID, polled.describe())
		}
		if status := stringField(polled.json(), "status"); status == "" {
			t.Fatalf("the poll carried no status: %s", polled.describe())
		}
	})
}

// liveCall 发一次带小额度上限的补全，让真实运行保持便宜。
// 失败时把供应商自己的错误原样报出来，而不是含糊地红掉——那多半是凭据或
// 供应商抖动，不是网关的缺陷。
// 参数 t（*testing.T）：当前测试；key/model/prompt（string）：密钥、模型、提示词。
// 返回 reply（reply）：供应商的响应。非 200 时已经让用例失败并说明原因。
func (h *harness) liveCall(t *testing.T, key, model, prompt string) reply {
	t.Helper()
	r := h.do(http.MethodPost, "/v1/chat/completions", key, map[string]any{
		"model":      model,
		"messages":   []any{map[string]any{"role": "user", "content": prompt}},
		"max_tokens": 16,
	})
	if r.status != http.StatusOK {
		t.Fatalf("the live vendor refused the call (%d). This is either a credential problem or a vendor outage, not a gateway defect: %s",
			r.status, r.describe())
	}
	return r
}

// liveChatDeployment 是一条指向真实供应商的对话部署。
// 对外名字带着供应商前缀，这样日志里的供应商 id 不会有歧义。
// 参数 name（string）：对外模型名，形如 qiniu/deepseek-v3；provider（string）：供应商标识；
// customProvider（string）：协议适配用的供应商标识；base（string）：供应商根地址；
// key（string）：供应商密钥。返回 config.ModelEntry（config.ModelEntry）：装进模型表的部署。
func liveChatDeployment(name, provider, customProvider, base, key string) config.ModelEntry {
	upstream := name
	if _, rest, ok := strings.Cut(name, "/"); ok {
		upstream = rest
	}
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"model":               upstream,
			"api_key":             key,
			"api_base":            base,
			"custom_llm_provider": customProvider,
			// 单价写死，这样测试不依赖生成目录里刚好有这条模型的费率——
			// 它可能刚发布。
			"input_cost_per_token":  testInputRate,
			"output_cost_per_token": testOutputRate,
		},
		ModelInfo: map[string]any{"mode": "chat"},
	}
}

// liveBypassDeployment 是一条指向真实供应商的 Bypass 部署。
// 它把供应商 id 留在对外名字里，因为 Bypass 正是从这个名字上剥掉网关那层前缀
// 来拼上游请求的。
// 参数 name（string）：对外模型名；endpointType（string）：端点类型；base（string）：供应商根地址；
// key（string）：供应商密钥。返回 config.ModelEntry（config.ModelEntry）：装进模型表的部署。
func liveBypassDeployment(name, endpointType, base, key string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"api_key":  key,
			"api_base": base,
		},
		ModelInfo: map[string]any{"mode": endpointType},
	}
}
