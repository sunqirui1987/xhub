package regression

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestLiveConfiguredVendorsAnswer 是这套测试里需要显式打开的那一半。
//
// 其余用例都跑在本地假供应商上，图的是确定、免费、快。这一条相反：它拿着环境
// 变量里的密钥去连真实供应商，因为假供应商只能证明网关自己的接线对，永远证明不了
// "网关发出去的请求形状还是供应商认的那个形状"。
//
// 供应商不写死在代码里，从环境变量发现（格式见 harness_test.go 的 liveVendor），
// 所以要接一家新的只加变量、不改代码：
//
//	XHUB_REGRESSION_LIVE=1
//	XHUB_REGRESSION_<ID>_KEY=<密钥>
//	XHUB_REGRESSION_<ID>_BASE=<chat 根地址>
//	XHUB_REGRESSION_<ID>_MODELS=<模型名,逗号分隔>
//	XHUB_REGRESSION_<ID>_PROTOCOL=openai|anthropic   （可选）
//
// 这里挑的是配置里给的模型、很短的提示，即便便宜也是要花钱的。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式或没配供应商时整个用例跳过。
func TestLiveConfiguredVendorsAnswer(t *testing.T) {
	for _, vendor := range liveCredentials(t) {
		for _, model := range vendor.Models {
			public := vendor.ID + "/" + model
			t.Run(public, func(t *testing.T) {
				h := newHarness(t, liveModelDeployment(vendor, model))
				h.live = true
				admin := h.adminSession()
				tn := h.provision(t, admin, "live-"+slugOf(public))

				r := h.liveCallFor(t, tn.key, public, vendor.Protocol, "Reply with the single word ok.")
				usage, _ := r.json()["usage"].(map[string]any)
				if usage == nil {
					t.Fatalf("the vendor answer carried no usage, so billing has nothing to read: %s", r.describe())
				}
				prompt, _ := liveCounts(usage)
				if prompt <= 0 {
					t.Fatalf("the vendor reported %d prompt tokens: %s", prompt, r.describe())
				}
				// 网关必须给这次调用定价。这里读出 0，就说明价目表里没有这条供应商
				// 正在服务的模型的费率。
				if cost := parseFloatOrZero(r.header("x-litellm-response-cost")); cost <= 0 {
					t.Fatalf("the call was priced at zero; the built-in catalog has no rate for %q", model)
				}
				t.Logf("%s answered with usage fields: %v", public, sortedKeys(usage))
			})
		}
	}
}

// TestLiveBypassCreatesAndPollsARealTask 证明 Bypass 对着真实供应商能建任务再查回来。
//
// 只有配了 XHUB_REGRESSION_<ID>_BYPASS_MODEL 的供应商会跑这一条：Bypass 的端点类型
// 因供应商而异，不是一个通用形状，所以它由配置点名而不是让套件去猜。
// 参数 t（*testing.T）：当前测试。
// 返回：无。没配这一项的供应商跳过。
func TestLiveBypassCreatesAndPollsARealTask(t *testing.T) {
	vendors := liveCredentials(t)
	var ran int
	for _, vendor := range vendors {
		model := bypassModelOf(vendor.ID)
		if model == "" {
			continue
		}
		ran++
		t.Run(vendor.ID+"/"+model, func(t *testing.T) {
			endpointType := bypassEndpointOf(vendor.ID)
			h := newHarness(t, config.ModelEntry{
				ModelName: vendor.ID + "/" + model,
				LiteLLMParams: map[string]any{
					"api_key":  vendor.Key,
					"api_base": vendor.BypassBase,
				},
				ModelInfo: map[string]any{"mode": endpointType},
			})
			h.live = true
			admin := h.adminSession()
			tn := h.provision(t, admin, "live-bypass-"+slugOf(vendor.ID))

			created := h.ok(http.MethodPost, "/v3/contents/generations/tasks", tn.key, map[string]any{
				"model":   vendor.ID + "/" + model,
				"content": []any{map[string]any{"type": "text", "text": "a red apple on a white table"}},
			})
			taskID := stringField(created.json(), "id")
			if taskID == "" {
				t.Fatalf("the vendor accepted no task: %s", created.describe())
			}

			// 查询这一步证明供应商返回的 id 就是网关能再查回去的 id，
			// 也就是调用方接下来要做的事。
			polled := h.ok(http.MethodGet, "/v3/contents/generations/tasks/"+taskID, tn.key, nil)
			if got := stringField(polled.json(), "id"); got != taskID {
				t.Fatalf("polling returned id=%q, want %q: %s", got, taskID, polled.describe())
			}
			if status := stringField(polled.json(), "status"); status == "" {
				t.Fatalf("the poll carried no status: %s", polled.describe())
			}
			t.Logf("created and polled vendor task %s", taskID)
		})
	}
	if ran == 0 {
		t.Skip("no vendor configured a bypass model; set XHUB_REGRESSION_<ID>_BYPASS_MODEL to exercise one")
	}
}

// bypassModelOf 读一家供应商配的 Bypass 模型名。没配时为空串，那家就不跑 Bypass。
// 参数 id（string）：供应商标识。
// 返回 string（string）：Bypass 模型名；没配时为空串。
func bypassModelOf(id string) string {
	return strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_BYPASS_MODEL"))
}

// bypassEndpointOf 读一家供应商 Bypass 部署用的端点类型。缺省是视频任务那一种。
// 参数 id（string）：供应商标识。
// 返回 string（string）：端点类型。
func bypassEndpointOf(id string) string {
	if v := strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_BYPASS_ENDPOINT")); v != "" {
		return v
	}
	return "qiniu_contents_generation"
}
