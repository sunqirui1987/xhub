package regression

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLegacyQiniuCredentialSupportsNativeModel 验证旧 OpenAI 协议七牛凭据的创建、编辑和原生队列调用。
// 参数 t：回归上下文；返回：无。前置为真实 PostgreSQL、网关及本地上游；核对持久化、
// Key 鉴权、结果和实测计费，同时拒绝普通 OpenAI/错误协议；显式删除模型，harness 清理隔离 schema。
func TestLegacyQiniuCredentialSupportsNativeModel(t *testing.T) {
	const model = "byteplus/seedance-2.0/text-to-video"
	const create = "/queue/" + model
	const result = "/queue/byteplus/seedance-2.0/requests/legacy-task"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Key saved-secret" {
			t.Error("原生 Fal 调用未使用凭据中的 Key 鉴权")
			w.WriteHeader(401)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "POST " + create:
			writeJSON(w, map[string]any{"request_id": "legacy-task", "response_url": "https://unused.invalid/result", "status_url": "https://unused.invalid/status"})
		case "GET " + result, "GET " + result + "/status":
			writeJSON(w, map[string]any{"status": "COMPLETED", "video": map[string]any{"url": "https://example.invalid/video.mp4"}, "usage": map[string]any{"completion_tokens": 100}, "resolution": "1080p"})
		default:
			t.Errorf("Fal 上游路径错误：%s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "legacy-qiniu")
	h.ok(http.MethodPost, "/credentials", admin, map[string]any{
		"credential_name":   "update-incompatible",
		"credential_info":   map[string]any{"custom_llm_provider": "openai"},
		"credential_values": map[string]any{"api_base": up.URL, "api_key": "saved-secret"},
	})
	for _, tc := range []struct {
		name, builtin, protocol string
		allowed                 bool
	}{
		{"legacy-qiniu", "qiniu", "openai", true},
		{"qiniu", "", "openai", true},
		{"ordinary-openai", "", "openai", false},
		{"incompatible-qiniu", "qiniu", "anthropic", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.ok(http.MethodPost, "/credentials", admin, map[string]any{
				"credential_name":   tc.name,
				"credential_info":   map[string]any{"builtin": tc.builtin, "custom_llm_provider": tc.protocol},
				"credential_values": map[string]any{"custom_llm_provider": tc.protocol, "api_base": up.URL, "api_key": "saved-secret"},
			})
			params := map[string]any{"model": model, "custom_llm_provider": "qiniu", "litellm_credential_name": tc.name}
			info := map[string]any{"transport": "qiniu_fal_dreamina_20", "endpoint_types": []string{"bypass:fal-video"}, "pricing_source": "catalog", "base_model": "byteplus/dreamina-seedance-2-0-260128"}
			name := "model-" + tc.name
			created := h.do(http.MethodPost, "/model/new", admin, map[string]any{"model_name": name, "litellm_params": params, "model_info": info})
			if !tc.allowed {
				if created.status != 400 || !strings.Contains(string(created.body), "model protocol does not match its provider") {
					t.Fatalf("不兼容供应商未被明确拒绝：%s", created.describe())
				}
				if contains(modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", owner.key, nil), "data")), name) {
					t.Fatal("被拒绝创建的模型仍进入可调用列表")
				}
				return
			}
			if created.status != 200 {
				t.Fatalf("旧七牛凭据无法保存原生模型：%s", created.describe())
			}
			id := stringField(created.json()["model_info"].(map[string]any), "id")
			h.ok(http.MethodPatch, "/model/"+id+"/update", admin, map[string]any{"model_name": name + "-edited", "litellm_params": params, "model_info": info})
			stored, err := h.gw.RecordStore().ListProxyModels()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range stored {
				if row.ModelName == name+"-edited" {
					found = row.Params["custom_llm_provider"] == "qiniu" && row.Params["litellm_credential_name"] == tc.name
				}
			}
			if !found {
				t.Fatal("编辑后的七牛协议及凭据关联未持久化")
			}
			before := h.moneyOf(t, owner)
			queued := h.ok(http.MethodPost, create, owner.key, map[string]any{"model": name + "-edited", "prompt": "apple", "resolution": "1080p"}).json()
			if queued["request_id"] != "legacy-task" || !strings.HasSuffix(stringField(queued, "response_url"), result) {
				t.Fatalf("队列结果或网关 URL 错误：%v", queued)
			}
			for i := 0; i < 2; i++ {
				response := h.ok(http.MethodGet, result, owner.key, nil)
				if response.json()["status"] != "COMPLETED" {
					t.Fatal("队列未返回完成结果")
				}
				if got := h.moneyOf(t, owner); !got.grewBy(before, 100*7.7e-6) {
					t.Fatalf("实测视频 token 结算错误或重复扣费：before=%+v after=%+v expected=%g", before, got, 100*7.7e-6)
				}
			}
			// 错误编辑既不能改变在线部署，也不能写入数据库。
			params["litellm_credential_name"] = "update-incompatible"
			rejected := h.do(http.MethodPatch, "/model/"+id+"/update", admin, map[string]any{"litellm_params": params})
			if rejected.status != 400 || !strings.Contains(string(rejected.body), "model protocol does not match its provider") {
				t.Fatalf("不兼容凭据编辑未拒绝：%s", rejected.describe())
			}
			persisted := h.ok(http.MethodGet, "/v2/model/info?modelId="+id, admin, nil).json()
			rows := listField(persisted, "data")
			if len(rows) != 1 || rows[0]["litellm_params"].(map[string]any)["litellm_credential_name"] != tc.name {
				t.Fatalf("错误编辑污染原有模型：%v", persisted)
			}
			savedCredential, err := h.gw.RecordStore().GetKV("credentials", tc.name)
			if err != nil || savedCredential["credential_info"].(map[string]any)["custom_llm_provider"] != tc.protocol {
				t.Fatal("兼容校验不应改写用户已有凭据")
			}
			h.ok(http.MethodGet, result, owner.key, nil)
			h.ok(http.MethodPost, "/model/delete", admin, map[string]any{"id": id})
			deleted, err := h.gw.RecordStore().ListProxyModels()
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range deleted {
				if row.ModelName == name+"-edited" {
					t.Fatal("已删除模型仍残留数据库")
				}
			}
			if contains(modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", owner.key, nil), "data")), name+"-edited") {
				t.Fatal("删除后模型仍在可调用列表")
			}
		})
	}
}
