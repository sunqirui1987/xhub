package dataplane

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/provider"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIVideoNativeLifecycle 验证标准视频创建、轮询、内容下载和任务隔离的真实上游流程。
// 参数 t：测试上下文；前置内存部署与本地原生服务；返回无，等待/失败不计费，完成事件保持唯一结算标识，服务自动关闭。
func TestOpenAIVideoNativeLifecycle(t *testing.T) {
	status := "queued"
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer saved-key" {
			t.Error("视频原生请求未使用供应商凭据")
		}
		if r.Method == "POST" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != "sora-real" || body["prompt"] != "hello" {
				t.Errorf("视频原生创建参数错误: %v %v", body, err)
			}
			io.WriteString(w, `{"id":"video-1","status":"queued"}`)
		} else if r.URL.Path == "/v1/videos/video-1/content" {
			w.Header().Set("Content-Type", "video/mp4")
			io.WriteString(w, "video-content")
		} else if r.URL.Path == "/v1/videos/video-1" {
			json.NewEncoder(w).Encode(map[string]any{"id": "video-1", "status": status, "seconds": "8"})
		} else {
			t.Errorf("错误视频路径 %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	h := officialHost(up, deployment("video-alias", "sora-real", "saved-key", up.URL, "openai_videos", nil))
	if r := h.call(t, "POST", "/v1/videos", `{"model":"video-alias","prompt":"hello"}`); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, state := range []string{"queued", "failed", "completed", "completed"} {
		status = state
		if r := h.call(t, "GET", "/v1/videos/video-1", ""); r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
		row, note := h.spend[len(h.spend)-1], h.notes[len(h.notes)-1]
		if state != "completed" && (row.usage != nil || note.SettlementID != "") {
			t.Fatalf("未成功视频产生账单: %v", state)
		}
		if state == "completed" && (catalog.NormalizeUsage(row.usage).Seconds != 8 || note.SettlementID == "") {
			t.Fatal("完成视频缺少实测时长或结算标识")
		}
	}
	if h.notes[len(h.notes)-1].SettlementID != h.notes[len(h.notes)-2].SettlementID {
		t.Fatal("重复轮询未共享持久结算标识")
	}
	if r := h.call(t, "GET", "/v1/videos/video-1/content", ""); r.Code != 200 || r.Body.String() != "video-content" || r.Header().Get("Content-Type") != "video/mp4" {
		t.Fatal("视频下载改变内容", r.Code, r.Body.String())
	}
	before := calls
	if r := h.call(t, "GET", "/v1/videos/unknown", ""); r.Code != 404 || calls != before {
		t.Fatal("未知任务访问上游")
	}
	if h.OfficialDeployment(officialTaskScope(&auth.Principal{UserID: "other"}, "openai_videos", "video-1")) != "" {
		t.Fatal("视频任务跨用户共享")
	}
}

// TestGoogleNativeForward 验证路径别名、模型替换、操作保留与凭据隔离；参数 t 为上下文，前置本地上游和内存部署，服务关闭清理。
func TestGoogleNativeForward(t *testing.T) {
	for _, p := range []string{"gemini", "vertex"} {
		root, upRoot := "/v1beta/models/", "/v1beta/models/"
		if p == "vertex" {
			root, upRoot = "/vertex/v1/models/", "/models/"
		}
		for _, op := range []string{"generateContent", "streamGenerateContent", "countTokens"} {
			t.Run(p+"/"+op, func(t *testing.T) {
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != upRoot+"real:"+op || r.URL.Query().Get("key") != "" || r.URL.Query().Get("api_key") != "" || r.URL.Query().Get("access_token") != "" || r.URL.Query().Get("alt") != "sse" {
						t.Errorf("路径或凭据错误: %s", r.URL)
					}
					if p == "gemini" && (r.Header.Get("x-goog-api-key") != "supplier-key" || r.Header.Get("Authorization") != "") {
						t.Error("Gemini 鉴权错配")
					}
					if p == "vertex" && (r.Header.Get("Authorization") != "Bearer supplier-key" || r.Header.Get("x-goog-api-key") != "") {
						t.Error("Vertex 鉴权错配")
					}
					var d map[string]json.RawMessage
					json.NewDecoder(r.Body).Decode(&d)
					if d["model"] != nil || string(d["opaque"]) != "9007199254740993" || !strings.Contains(string(d["contents"]), "hello") {
						t.Errorf("正文被转换: %v", d)
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}}`)
				}))
				defer up.Close()
				h := officialHost(up, deployment("alias", "real", "supplier-key", up.URL, p+"_generate_content", nil))
				r := httptest.NewRequest("POST", root+"alias:"+op+"?key=gateway-key&api_key=private&access_token=private&alt=sse", strings.NewReader(`{"model":"ignored","contents":[{"parts":[{"text":"hello"}]}],"opaque":9007199254740993}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("x-goog-api-key", "gateway-key")
				hit, _ := provider.Match("POST", r.URL.Path, nil)
				rec := httptest.NewRecorder()
				ServeBypass(h, rec, r, hit)
				if rec.Code != 200 || len(h.spend) != 1 {
					t.Fatalf("原生转发失败: %d %s", rec.Code, rec.Body.String())
				}
				if denied := h.call(t, "POST", root+"missing:"+op, `{"contents":[]}`); denied.Code == 200 {
					t.Fatal("未声明模型被调用")
				}
			})
		}
	}
}

// TestNativeURLParameterEncoding 验证模型斜杠与空格编码；参数 t 为上下文，固定路径纯函数无需清理。
func TestNativeURLParameterEncoding(t *testing.T) {
	hit := provider.Hit{Action: provider.Action{UpstreamPath: "/models/{model}:generateContent"}, Names: map[string]string{"model": "real/name with space"}}
	got := bypassDeploymentURL("https://example.test/projects/p", hit, deployment("a", "m", "k", "", "gemini_generate_content", nil))
	if got != "https://example.test/projects/p/models/real%2Fname%20with%20space:generateContent" {
		t.Fatal(got)
	}
	if got := bypassURL("https://example.test/root/v1", "/v1/images/edits"); got != "https://example.test/root/v1/images/edits" {
		t.Fatal(got)
	}
}
