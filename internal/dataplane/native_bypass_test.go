package dataplane

import (
	"bytes"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNativeBypassJSONAndUsage 验证原生路径、协议鉴权、模型替换、数字精度和用量合并，并阻止客户端凭据泄露。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestNativeBypassJSONAndUsage(t *testing.T) {
	for _, spec := range []struct{ transport, path, upstream, protocol, response string }{
		{"bypass_openai_responses", "/bypass/openai/v1/responses", "/v1/responses", "openai", `{"id":"r1","usage":{"input_tokens":100,"output_tokens":4,"input_tokens_details":{"cached_tokens":25}}}`},
		{"bypass_anthropic_messages", "/bypass/anthropic/v1/messages", "/v1/messages", "anthropic", `{"id":"m1","usage":{"input_tokens":20,"output_tokens":4,"cache_read_input_tokens":30,"cache_creation_input_tokens":10}}`},
	} {
		t.Run(spec.transport, func(t *testing.T) {
			hits := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if r.URL.Path != spec.upstream || r.URL.RawQuery != "beta=1" {
					t.Error(r.URL)
				}
				if spec.protocol == "anthropic" {
					if r.Header.Get("X-Api-Key") != "supplier-key" || r.Header.Get("Authorization") != "" || r.Header.Get("Anthropic-Version") != "2023-06-01" || r.Header.Get("Anthropic-Beta") != "test-beta" {
						t.Error("native auth/headers incorrect")
					}
				} else if r.Header.Get("Authorization") != "Bearer supplier-key" || r.Header.Get("X-Api-Key") != "" {
					t.Error("native auth incorrect")
				}
				if r.Header.Get("Cookie") != "" || r.Header.Get("X-Hop") != "" {
					t.Error("gateway credentials/hop headers leaked")
				}
				var doc map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&doc)
				if string(doc["model"]) != `"upstream"` || string(doc["opaque"]) != `9007199254740993` || !bytes.Contains(doc["custom"], []byte("keep")) {
					t.Error("native fields modified", doc)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "upstream-id")
				io.WriteString(w, spec.response)
			}))
			defer up.Close()
			h := officialHost(up, deployment("public", spec.protocol+"/upstream", "supplier-key", up.URL+"/v1", spec.transport, nil))
			req := httptest.NewRequest("POST", spec.path+"?beta=1", strings.NewReader(`{"model":"public","opaque":9007199254740993,"custom":{"keep":true}}`))
			req.Header.Set("Authorization", "Bearer gateway-key")
			req.Header.Set("X-Api-Key", "gateway-key")
			req.Header.Set("Cookie", "session=private")
			req.Header.Set("Connection", "X-Hop")
			req.Header.Set("X-Hop", "private")
			req.Header.Set("Anthropic-Beta", "test-beta")
			hit, _ := provider.Match("POST", req.URL.Path, nil)
			rec := httptest.NewRecorder()
			ServeBypass(h, rec, req, hit)
			if rec.Code != 200 || rec.Body.String() != spec.response || rec.Header().Get("X-Request-Id") != "upstream-id" || hits != 1 || len(h.spend) != 1 {
				t.Fatal(rec.Code, rec.Body.String(), hits)
			}
			usage := catalog.NormalizeUsage(h.spend[0].usage)
			if usage.CompletionTokens != 4 {
				t.Fatal(usage)
			}
			if spec.protocol == "anthropic" && (usage.PromptTokens != 50 || usage.CachedTokens != 30 || usage.CacheWriteTokens != 10) {
				t.Fatal(usage)
			}
			if spec.protocol == "openai" && (usage.PromptTokens != 100 || usage.CachedTokens != 25) {
				t.Fatal(usage)
			}
		})
	}
}

