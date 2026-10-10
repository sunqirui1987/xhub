package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestImagePlaygroundParameters 验证调试台图片正文经真实普通/Bypass网关到上游仍保留尺寸、数量及扩展字段。
// 参数 t 为测试上下文；前置独立 PostgreSQL、本地严格图片渠道；覆盖400透传、模型替换、鉴权及日志用量。
// 返回无；本地服务 defer 关闭，账单与部署随 harness 私有 schema 清理，不使用外部凭据。
func TestImagePlaygroundParameters(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var doc map[string]any
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.URL.Path != "/v1/images/generations" || doc["model"] != "real-image" || r.Header.Get("Authorization") != "Bearer local-key" {
			t.Errorf("图片路径、模型替换或上游鉴权错误: %s %+v", r.URL.Path, doc)
		}
		if doc["size"] != "1024x1024" {
			w.WriteHeader(400)
			writeJSON(w, map[string]any{"error": map[string]any{"code": "invalid_request", "message": "image size is not available for this channel"}})
			return
		}
		if doc["n"] != float64(1) || doc["seed"] != float64(42) || doc["prompt"] != "hello" {
			t.Errorf("图片原生参数丢失: %+v", doc)
		}
		writeJSON(w, map[string]any{"data": []any{map[string]any{"b64_json": "AAAA"}}, "usage": map[string]any{"input_tokens": 8, "output_tokens": 2}})
	}))
	defer up.Close()
	for _, tc := range []struct{ transport, path string }{
		{"openai_image_generation", "/v1/images/generations"},
		{"bypass_openai_image_generation", "/bypass/openai/v1/images/generations"},
	} {
		t.Run(tc.transport, func(t *testing.T) {
			endpointType := "image_generation"
			if tc.transport == "bypass_openai_image_generation" {
				endpointType = "bypass:openai-images"
			}
			h := newHarness(t, config.ModelEntry{ModelName: "image-playground", LiteLLMParams: map[string]any{"model": "real-image", "custom_llm_provider": "openai", "api_base": up.URL, "api_key": "local-key", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}, ModelInfo: map[string]any{"transport": tc.transport, "endpoint_types": []string{endpointType}}})
			admin := h.adminSession()
			body := map[string]any{"model": "image-playground", "prompt": "hello", "n": 1, "seed": 42}
			if denied := h.do("POST", tc.path, "", body); denied.status != 401 {
				t.Fatalf("缺失网关鉴权未拒绝: %s", denied.describe())
			}
			for _, size := range []any{nil, "", "2048x2048"} {
				body["size"] = size
				failed := h.do("POST", tc.path, admin, body)
				if failed.status != 400 {
					t.Fatalf("渠道非法尺寸错误未保留: %s", failed.describe())
				}
			}
			body["size"] = "1024x1024"
			success := h.ok("POST", tc.path, admin, body)
			if len(success.json()["data"].([]any)) != 1 {
				t.Fatal("图片响应契约丢失")
			}
			log := h.ok("GET", "/spend/logs/ui/"+success.header("x-litellm-call-id"), admin, nil).json()
			if log["model"] != "image-playground" || !nearlyEqual(numberOrZero(log["spend"]), 0.000012) {
				t.Fatalf("图片日志或计量错误: %+v", log)
			}
		})
	}
}
