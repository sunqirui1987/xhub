package regression

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// TestDialogueAliasLifecycle 验证真实管理路由支持带斜杠和冒号的公开名称，并保留上游 ID。
// 参数 t：回归上下文；返回无。前置隔离 PostgreSQL 和本地上游，覆盖创建、非法编辑、
// 合法编辑、持久化和 Google/OpenAI 数据面；显式删除部署和凭据，harness 清理账单及 schema。
func TestDialogueAliasLifecycle(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	h.ok("POST", "/credentials", admin, map[string]any{
		"credential_name": "alias-supplier", "credential_info": map[string]any{"custom_llm_provider": "openai"},
		"credential_values": map[string]any{"api_base": h.prices.URL + "/v1", "api_key": "sk-fake"},
	})
	defer h.ok("DELETE", "/credentials/alias-supplier", admin, nil)
	body := map[string]any{
		"model_name":     "group/model:latest",
		"litellm_params": map[string]any{"model": "group/model:latest", "custom_llm_provider": "openai", "litellm_credential_name": "alias-supplier", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002},
		"model_info":     map[string]any{"transport": "bypass_openai_chat", "pricing_source": "manual"},
	}
	for _, name := range []string{"group//model", "group/../model", ".", "..", "/model", "model/"} {
		body["model_name"] = name
		got := h.do("POST", "/model/new", admin, body)
		if got.status != 400 || !strings.Contains(fmt.Sprint(got.json()), "对外模型名称") {
			t.Fatalf("非法路径名称 %q 未返回 400 中文说明：%s", name, got.describe())
		}
	}
	body["model_name"] = "group/model:latest"
	created := h.ok("POST", "/model/new", admin, body).json()
	id := stringField(created["model_info"].(map[string]any), "id")
	defer h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
	if created["litellm_params"].(map[string]any)["model"] != "group/model:latest" || created["model_name"] != "group/model:latest" {
		t.Fatal("创建改变了公开名称或上游模型 ID")
	}
	invalid := h.do("PATCH", "/model/"+id+"/update", admin, map[string]any{"model_name": "bad/../name"})
	if invalid.status != 400 || !strings.Contains(fmt.Sprint(invalid.json()), "对外模型名称") {
		t.Fatalf("非法编辑未返回中文原因：%s", invalid.describe())
	}
	for _, name := range []string{"group/model:latest", "next/group/model:version", "next/group/model：version"} {
		if name != "group/model:latest" {
			h.ok("PATCH", "/model/"+id+"/update", admin, map[string]any{"model_name": name})
		}
		stored := h.ok("GET", "/v2/model/info?modelId="+id, admin, nil).json()["data"].([]any)[0].(map[string]any)
		if stored["model_name"] != name {
			t.Fatalf("公开名称未持久化：%v", stored)
		}
		for _, endpoint := range []struct {
			path string
			body map[string]any
		}{
			{"/v1/chat/completions", map[string]any{"model": name, "messages": []any{map[string]any{"role": "user", "content": "hello"}}}},
			{"/v1beta/models/" + name + ":generateContent", map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}}}},
			{"/vertex/v1/models/" + url.PathEscape(name) + ":generateContent", map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}}}},
		} {
			result := h.ok("POST", endpoint.path, admin, endpoint.body).json()
			if !strings.Contains(fmt.Sprint(result), defaultReply.Content) {
				t.Fatalf("路径别名调用失败 %s：%v", endpoint.path, result)
			}
		}
	}
}