// TestNativeImageEditMultipart 验证图片编辑保留 multipart 边界、文件字节与字段，并正确记录图片规格和用量。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestNativeImageEditMultipart(t *testing.T) {
	var raw bytes.Buffer
	mw := multipart.NewWriter(&raw)
	mw.SetBoundary("native-boundary")
	mw.WriteField("model", "public")
	mw.WriteField("quality", "high")
	mw.WriteField("size", "1024x1024")
	image := []byte{0, 255, 1, 2, 13, 10, 3}
	part, _ := mw.CreateFormFile("image[]", "original.png")
	part.Write(image)
	mw.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" || !strings.Contains(r.Header.Get("Content-Type"), "native-boundary") {
			t.Error(r.URL, r.Header.Get("Content-Type"))
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		fields := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			data, _ := io.ReadAll(part)
			if part.FileName() != "" {
				if part.FileName() != "original.png" || part.FormName() != "image[]" || !bytes.Equal(data, image) {
					t.Error("file changed")
				}
			} else {
				fields[part.FormName()] = string(data)
			}
		}
		if fields["model"] != "gpt-image" || fields["quality"] != "high" || fields["size"] != "1024x1024" {
			t.Error(fields)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"b64_json":"AAAA"}],"usage":{"input_tokens":2,"output_tokens":3}}`)
	}))
	defer up.Close()
	h := officialHost(up, deployment("public", "openai/gpt-image", "supplier-key", up.URL, "bypass_openai_image_edit", nil))
	req := httptest.NewRequest("POST", "/bypass/openai/v1/images/edits", &raw)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	hit, _ := provider.Match("POST", req.URL.Path, nil)
	rec := httptest.NewRecorder()
	ServeBypass(h, rec, req, hit)
	if rec.Code != 200 || len(h.spend) != 1 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	u := catalog.NormalizeUsage(h.spend[0].usage)
	if u.Images != 1 || u.ImageVariant != "high_1024x1024" || u.OutputVariant != "" || u.CompletionTokens != 3 {
		t.Fatal(u)
	}
}

// TestNativeBypassStreamingFlushAndMergedUsage 验证 SSE 首个事件立即送达，保留原始换行，并合并 Anthropic 分散报告的缓存和 token 事实。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestNativeBypassStreamingFlushAndMergedUsage(t *testing.T) {
	first := "event: message_start\r\ndata: {\"message\":{\"usage\":{\"input_tokens\":20,\"output_tokens\":0,\"cache_read_input_tokens\":30}}}\r\n\r\n"
	last := "event: message_delta\ndata: {\"usage\":{\"output_tokens\":4}}\n\nevent: message_stop\ndata: {}\n\n"
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, last)
	}))
	defer up.Close()
	h := officialHost(up, deployment("public", "anthropic/claude", "supplier-key", up.URL, "bypass_anthropic_messages", nil))
	done := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		hit, _ := provider.Match("POST", r.URL.Path, nil)
		ServeBypass(h, w, r, hit)
	}))
	defer gateway.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(gateway.URL+"/bypass/anthropic/v1/messages", "application/json", strings.NewReader(`{"model":"public","stream":true}`))
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	head := make([]byte, len(first))
	_, err = io.ReadFull(resp.Body, head)
	close(release)
	if err != nil || string(head) != first {
		t.Fatal("first event did not flush", err)
	}
	tail, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	<-done
	if err != nil || string(tail) != last || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(err, string(tail))
	}
	if len(h.spend) != 1 {
		t.Fatal(h.spend)
	}
	u := catalog.NormalizeUsage(h.spend[0].usage)
	if u.PromptTokens != 50 || u.CachedTokens != 30 || u.CompletionTokens != 4 {
		t.Fatal(u)
	}
}

// TestNativeCreateServerErrorIsNotReplayed 验证创建操作返回 5xx 后不会因路由重试设置生成第二个付费任务。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestNativeCreateServerErrorIsNotReplayed(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		io.WriteString(w, `{"error":{"message":"busy"}}`)
	}))
	defer up.Close()
	h := officialHost(up, deployment("public", "openai/gpt", "key", up.URL, "bypass_openai_responses", nil))
	h.settings = map[string]any{"num_retries": 3}
	rec := h.call(t, "POST", "/bypass/openai/v1/responses", `{"model":"public"}`)
	if rec.Code != 503 || calls != 1 || h.spend[0].usage != nil {
		t.Fatal(rec.Code, calls, h.spend)
	}
}

// TestQiniuNativeNamespace 验证七牛根地址扩展到原生协议路径，并保留 openai/ 模型命名空间。
// 参数 t：测试上下文。返回：无。只调用本地假上游，不创建付费任务。
func TestQiniuNativeNamespace(t *testing.T) {
	for _, suffix := range []string{"", "/v1", "/v1/", "/bypass/openai/v1"} {
		t.Run(suffix, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/bypass/openai/v1/responses" {
					t.Error(r.URL.Path)
				}
				var doc map[string]any
				json.NewDecoder(r.Body).Decode(&doc)
				if doc["model"] != "openai/gpt-test" {
					t.Error(doc["model"])
				}
				io.WriteString(w, `{"status":"completed","usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer up.Close()
			dep := deployment("public", "openai/gpt-test", "supplier-key", up.URL+suffix, "bypass_openai_responses", nil)
			dep.LiteLLMParams["custom_llm_provider"] = "qiniu"
			h := officialHost(up, dep)
			req := httptest.NewRequest("POST", "/bypass/openai/v1/responses", strings.NewReader(`{"model":"public","input":"hello"}`))
			hit, _ := provider.Match("POST", req.URL.Path, nil)
			rec := httptest.NewRecorder()
			ServeBypass(h, rec, req, hit)
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
		})
	}
}

