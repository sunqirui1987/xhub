package regression

import (
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestEndpointAvailableContract 验证真实接口对缺失声明、同别名有效部署和未授权访问的响应。
// 参数 t 为测试上下文；前置隔离数据库与本地上游，验证端点并集和实际调用；harness 清理 schema。
func TestEndpointAvailableContract(t *testing.T) {
	invalid := config.ModelEntry{ModelName: "unbound", LiteLLMParams: map[string]any{"model": "old"}, ModelInfo: map[string]any{"mode": "chat"}}
	mixed := invalid
	mixed.ModelName = "mixed"
	valid := config.ModelEntry{ModelName: "mixed", LiteLLMParams: map[string]any{"model": "real"}, ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat", "responses"}}}
	h := newHarness(t, invalid, mixed, valid)
	admin := h.adminSession()
	rows := listField(h.ok("GET", "/model/available", admin, nil).json(), "data")
	if len(rows) != 2 {
		t.Fatalf("端点别名未合并: %+v", rows)
	}
	for _, row := range rows {
		endpoints := row["endpoints"].([]any)
		if row["id"] == "unbound" {
			if len(endpoints) != 0 || !strings.Contains(stringField(row, "unavailable_reason"), "transport") {
				t.Fatalf("无效配置未解释: %+v", row)
			}
		} else if len(endpoints) != 7 || row["unavailable_reason"] != nil {
			t.Fatalf("有效部署受无效部署影响: %+v", row)
		}
	}
	if denied := h.do("GET", "/model/available", "", nil); denied.status != 401 {
		t.Fatalf("未授权模型列表未拒绝: %s", denied.describe())
	}
	if denied := h.do("POST", "/v1/chat/completions", admin, map[string]any{"model": "unbound", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}); denied.status == 200 {
		t.Fatalf("旧配置仍被调用: %s", denied.describe())
	}
	h.ok("POST", "/v1/chat/completions", admin, map[string]any{"model": "mixed", "messages": []any{map[string]any{"role": "user", "content": "hello"}}})
}

// TestEndpointRedesignNativeLifecycle 验证原生目录、拒绝旧声明、持久化编辑、Google/图片实调与计量。
// 参数 t 为回归上下文；前置真实隔离数据库和本地上游，删除模型与凭据，harness 删除账单和 schema。
func TestEndpointRedesignNativeLifecycle(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&d)
		if r.URL.Query().Get("key") != "" || r.URL.Query().Get("access_token") != "" {
			t.Error("网关查询凭据泄露")
		}
		switch r.URL.Path {
		case "/v1beta/models/real-google:generateContent", "/models/real-google:generateContent":
			if d["model"] != nil || !strings.Contains(string(d["contents"]), "hello") {
				t.Errorf("Google 原生字段丢失: %v", d)
			}
			if strings.HasPrefix(r.URL.Path, "/v1beta/") {
				if r.Header.Get("x-goog-api-key") != "saved-secret" || r.Header.Get("Authorization") != "" {
					t.Error("Gemini 鉴权错误")
				}
			} else if r.Header.Get("Authorization") != "Bearer saved-secret" || r.Header.Get("x-goog-api-key") != "" {
				t.Error("Vertex 鉴权错误")
			}
			writeJSON(w, map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": "e2e-ok"}}}}}, "usageMetadata": map[string]any{"promptTokenCount": 8, "candidatesTokenCount": 2}})
		case "/v1/images/generations":
			if string(d["model"]) != `"real-image"` || string(d["opaque"]) != "9007199254740993" || r.Header.Get("Authorization") != "Bearer saved-secret" {
				t.Errorf("图片原生字段或鉴权错误: %v", d)
			}
			writeJSON(w, map[string]any{"data": []any{map[string]any{"b64_json": "AAAA"}}, "usage": map[string]any{"input_tokens": 8, "output_tokens": 2}})
		default:
			t.Errorf("未登记路径被调用: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "endpoint-redesign")
	h.ok("POST", "/credentials", admin, map[string]any{"credential_name": "endpoint-native", "credential_info": map[string]any{"custom_llm_provider": "custom"}, "credential_values": map[string]any{"api_base": up.URL, "api_key": "saved-secret"}})
	for _, tc := range []struct{ id, tr, model, path string }{{"gemini", "gemini_generate_content", "real-google", "/v1beta/models/"}, {"vertex", "vertex_generate_content", "real-google", "/vertex/v1/models/"}, {"image_generation", "openai_image_generation", "real-image", "/v1/images/generations"}} {
		t.Run(tc.id, func(t *testing.T) {
			name := "native-" + tc.id
			params := map[string]any{"model": tc.model, "custom_llm_provider": "custom", "litellm_credential_name": "endpoint-native", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}
			info := map[string]any{"transport": tc.tr, "endpoint_types": []string{tc.id}, "pricing_source": "manual"}
			body := map[string]any{"model_name": name, "litellm_params": params, "model_info": info}
			created := h.ok("POST", "/model/new", admin, body).json()
			id := stringField(created["model_info"].(map[string]any), "id")
			defer h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
			for _, old := range []string{"image", "bypass:fal-video"} {
				bad := map[string]any{"model_info": map[string]any{"transport": tc.tr, "endpoint_types": []string{old}}}
				r := h.do("PATCH", "/model/"+id+"/update", admin, bad)
				if r.status != 400 {
					t.Fatalf("旧端点编辑未拒绝: %s", r.describe())
				}
			}
			rows := listField(h.ok("GET", "/v2/model/info?modelId="+id, admin, nil).json(), "data")
			if len(rows) != 1 || stringField(rows[0]["model_info"].(map[string]any), "transport") != tc.tr {
				t.Fatalf("拒绝后配置被污染: %v", rows)
			}
			name += "-edited"
			h.ok("PATCH", "/model/"+id+"/update", admin, map[string]any{"model_name": name})
			path := tc.path
			request := map[string]any{"model": name, "prompt": "hello", "opaque": json.Number("9007199254740993")}
			if tc.id != "image_generation" {
				path += name + ":generateContent"
				request = map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "hello"}}}}}
			}
			before := h.moneyOf(t, owner)
			var r reply
			if tc.id == "image_generation" {
				r = h.do("POST", path, owner.key, request)
			} else {
				r = h.doHeaders("POST", path+"?key="+owner.key+"&access_token=private", "", request, map[string]string{"x-goog-api-key": owner.key})
			}
			if r.status != 200 {
				t.Fatalf("原生数据面失败: %s", r.describe())
			}
			if !h.moneyOf(t, owner).grewBy(before, 0.000012) {
				t.Fatal("原生用量未按8输入2输出计费")
			}
			if tc.id == "image_generation" {
				if denied := h.do("POST", "/v1/images/edits", owner.key, request); denied.status == 200 {
					t.Fatal("生图部署错误开放编辑图片")
				}
			}
			h.ok("PATCH", "/model/"+id+"/update", admin, map[string]any{"model_info": map[string]any{"disabled": true}})
			if denied := h.do("POST", path, owner.key, request); denied.status == 200 {
				t.Fatal("禁用模型仍可调用")
			}
		})
	}
	h.ok("DELETE", "/credentials/endpoint-native", admin, nil)
}

