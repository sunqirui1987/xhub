package regression

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestModelDiscoveryPathFallback 验证真实管理路由遵循已存供应商地址并双向适配 /v1。
// 参数 t：测试上下文；返回：无。前置为真实 PostgreSQL 与本地上游；核对目录、错误、
// 鉴权和请求顺序，拒绝请求中的连接覆盖；每个子用例的隔离 schema 随 harness 删除。
func TestModelDiscoveryPathFallback(t *testing.T) {
	for _, tc := range []struct {
		name, base, working string
		status              int
		paths               []string
	}{
		{"root direct", "", "/models", 404, []string{"/models"}},
		{"root needs v1", "", "/v1/models", 404, []string{"/models", "/v1/models"}},
		{"v1 needs root", "/v1", "/models", 404, []string{"/v1/models", "/models"}},
		{"custom prefix", "/relay", "/relay/v1/models", 404, []string{"/relay/models", "/relay/v1/models"}},
		{"explicit models URL", "/v1/models/", "/v1/models", 404, []string{"/v1/models"}},
		{"both missing", "", "", 404, []string{"/models", "/v1/models"}},
		{"unauthorized", "", "", 401, []string{"/models"}},
		{"rate limited", "", "", 429, []string{"/models"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			admin := h.adminSession()
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer saved-secret" {
					t.Error("目录鉴权没有使用已存凭据")
				}
				if r.URL.Path != tc.working {
					w.WriteHeader(tc.status)
					return
				}
				writeJSON(w, map[string]any{"data": []any{map[string]any{"id": "supplier-chat"}}})
			}))
			defer upstream.Close()
			h.ok(http.MethodPost, "/credentials", admin, map[string]any{
				"credential_name": "path-supplier", "credential_info": map[string]any{"custom_llm_provider": "openai"},
				"credential_values": map[string]any{"api_key": "saved-secret", "api_base": upstream.URL + tc.base},
			})
			got := h.ok(http.MethodPost, "/model/builtin/models", admin, map[string]any{
				"credential_name": "path-supplier", "provider": "qiniu", "api_key": "wrong-secret", "api_base": "https://unused.invalid",
			}).json()
			if !reflect.DeepEqual(paths, tc.paths) {
				t.Fatalf("目录回退顺序错误：%v want=%v", paths, tc.paths)
			}
			if got["api_base"] != upstream.URL+tc.base || got["credential_name"] != "path-supplier" {
				t.Fatalf("目录来源被覆盖：%v", got)
			}
			if strings.Contains(fmt.Sprint(got), "secret") {
				t.Fatal("目录结果泄露凭据")
			}
			models := listField(got, "models")
			if tc.working != "" {
				if got["error"] != nil || len(models) != 1 || stringField(models[0], "id") != "supplier-chat" {
					t.Fatalf("供应商目录错误：%v", got)
				}
			} else if len(models) != 0 || !strings.Contains(fmt.Sprint(got["error"]), fmt.Sprint(tc.status)) {
				t.Fatalf("目录失败未返回空列表和原始状态：%v", got)
			}
		})
	}
}

// TestQiniuDiscoveryIncludesRegisteredFalModels 验证管理 API 合并供应商目录与已登记原生模型，
// 通过三种七牛身份声明及普通 OpenAI 凭据检查去重和供应商隔离。
// 参数 t：当前测试；返回：无。使用真实网关和 PostgreSQL、本地目录，不产生付费调用。
func TestQiniuDiscoveryIncludesRegisteredFalModels(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	const kling = "fal-ai/kling-video/v3/pro/text-to-video"
	const vidu = "fal-ai/vidu/q3/text-to-video/pro"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected discovery path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]any{"data": []any{map[string]any{"id": "glm-4.5"}, map[string]any{"id": kling}}})
	}))
	defer upstream.Close()
	for _, supplier := range []struct {
		name, builtin, protocol string
		native                  bool
	}{
		{"qiniu-discovery", "qiniu", "openai", true},
		{"qiniu", "", "openai", true},
		{"qiniu-protocol", "", "qiniu", true},
		{"ordinary-openai", "", "openai", false},
	} {
		h.ok(http.MethodPost, "/credentials", admin, map[string]any{
			"credential_name":   supplier.name,
			"credential_info":   map[string]any{"custom_llm_provider": supplier.protocol, "builtin": supplier.builtin},
			"credential_values": map[string]any{"custom_llm_provider": supplier.protocol, "api_key": "sk-fake", "api_base": upstream.URL + "/v1"},
		})
		got := h.ok(http.MethodPost, "/model/builtin/models", admin, map[string]any{"credential_name": supplier.name}).json()
		counts := map[string]int{}
		for _, row := range listField(got, "models") {
			counts[stringField(row, "id")]++
		}
		if counts["glm-4.5"] != 1 || counts[kling] != 1 {
			t.Fatal("discovery omitted or duplicated upstream models")
		}
		if !supplier.native {
			if counts[vidu] != 0 || len(counts) != 2 {
				t.Fatal("unrelated supplier inherited Qiniu models")
			}
			continue
		}
		for _, model := range []string{vidu, "fal-ai/veo3.1", "minimax/h3-max/text-to-video", "bytedance/seedance-2.0/text-to-video"} {
			if counts[model] != 1 {
				t.Fatalf("missing or duplicated native model %s", model)
			}
		}
		for model := range counts {
			if strings.HasPrefix(model, "qiniu/") {
				t.Fatal("supplier prefix leaked into upstream model ID")
			}
		}
		if got["error"] != nil {
			t.Fatal("successful discovery returned an error")
		}
	}
}
