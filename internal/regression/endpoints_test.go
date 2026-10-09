package regression

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// seedanceDeployment 是内置的 Seedance Bypass 类型。model_info.transport 选择传输方式；
// 部署自己的名字就是供应商 id——Bypass 会就地把它上面属于网关的那层前缀去掉，
// 而不是另读一个"上游模型名"字段。
// 参数 name（string）：对外模型名，含 qiniu/ 前缀；transportID（string）：执行传输标识，据此显式声明 Ark 或 Fal 公开端点。
// 返回 config.ModelEntry（config.ModelEntry）：可装进模型表的部署。
func seedanceDeployment(name, transportID string) config.ModelEntry {
	endpointID, supplier := "bypass:ark-video", "qiniu"
	if strings.HasPrefix(transportID, "qiniu_fal_") {
		endpointID = "bypass:fal-video"
	}
	if strings.HasPrefix(transportID, "volcengine_") {
		supplier = "volcengine"
	}
	return config.ModelEntry{
		ModelName:     name,
		LiteLLMParams: map[string]any{"api_key": "sk-fake-upstream", "model": name, "custom_llm_provider": supplier},
		ModelInfo:     map[string]any{"transport": transportID, "endpoint_types": []string{endpointID}},
	}
}

// embeddingDeployment 是一个向量模型。能力来自 endpoint_types 字段——正是它决定
// 这条模型在 /v1/embeddings 上能调，而在 /v1/chat/completions 上不能。
// 参数 name（string）：对外模型名。
// 返回 config.ModelEntry（config.ModelEntry）：可装进模型表的部署。
func embeddingDeployment(name string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"model":               "openai/" + name,
			"api_key":             "sk-fake-upstream",
			"custom_llm_provider": "openai",
		},
		ModelInfo: map[string]any{"transport": "adapted", "endpoint_types": []string{"embedding"}},
	}
}

// TestChatEndpointRewritesAndAnswers 测最普通的 chat 路径：请求到了上游、转发出去
// 的是调用方写的模型名、回答按 OpenAI 形状回到调用方。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestChatEndpointRewritesAndAnswers(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-chat"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "chat-ep")
	h.resetUpstream()

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-chat", "messages": []any{map[string]any{"role": "user", "content": "ping"}},
	})
	body := r.json()
	choices, _ := body["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("completion had %d choices: %s", len(choices), r.describe())
	}
	if got := stringField(body, "object"); got != "chat.completion" {
		t.Fatalf("object=%q, want chat.completion", got)
	}

	calls := h.upstreamCalls()
	if len(calls) != 1 {
		t.Fatalf("upstream saw %d calls, want 1", len(calls))
	}
	if got := stringField(calls[0].Body, "model"); got != "regression-chat" {
		t.Fatalf("upstream saw model=%q, want the public name regression-chat", got)
	}
}

// TestEmbeddingEndpointUsesItsOwnPath 证明向量部署在 /v1/embeddings 上应答，并且
// 上游收到的路径和 chat 不一样。
//
// 这一条是有意加重的：一个"把所有请求都发到 chat"的故障网关，在只看状态码的
// 断言下照样绿，只有核对上游路径才抓得住。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEmbeddingEndpointUsesItsOwnPath(t *testing.T) {
	h := newHarness(t, embeddingDeployment("regression-embed"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "embed-ep")
	h.resetUpstream()

	r := h.ok(http.MethodPost, "/v1/embeddings", tn.key, map[string]any{
		"model": "regression-embed", "input": "ping",
	})
	if got := stringField(r.json(), "object"); got != "list" {
		t.Fatalf("embedding answer object=%q: %s", got, r.describe())
	}

	calls := h.upstreamCalls()
	if len(calls) != 1 {
		t.Fatalf("upstream saw %d calls, want 1", len(calls))
	}
	if !containsPath(calls[0].Path, "/embeddings") {
		t.Fatalf("embedding went to %q, want an /embeddings path", calls[0].Path)
	}
}