// TestEndpointDialogueMatrix 验证真实网关的五种上游协议自动公开五种客户协议，鉴权、正文、响应及计费保持一致。
// 参数 t 为回归上下文；前置隔离 PostgreSQL 和本地 HTTP 上游，不声明 endpoint_types，覆盖普通与流式实调，模型和账单随 schema 清理。
func TestEndpointDialogueMatrix(t *testing.T) {
	protocols := []string{"openai-chat", "openai-responses", "anthropic-messages", "gemini", "vertex"}
	transports := []string{"bypass_openai_chat", "bypass_openai_responses", "bypass_anthropic_messages", "gemini_generate_content", "vertex_generate_content"}
	for i, source := range protocols {
		t.Run(source, func(t *testing.T) {
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				expected := map[string]string{"openai-chat": "/v1/chat/completions", "openai-responses": "/v1/responses", "anthropic-messages": "/v1/messages", "gemini": "/v1beta/models/real:generateContent", "vertex": "/models/real:generateContent"}[source]
				stream := body["stream"] == true || strings.HasSuffix(r.URL.Path, ":streamGenerateContent")
				if stream && (source == "gemini" || source == "vertex") {
					expected = strings.Replace(expected, ":generateContent", ":streamGenerateContent", 1)
					if r.URL.Query().Get("alt") != "sse" {
						t.Error("Google 流式上游缺少 alt=sse")
					}
				}
				if r.URL.Path != expected {
					t.Errorf("上游路径错误：%s 应为 %s", r.URL.Path, expected)
				}
				if source == "gemini" {
					if r.Header.Get("x-goog-api-key") != "secret" || r.Header.Get("Authorization") != "" {
						t.Error("Gemini 上游鉴权错误")
					}
				} else if source == "anthropic-messages" {
					if r.Header.Get("x-api-key") != "secret" {
						t.Error("Claude 上游鉴权错误")
					}
				} else if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("上游 Bearer 错误")
				}
				if source == "gemini" || source == "vertex" {
					if body["model"] != nil {
						t.Error("Google 正文不应包含 model")
					}
					body["model"] = "real"
				}
				delete(body, "store")
				parsed, err := llm.ParseDialogue(source, body)
				if err != nil || len(parsed.Turns) != 1 || parsed.Turns[0].Text != "hello" {
					t.Errorf("上游内容错配：%+v %v", parsed, err)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					for _, frame := range endpointStreamFrames(source) {
						fmt.Fprintf(w, "data: %s\n\n", frame)
					}
					return
				}
				data, err := llm.EncodeDialogueResult(llm.DialogueResult{ID: "matrix", Text: "matrix-ok", Stop: "stop", Usage: map[string]any{"prompt_tokens": float64(8), "completion_tokens": float64(2)}}, source, "real")
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(data)
			}))
			defer up.Close()
			model := config.ModelEntry{ModelName: "matrix", LiteLLMParams: map[string]any{"model": "real", "api_base": up.URL, "api_key": "secret", "custom_llm_provider": "custom", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}, ModelInfo: map[string]any{"transport": transports[i]}}
			h := newHarness(t, model)
			admin := h.adminSession()
			owner := h.openScope(t, admin, "matrix")
			rows := listField(h.ok("GET", "/model/available", admin, nil).json(), "data")
			if len(rows) != 1 || len(rows[0]["endpoints"].([]any)) != 7 {
				t.Fatalf("自动公开接口不完整：%v", rows)
			}
			for _, target := range protocols {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
						body, err := llm.EncodeDialogue(llm.Dialogue{Stream: stream, Turns: []llm.Turn{{Role: "user", Text: "hello"}}}, target, "matrix")
						if err != nil {
							t.Fatal(err)
						}
						delete(body, "store")
						path := map[string]string{"openai-chat": "/v1/chat/completions", "openai-responses": "/v1/responses", "anthropic-messages": "/v1/messages", "gemini": "/v1beta/models/matrix:generateContent", "vertex": "/vertex/v1/models/matrix:generateContent"}[target]
						if stream && (target == "gemini" || target == "vertex") {
							path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						}
						before := h.moneyOf(t, owner)
						response := h.ok("POST", path, owner.key, body)
						if stream {
							terminal := map[string]string{"openai-chat": "[DONE]", "openai-responses": "response.completed", "anthropic-messages": "message_stop", "gemini": "finishReason", "vertex": "finishReason"}[target]
							if !strings.Contains(string(response.body), "matrix-ok") || !strings.Contains(string(response.body), terminal) {
								t.Fatalf("客户流式正文或终态缺失：%s", response.describe())
							}
						} else {
							result, err := llm.ParseDialogueResult(target, response.body)
							if err != nil || result.Text != "matrix-ok" {
								t.Fatalf("客户回复协议错误：%s %v", response.describe(), err)
							}
						}
						if !h.moneyOf(t, owner).grewBy(before, 0.000012) {
							t.Fatalf("跨协议计费不等于8输入2输出：%s", target)
						}
					})
				}
			}
			beforeCalls := calls.Load()
			denied := h.do("POST", "/v1/chat/completions", owner.key, map[string]any{"model": "matrix", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "unsupported": true})
			if denied.status != 400 || calls.Load() != beforeCalls {
				t.Fatalf("未知能力必须调用上游前失败：%s calls=%d", denied.describe(), calls.Load())
			}
			if denied := h.do("POST", "/bypass/openai/v1/chat/completions", owner.key, map[string]any{"model": "matrix", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}); denied.status == 200 {
				t.Fatal("Bypass 未显式启用仍被调用")
			}
			if calls.Load() != 10 {
				t.Fatal(fmt.Sprintf("上游调用数=%d，应为10", calls.Load()))
			}
		})
	}
}