// TestModelinkNativeForward 验证按 OpenAI 配置的 Modelink 凭据实际转发到七牛原生路径。
// 参数 t：Go 测试上下文。返回：无；检查目标地址、模型与响应，失败时报告回归。
// 调用：go test；用本地假上游拦截请求，不访问供应商或产生费用。
func TestModelinkNativeForward(t *testing.T) {
	const response = `{"status":"completed","output":[{"content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bypass/openai/v1/responses" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != "glm-4.5" || body["input"] != "hello" {
			t.Errorf("unexpected upstream body: %v, error: %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, response)
	}))
	defer up.Close()
	h := officialHost(up, deployment("public", "glm-4.5", "supplier-key", "https://api.modelink.ai/v1", "bypass_openai_responses", nil))
	localTransport := up.Client().Transport
	h.client = &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.modelink.ai/bypass/openai/v1/responses" {
			t.Errorf("unexpected Modelink URL: %s", r.URL)
		}
		// 拦截最终供应商地址后只替换主机，保持路径和请求体交给本地上游验证。
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(up.URL, "http://")
		return localTransport.RoundTrip(r)
	})}
	rec := h.call(t, http.MethodPost, "/bypass/openai/v1/responses", `{"model":"public","input":"hello"}`)
	if rec.Code != http.StatusOK || rec.Body.String() != response || len(h.spend) != 1 {
		t.Fatalf("forwarded response: %d %s, spend count: %d", rec.Code, rec.Body.String(), len(h.spend))
	}
}

// TestNativeSupplierURLs 验证兼容 OpenAI 的七牛 Qnaigc、Modelink 凭据仍使用七牛原生路径，其他供应商保留自己的路径。
// 参数 t：测试上下文。返回：无；只组合本地声明，不访问供应商。
func TestNativeSupplierURLs(t *testing.T) {
	for _, spec := range []struct {
		base, path, wantURL, model, wantModel string
	}{
		{"https://api.qnaigc.com/v1", "/bypass/openai/v1/responses", "https://api.qnaigc.com/bypass/openai/v1/responses", "openai/gpt-test", "openai/gpt-test"},
		{"https://api.qnaigc.com", "/bypass/openai/v1/responses", "https://api.qnaigc.com/bypass/openai/v1/responses", "glm-4.5", "glm-4.5"},
		{"https://api.qnaigc.com/bypass/openai/v1", "/bypass/openai/v1/responses", "https://api.qnaigc.com/bypass/openai/v1/responses", "openai/gpt-test", "openai/gpt-test"},
		{"https://api.modelink.ai/v1", "/bypass/openai/v1/responses", "https://api.modelink.ai/bypass/openai/v1/responses", "glm-4.5", "glm-4.5"},
		{"https://api.modelink.ai", "/bypass/openai/v1/responses", "https://api.modelink.ai/bypass/openai/v1/responses", "openai/gpt-test", "openai/gpt-test"},
		{"https://api.modelink.ai/v1/", "/bypass/openai/v1/responses", "https://api.modelink.ai/bypass/openai/v1/responses", "qiniu/openai/gpt-test", "openai/gpt-test"},
		{"https://api.modelink.ai/bypass/openai/v1", "/bypass/openai/v1/responses", "https://api.modelink.ai/bypass/openai/v1/responses", "openai/gpt-test", "openai/gpt-test"},
		{"https://API.MODELINK.AI/v1", "/bypass/openai/v1/responses", "https://API.MODELINK.AI/bypass/openai/v1/responses", "openai/gpt-test", "openai/gpt-test"},
		{"https://api.modelink.ai/v1", "/bypass/anthropic/v1/messages", "https://api.modelink.ai/bypass/anthropic/v1/messages", "anthropic/claude-test", "anthropic/claude-test"},
		{"https://api.modelink.ai/v1", "/bypass/openai/v1/images/generations", "https://api.modelink.ai/bypass/openai/v1/images/generations", "openai/gpt-image", "openai/gpt-image"},
		{"https://api.modelink.ai/v1", "/bypass/openai/v1/images/edits", "https://api.modelink.ai/bypass/openai/v1/images/edits", "openai/gpt-image", "openai/gpt-image"},
		{"https://api.qnaigc.com/v1", "/bypass/anthropic/v1/messages", "https://api.qnaigc.com/bypass/anthropic/v1/messages", "anthropic/claude-test", "anthropic/claude-test"},
		{"https://api.qnaigc.com/v1", "/bypass/openai/v1/images/generations", "https://api.qnaigc.com/bypass/openai/v1/images/generations", "openai/gpt-image", "openai/gpt-image"},
		{"https://api.openai.com/v1", "/bypass/openai/v1/responses", "https://api.openai.com/v1/responses", "openai/gpt-test", "gpt-test"},
		{"https://relay.example/custom/v1", "/bypass/openai/v1/responses", "https://relay.example/custom/v1/responses", "openai/gpt-test", "gpt-test"},
		{"https://api.qnaigc.com.example/v1", "/bypass/openai/v1/responses", "https://api.qnaigc.com.example/v1/responses", "openai/gpt-test", "gpt-test"},
		{"https://api.modelink.ai.example/v1", "/bypass/openai/v1/responses", "https://api.modelink.ai.example/v1/responses", "openai/gpt-test", "gpt-test"},
	} {
		t.Run(spec.wantURL, func(t *testing.T) {
			hit, ok := provider.Match("POST", spec.path, nil)
			if !ok {
				t.Fatal("unregistered native path", spec.path)
			}
			dep := config.ModelEntry{LiteLLMParams: map[string]any{"custom_llm_provider": "openai"}}
			if got := bypassDeploymentURL(spec.base, hit, dep); got != spec.wantURL {
				t.Errorf("URL = %s, want %s", got, spec.wantURL)
			}
			if got := bypassUpstreamModel(hit.Transport, spec.model, bypassSupplier(spec.base, dep)); got != spec.wantModel {
				t.Errorf("model = %s, want %s", got, spec.wantModel)
			}
		})
	}
}

// TestNativeMultipleImageEventsRemainUnpriced 验证多张流式图片不会按最后一张的规格错误结算。
// 参数 t：Go 测试上下文。返回：无；同时断言原始流保真、完成图片数和待核价原因。
// 调用：go test；使用本地假上游，不消耗图片生成额度。
func TestNativeMultipleImageEventsRemainUnpriced(t *testing.T) {
	const events = "event: image_generation.completed\ndata: {\"quality\":\"high\",\"size\":\"1024x1024\",\"usage\":{\"output_tokens\":3}}\n\nevent: image_generation.completed\ndata: {\"quality\":\"low\",\"size\":\"1536x1024\",\"usage\":{\"output_tokens\":5}}\n\n"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, events)
	}))
	defer up.Close()
	h := officialHost(up, deployment("public", "openai/gpt-image", "supplier-key", up.URL, "bypass_openai_image_generation", nil))
	rec := h.call(t, "POST", "/bypass/openai/v1/images/generations", `{"model":"public","stream":true}`)
	if rec.Code != http.StatusOK || rec.Body.String() != events || len(h.spend) != 1 {
		t.Fatal("image stream was changed or not recorded", rec.Code, len(h.spend))
	}
	usage := catalog.NormalizeUsage(h.spend[0].usage)
	if usage.Images != 2 || usage.PricingBlocked != "multiple_image_events_require_itemized_pricing" {
		t.Fatal("multiple variants incorrectly became a single priced bill", usage)
	}
}