// TestSeedanceBypassCreatesAndPollsATask 是发布必须依赖的 Bypass 用例：通过网关自己
// 的路径创建一个 Seedance 任务，Bypass 把正文改写成供应商的内容生成形状，
// 任务 id 回到调用方手里供它随后查询。
//
// 2.0 和 2.5 各跑一遍，因为它们的模型名不同而端点类型相同。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSeedanceBypassCreatesAndPollsATask(t *testing.T) {
	for _, probe := range []struct {
		name         string
		endpointType string
		modelID      string
	}{
		{"seedance 2.0", "qiniu_contents_generation", "qiniu/bytedance/doubao-seedance-2-0-260128"},
		{"seedance 2.5", "qiniu_contents_generation", "qiniu/bytedance/doubao-seedance-2-5-260628"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, seedanceDeployment(probe.modelID, probe.endpointType))
			admin := h.adminSession()
			tn := h.provision(t, admin, "seedance")
			h.resetUpstream()

			created := h.ok(http.MethodPost, "/v3/contents/generations/tasks", tn.key, map[string]any{
				"model":   probe.modelID,
				"content": []any{map[string]any{"type": "text", "text": "a cat on a beach"}},
			})
			taskID := stringField(created.json(), "id")
			if taskID == "" {
				t.Fatalf("create returned no task id: %s", created.describe())
			}

			calls := h.upstreamCalls()
			if len(calls) != 1 {
				t.Fatalf("upstream saw %d calls, want 1", len(calls))
			}
			// Bypass 去掉的是网关自己那层前缀，供应商那层留着。
			if got := stringField(calls[0].Body, "model"); got != "bytedance/doubao-seedance-2-0-260128" && got != "bytedance/doubao-seedance-2-5-260628" {
				t.Fatalf("upstream saw model=%q, want the vendor id with only the gateway prefix stripped", got)
			}
			if !containsPath(calls[0].Path, "contents/generations/tasks") {
				t.Fatalf("create went to %q, want the content-generation path", calls[0].Path)
			}

			// 查询走同一个端点类型，只是把任务 id 放进路径里。
			polled := h.ok(http.MethodGet, "/v3/contents/generations/tasks/"+taskID, tn.key, nil)
			if got := stringField(polled.json(), "id"); got != taskID {
				t.Fatalf("poll returned id=%q, want %q: %s", got, taskID, polled.describe())
			}
		})
	}
}

// TestABypassCannotBeFilledInFromTheConsole 钉住一个决定：bypass 只能来自后台
// 登记的转发方式，运维在界面上填一份路径表并不构成一条能用的端点。
//
// 理由是这样填出来的东西做不成。路径表填对了也只是把请求发出去，填错了就发不出去；
// 两种情况都拿不到上游的结构化返回值，于是预选、日志、用量、任务 id 的跟进都没有
// 依据。上游必须和 chat、image、video 一样是固定的，只是它走原样转发。
//
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestABypassCannotBeFilledInFromTheConsole(t *testing.T) {
	h := newHarness(t, customBypassDeployment("text_to_model"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "no-custom-bypass")
	h.resetUpstream()

	r := h.do(http.MethodPost, "/v2/openapi/task", tn.key, map[string]any{
		"type": "text_to_model", "prompt": "a small cat",
	})
	// 既不是成功，也不该碰到上游——这条路径没有任何转发方式认领。
	if r.status < 300 {
		t.Fatalf("a hand-filled bypass document was served: %s", r.describe())
	}
	if got := len(h.upstreamCalls()); got != 0 {
		t.Fatalf("an unregistered path was forwarded upstream %d times", got)
	}
}

// customBypassDeployment 是一条自带端点文档的部署，用来证明这种写法已经不管用。
// 它模拟的是旧的行：那时运维可以在界面上填一份路径表当作自定义 Bypass。
// 参数 name（string）：对外模型名，也是这套形状里作为模型名的那个值。
// 返回 config.ModelEntry（config.ModelEntry）：带自定义端点文档的部署。
func customBypassDeployment(name string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"api_key": "sk-fake-upstream",
			"endpoint": map[string]any{
				"kind":         "bypass",
				"model_field":  "type",
				"task_id":      "data.task_id",
				"strip_prefix": "",
				"actions": []any{
					map[string]any{"name": "create", "method": "POST", "public_path": "/v2/openapi/task", "upstream_path": "/v2/openapi/task"},
					map[string]any{"name": "get", "method": "GET", "public_path": "/v2/openapi/task/{task_id}", "upstream_path": "/v2/openapi/task/{task_id}"},
				},
			},
		},
		ModelInfo: map[string]any{"mode": "custom"},
	}
}

// containsPath 判断上游路径里是否含某个片段。
// 参数 path（string）：上游收到的路径；fragment（string）：要找的片段。
// 返回 bool（bool）：含有时为真。
func containsPath(path, fragment string) bool { return strings.Contains(path, fragment) }