// TestEndpointRetiredGlobals 验证废弃全局字段不进入严格客户模板，也不能通过新设置写入。
// 参数 t 为上下文；前置隔离数据库，直接写旧字段只构造失败复现，真实 HTTP 验证后 schema 清理。
func TestEndpointRetiredGlobals(t *testing.T) {
	h := newHarness(t, chatDeployment("globals"))
	admin := h.adminSession()
	for _, key := range []string{"stream_timeout", "enable_tag_filtering"} {
		if err := h.store.PutConfig("router_settings", key, true); err != nil {
			t.Fatal(err)
		}
	}
	h.ok("POST", "/v1/chat/completions", admin, map[string]any{"model": "globals", "messages": []any{map[string]any{"role": "user", "content": "hello"}}})
	for _, key := range []string{"stream_timeout", "enable_tag_filtering"} {
		response := h.do("POST", "/config/update", admin, map[string]any{"router_settings": map[string]any{key: true}})
		if response.status != 400 {
			t.Fatalf("废弃字段仍被写入：%s %s", key, response.describe())
		}
	}
}

// TestEndpointGoogleGuardrails 验证 Google 客户正文在转换调用上游前经过真实拦截和脱敏链。
// 参数 t 为上下文；前置隔离数据库、真实护栏与本地 Chat 上游，拒绝不计费，脱敏后放行，schema 清理。
func TestEndpointGoogleGuardrails(t *testing.T) {
	for _, kind := range []string{kindBlockedWords, kindRedact} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t, chatDeployment("google-guard"))
			admin := h.adminSession()
			owner := h.openScope(t, admin, "google-guard")
			mode := ""
			if kind == kindRedact {
				mode = "redact"
			}
			h.putGuardrail(t, "google-rule", guardrailRow("google-rule", kind, []string{"secret-word"}, mode))
			h.resetUpstream()
			body := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "say secret-word please"}}}}}
			before := h.moneyOf(t, owner)
			response := h.do("POST", "/v1beta/models/google-guard:generateContent", owner.key, body)
			calls := h.upstreamCalls()
			if kind == kindBlockedWords {
				if response.status != 400 || len(calls) != 0 || !h.moneyOf(t, owner).grewBy(before, 0) {
					t.Fatalf("Google 拦截失败或扣费：%s calls=%v", response.describe(), calls)
				}
			} else {
				if response.status != 200 || len(calls) != 1 {
					t.Fatalf("Google 脱敏未放行：%s calls=%v", response.describe(), calls)
				}
				raw, _ := json.Marshal(calls[0].Body)
				if strings.Contains(string(raw), "secret-word") {
					t.Fatalf("Google 明文泄露给上游：%s", raw)
				}
			}
		})
	}
}

// endpointStreamFrames 提供五种原厂 SSE 夹具；参数 source 是上游协议，返回带实测用量和终态的帧。
// 调用：真实网关矩阵回归，未知协议返回空；纯内存夹具，无外部副作用与清理。
func endpointStreamFrames(source string) []string {
	switch source {
	case "openai-chat":
		return []string{`{"choices":[{"index":0,"delta":{"content":"matrix-ok"}}]}`, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2}}`, "[DONE]"}
	case "openai-responses":
		return []string{`{"type":"response.output_text.delta","delta":"matrix-ok"}`, `{"type":"response.completed","response":{"id":"matrix","status":"completed","usage":{"input_tokens":8,"output_tokens":2}}}`}
	case "anthropic-messages":
		return []string{`{"type":"message_start","message":{"id":"matrix","usage":{"input_tokens":8}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"matrix-ok"}}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`, `{"type":"message_stop"}`}
	case "gemini", "vertex":
		return []string{`{"candidates":[{"content":{"parts":[{"text":"matrix-ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}}`}
	}
	return nil
}
