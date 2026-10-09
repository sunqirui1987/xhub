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

// TestDiscoveryTreatsProviderNamesUniformly 验证供应商名称和 builtin 元数据不会改变普通 Custom 凭据的行为。
// 参数 t：测试上下文；返回：无。真实数据库与本地目录返回两条模型，名称为 qiniu、fennoai 或普通名称时结果一致；schema 自动清理。
func TestDiscoveryTreatsProviderNamesUniformly(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("目录路径错误: %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"data": []any{map[string]any{"id": "supplier-chat"}, map[string]any{"id": "supplier-video"}}})
	}))
	defer upstream.Close()
	for _, name := range []string{"qiniu", "fennoai", "ordinary-openai"} {
		h.ok(http.MethodPost, "/credentials", admin, map[string]any{
			"credential_name": name, "credential_info": map[string]any{"custom_llm_provider": "custom_openai", "provider_id": "CUSTOM_OPENAI", "builtin": name},
			"credential_values": map[string]any{"api_key": "sk-fake", "api_base": upstream.URL + "/v1"},
		})
		got := h.ok(http.MethodPost, "/model/builtin/models", admin, map[string]any{"credential_name": name}).json()
		models := listField(got, "models")
		if len(models) != 2 || models[0]["id"] != "supplier-chat" || models[1]["id"] != "supplier-video" {
			t.Fatalf("供应商名称 %s 改变了普通目录行为: %v", name, got)
		}
	}
}
