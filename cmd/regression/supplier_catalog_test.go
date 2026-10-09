package regression

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestSupplierCatalogLifecycle 验证真实连接目录、模型创建/编辑、数据面及连接变更后拒绝旧能力。
// 参数 t 为回归上下文；前置隔离数据库、本地 Ark/Fal 服务及 OpenAI 兼容连接；模型和连接显式删除，schema 自动清理。
func TestSupplierCatalogLifecycle(t *testing.T) {
	var calls atomic.Int32
	var discoveryUnavailable atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/models" && r.Header.Get("Authorization") == "Bearer sk-fake" {
			if discoveryUnavailable.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, map[string]any{"data": []any{map[string]any{"id": "gpt-5.6-sol"}, map[string]any{"id": "bytedance/seedance-2.0/text-to-video"}}})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			writeJSON(w, map[string]any{"id": "ordinary-chat", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
			return
		}
		if r.URL.Path == "/v3/contents/generations/tasks" && r.Header.Get("Authorization") == "Bearer sk-fake" {
			writeJSON(w, map[string]any{"id": "catalog-task"})
			return
		}
		if r.URL.Path == "/queue/bytedance/seedance-2.0/text-to-video" && r.Header.Get("Authorization") == "Key sk-fake" {
			writeJSON(w, map[string]any{"request_id": "catalog-fal-task"})
			return
		}
		t.Errorf("供应商协议或鉴权错误: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(400)
	}))
	defer up.Close()
	h := newHarness(t)
	admin := h.adminSession()
	credential := map[string]any{"credential_name": "friendly-video", "credential_info": map[string]any{"custom_llm_provider": "openai", "catalog_id": "qiniu"}, "credential_values": map[string]any{"api_base": up.URL, "api_key": "sk-fake"}}
	h.ok("POST", "/credentials", admin, credential)
	defer h.ok("DELETE", "/credentials/friendly-video", admin, nil)
	public := h.ok("GET", "/public/endpoints", admin, nil).json()
	bindings := public["catalogs"].(map[string]any)["qiniu"].(map[string]any)
	if len(bindings["bytedance/seedance-2.0/text-to-video"].([]any)) == 0 {
		t.Fatal("公开目录未提供 Fal 实现绑定")
	}
	// 未实现的型号必须在真实保存接口被拒绝，即使调用方选择了已登记执行。
	invalid := h.do("POST", "/model/new", admin, map[string]any{"model_name": "unimplemented", "litellm_params": map[string]any{"model": "unknown-video", "custom_llm_provider": "openai", "litellm_credential_name": "friendly-video"}, "model_info": map[string]any{"transport": "qiniu_contents_generation", "endpoint_types": []string{"bypass:ark-video"}}})
	if invalid.status != 400 {
		t.Fatalf("未实现型号保存未拒绝: %s", invalid.describe())
	}
	catalog := h.ok("POST", "/model/builtin/models", admin, map[string]any{"credential_name": "friendly-video"}).json()
	if len(catalog["models"].([]any)) < 3 || calls.Load() != 1 || catalog["error"] != nil {
		t.Fatal("登记目录未合并供应商完整 /models 与专用型号")
	}
	// 上游目录不可用时必须保留本地型号并返回真实错误；503 不通过更换 /v1 重试。
	discoveryUnavailable.Store(true)
	fallback := h.ok("POST", "/model/builtin/models", admin, map[string]any{"credential_name": "friendly-video"}).json()
	if len(fallback["models"].([]any)) == 0 || fallback["error"] == nil || calls.Load() != 2 {
		t.Fatalf("供应商目录失败未保留专用型号与错误: %v", fallback)
	}
	discoveryUnavailable.Store(false)
	var ids []string
	defer func() {
		for _, id := range ids {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
		}
	}()
	// 目录未登记的普通模型仍按默认 OpenAI 保存、持久化和执行。
	ordinary := h.ok("POST", "/model/new", admin, map[string]any{"model_name": "ordinary/chat:latest", "litellm_params": map[string]any{"model": "gpt-5.6-sol", "custom_llm_provider": "openai", "litellm_credential_name": "friendly-video", "output_cost_per_token": 0.00001}, "model_info": map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}, "pricing_source": "manual"}}).json()
	ids = append(ids, ordinary["model_info"].(map[string]any)["id"].(string))
	h.ok("POST", "/v1/chat/completions", admin, map[string]any{"model": "ordinary/chat:latest", "messages": []any{map[string]any{"role": "user", "content": "test"}}})
	for _, tc := range []struct{ model, transport, endpoint, path, key string }{
		{"bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation", "bypass:ark-video", "/v3/contents/generations/tasks", "id"},
		{"bytedance/seedance-2.0/text-to-video", "qiniu_fal_doubao_20", "fal:queue", "/queue/bytedance/seedance-2.0/text-to-video", "request_id"},
	} {
		params := map[string]any{"model": tc.model, "custom_llm_provider": "openai", "litellm_credential_name": "friendly-video", "output_cost_per_token": 0.00001}
		info := map[string]any{"transport": tc.transport, "endpoint_types": []string{tc.endpoint}, "pricing_source": "manual", "catalog_id": "forged"}
		body := map[string]any{"model_name": "catalog/" + tc.transport + ":latest", "litellm_params": params, "model_info": info}
		created := h.ok("POST", "/model/new", admin, body).json()
		actual := created["model_info"].(map[string]any)
		id := actual["id"].(string)
		ids = append(ids, id)
		if actual["catalog_id"] != "qiniu" {
			t.Fatal("请求伪造了供应商目录")
		}
		reject := h.do("PATCH", "/model/"+id+"/update", admin, map[string]any{"model_info": map[string]any{"transport": "openai_video"}})
		if reject.status != 400 {
			t.Fatalf("错协议编辑未拒绝: %s", reject.describe())
		}
		result := h.ok("POST", tc.path, admin, map[string]any{"model": "catalog/" + tc.transport + ":latest", "prompt": "video", "content": []any{map[string]any{"type": "text", "text": "video"}}}).json()
		if result[tc.key] == nil {
			t.Fatalf("专用能力未执行: %v", result)
		}
	}
	credential["credential_info"] = map[string]any{"custom_llm_provider": "openai", "catalog_id": "volcengine"}
	h.ok("PATCH", "/credentials/friendly-video", admin, credential)
	before := calls.Load()
	reject := h.do("POST", "/v3/contents/generations/tasks", admin, map[string]any{"model": "catalog/qiniu_contents_generation:latest", "content": []any{map[string]any{"type": "text", "text": "video"}}})
	if reject.status == 200 || calls.Load() != before {
		t.Fatalf("目录变更仍执行旧供应商: %s", reject.describe())
	}
	// 未登记的中转目录保持显式 FAL 执行；错误型号仍被具体路径白名单拒绝。
	credential["credential_info"] = map[string]any{"custom_llm_provider": "openai", "catalog_id": "fennoai"}
	h.ok("PATCH", "/credentials/friendly-video", admin, credential)
	h.ok("POST", "/queue/bytedance/seedance-2.0/text-to-video", admin, map[string]any{"model": "catalog/qiniu_fal_doubao_20:latest", "prompt": "relay video"})
	if calls.Load() != before+1 {
		t.Fatal("中转连接未执行已配置的 FAL 路径")
	}
}
