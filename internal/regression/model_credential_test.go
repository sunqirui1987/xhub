package regression

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOfficialCredentialSupportsNativeModel 验证官方 Ark 与 Custom 七牛兼容凭据创建、编辑、调用和删除。
// 参数 t：回归上下文；返回：无。前置真实数据库和本地上游，检查通用协议错误不污染持久化、
// Bearer 鉴权、名为 qiniu 的 Custom 凭据显式选择七牛传输且不依赖名称或主机名；显式删除模型与凭据，harness 清理隔离 schema。
func TestOfficialCredentialSupportsNativeModel(t *testing.T) {
	const path = "/api/v3/contents/generations/tasks"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-secret" {
			t.Error("Ark 未使用已保存 Bearer 密钥")
			w.WriteHeader(401)
			return
		}
		normalizedPath := r.URL.Path
		if strings.HasPrefix(normalizedPath, "/v3/") {
			normalizedPath = "/api" + normalizedPath
		}
		switch r.Method + " " + normalizedPath {
		case "POST " + path:
			writeJSON(w, map[string]any{"id": "official-task"})
		case "GET " + path + "/official-task":
			writeJSON(w, map[string]any{"id": "official-task", "status": "succeeded", "usage": map[string]any{"completion_tokens": 100}})
		default:
			t.Errorf("Ark 路径错误: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "official-credential")
	for _, protocol := range []string{"volcengine", "openai"} {
		h.ok(http.MethodPost, "/credentials", admin, map[string]any{"credential_name": protocol, "credential_info": map[string]any{"custom_llm_provider": protocol}, "credential_values": map[string]any{"api_base": up.URL, "api_key": "saved-secret"}})
	}
	params := map[string]any{"model": "volcengine/doubao-seedance-2-0-260128", "custom_llm_provider": "volcengine", "litellm_credential_name": "volcengine", "output_cost_per_token": 0.00001}
	info := map[string]any{"transport": "ark_contents_generation", "endpoint_types": []string{"bypass:ark-video"}, "pricing_source": "manual", "billing_mode": "token"}
	created := h.ok(http.MethodPost, "/model/new", admin, map[string]any{"model_name": "official-model", "litellm_params": params, "model_info": info}).json()
	id := stringField(created["model_info"].(map[string]any), "id")
	h.ok(http.MethodPatch, "/model/"+id+"/update", admin, map[string]any{"model_name": "official-model-edited", "litellm_params": params, "model_info": info})
	params["litellm_credential_name"] = "openai"
	r := h.do(http.MethodPatch, "/model/"+id+"/update", admin, map[string]any{"litellm_params": params})
	if r.status != 400 || !strings.Contains(errorMessage(r), "protocol") {
		t.Fatalf("协议错误编辑未拒绝: %s", r.describe())
	}
	rows := listField(h.ok(http.MethodGet, "/v2/model/info?modelId="+id, admin, nil).json(), "data")
	if len(rows) != 1 || rows[0]["litellm_params"].(map[string]any)["litellm_credential_name"] != "volcengine" {
		t.Fatalf("错误编辑污染模型: %v", rows)
	}
	before := h.moneyOf(t, owner)
	h.ok(http.MethodPost, path, owner.key, map[string]any{"model": "official-model-edited", "content": []any{map[string]any{"type": "text", "text": "apple"}}})
	for i := 0; i < 2; i++ {
		h.ok(http.MethodGet, path+"/official-task", owner.key, nil)
		if !h.moneyOf(t, owner).grewBy(before, 0.001) {
			t.Fatal("任务未按实测100 token结算或重复扣费")
		}
	}
	const customCredential = "qiniu"
	h.ok(http.MethodPost, "/credentials", admin, map[string]any{
		"credential_name":   customCredential,
		"credential_info":   map[string]any{"custom_llm_provider": "custom", "provider_id": "CUSTOM"},
		"credential_values": map[string]any{"api_base": up.URL, "api_key": "saved-secret"},
	})
	customParams := map[string]any{
		"model": "qiniu/bytedance/doubao-seedance-2-0-260128", "custom_llm_provider": "custom",
		"litellm_credential_name": customCredential, "output_cost_per_token": 0.00001,
	}
	customInfo := map[string]any{"transport": "qiniu_contents_generation", "endpoint_types": []string{"bypass:ark-video"}, "pricing_source": "manual", "billing_mode": "token"}
	customCreated := h.ok(http.MethodPost, "/model/new", admin, map[string]any{"model_name": "custom-qiniu-model", "litellm_params": customParams, "model_info": customInfo}).json()
	customID := stringField(customCreated["model_info"].(map[string]any), "id")
	createdTask := h.ok(http.MethodPost, "/v3/contents/generations/tasks", owner.key, map[string]any{"model": "custom-qiniu-model", "content": []any{map[string]any{"type": "text", "text": "custom apple"}}}).json()
	if stringField(createdTask, "id") != "official-task" {
		t.Fatalf("Custom 七牛传输未调用本地上游：%v", createdTask)
	}
	openAIParams := map[string]any{"model": customParams["model"], "custom_llm_provider": "openai", "litellm_credential_name": "openai"}
	rejected := h.do(http.MethodPost, "/model/new", admin, map[string]any{"model_name": "protocol-mismatch-model", "litellm_params": openAIParams, "model_info": customInfo})
	if rejected.status != 400 || !strings.Contains(errorMessage(rejected), "does not support provider") {
		t.Fatalf("凭据协议与显式传输不匹配时未拒绝：%s", rejected.describe())
	}
	h.ok(http.MethodPost, "/model/delete", admin, map[string]any{"id": customID})
	h.ok(http.MethodPost, "/model/delete", admin, map[string]any{"id": id})
	if contains(modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", owner.key, nil), "data")), "official-model-edited") {
		t.Fatal("删除后模型仍可见")
	}
	for _, name := range []string{"volcengine", "openai", customCredential} {
		h.ok(http.MethodDelete, "/credentials/"+name, admin, nil)
	}
}
