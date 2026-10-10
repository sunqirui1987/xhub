package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUpstreamDiagnosticsContract 前置本地供应商和私有schema，验证适配/原生JSON/SSE日志契约及错误。
// 参数t为测试上下文；验证完整多值头、原始扩展统计、计费用量、鉴权和SQL落库。
// 每例删除模型/凭据，服务和schema由Cleanup释放，不使用外部供应商。
func TestUpstreamDiagnosticsContract(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/stream=%v", native, stream), func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// 本地HTTP边界返回多值头及供应商扩展字段，供真实网关记录和脱敏。
					w.Header().Add("X-Trace", "first")
					w.Header().Add("X-Trace", "second")
					w.Header().Add("Set-Cookie", "private-session=secret")
					var request map[string]any
					if json.NewDecoder(r.Body).Decode(&request) != nil {
						w.WriteHeader(400)
						return
					}
					raw := string(mustJSON(request))
					if strings.Contains(raw, "fail-diagnostic") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(400)
						writeJSON(w, map[string]any{"error": map[string]any{"message": "failure diagnostic"}, "usage": map[string]any{"vendor_error_counter": 1}})
						return
					}
					usage := map[string]any{"prompt_tokens": 20, "completion_tokens": 2, "total_tokens": 22, "prompt_tokens_details": map[string]any{"cached_tokens": 12}, "vendor_statistics": map[string]any{"cache_region": "test"}}
					answer := map[string]any{"id": "chatcmpl-diagnostic", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "diagnostic-ok"}, "finish_reason": "stop"}}, "usage": usage}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						delta := map[string]any{"id": "chatcmpl-diagnostic", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "diagnostic-ok"}}}}
						fmt.Fprintf(w, "data: %s\n\n", mustJSON(delta))
						final := map[string]any{"id": "chatcmpl-diagnostic", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": usage}
						fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", mustJSON(final))
					} else {
						writeJSON(w, answer)
					}
				}))
				t.Cleanup(upstream.Close)
				h := newHarness(t)
				admin := h.adminSession()
				h.ok("POST", "/credentials", admin, map[string]any{"credential_name": "diagnostic-local", "credential_info": map[string]any{"custom_llm_provider": "openai"}, "credential_values": map[string]any{"api_base": upstream.URL + "/v1", "api_key": "local-only"}})
				dep := chatDeployment("diagnostic-local")
				dep.LiteLLMParams["api_base"] = upstream.URL + "/v1"
				dep.LiteLLMParams["litellm_credential_name"] = "diagnostic-local"
				dep.ModelInfo["pricing_source"] = "manual"
				dep.ModelInfo["endpoint_types"] = []string{"chat", "responses", "bypass:openai-chat"}
				saved := h.ok("POST", "/model/new", admin, map[string]any{"model_name": dep.ModelName, "litellm_params": dep.LiteLLMParams, "model_info": dep.ModelInfo}).json()
				id := saved["model_info"].(map[string]any)["id"]
				path := "/responses"
				request := map[string]any{"model": dep.ModelName, "input": "diagnostic request", "stream": stream}
				if native {
					path = "/bypass/openai/v1/chat/completions"
					request = chatRequest(dep.ModelName, "diagnostic request")
					request["stream"] = stream
				}
				response := h.ok("POST", path, admin, request)
				if !strings.Contains(string(response.body), "diagnostic-ok") {
					t.Fatalf("响应传输错误: %s", response.describe())
				}
				h.flushSpend()
				call := response.header("x-litellm-call-id")
				detail := logDetail(t, h, admin, call)
				meta := detail["metadata"].(map[string]any)
				diagnostic, ok := meta["upstream_response"].(map[string]any)
				if !ok {
					t.Fatalf("缺少持久化上游诊断: %v", meta)
				}
				headers := diagnostic["headers"].(map[string]any)
				if string(mustJSON(headers["X-Trace"])) != `["first","second"]` || string(mustJSON(headers["Set-Cookie"])) != `["***"]` {
					t.Fatalf("响应头丢失或凭据泄漏: %v", headers)
				}
				usage := diagnostic["usage"].(map[string]any)
				if diagnostic["usage_reported"] != true || usage["vendor_statistics"].(map[string]any)["cache_region"] != "test" || meta["cached_tokens"] != float64(12) || detail["cache_hit"] != "false" {
					t.Fatalf("原始统计或缓存分类错误: %v", detail)
				}
				if diagnostic["billing_usage"] == nil {
					t.Fatal("缺少计费统计对照")
				}
				stored, err := h.db.GetRequestLog(t.Context(), call)
				if err != nil || !strings.Contains(stored.UpstreamResponse, "cache_region") {
					t.Fatalf("SQL原始诊断未保存: %v", err)
				}
				denied := h.do("GET", "/spend/logs/ui/"+call, "", nil)
				if denied.status != 401 {
					t.Fatalf("未鉴权可读诊断: %s", denied.describe())
				}
				failure := map[string]any{"model": dep.ModelName, "input": "fail-diagnostic"}
				if native {
					failure = chatRequest(dep.ModelName, "fail-diagnostic")
				}
				failed := h.do("POST", path, admin, failure)
				if failed.status != 400 {
					t.Fatalf("错误状态未保留: %s", failed.describe())
				}
				h.flushSpend()
				failedDetail := logDetail(t, h, admin, failed.header("x-litellm-call-id"))
				failedDiagnostic := failedDetail["metadata"].(map[string]any)["upstream_response"].(map[string]any)
				if failedDiagnostic["status_code"] != float64(400) || failedDiagnostic["usage"].(map[string]any)["vendor_error_counter"] != float64(1) || failedDetail["spend"] != float64(0) {
					t.Fatalf("失败诊断或零费用错误: %v", failedDetail)
				}
				h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
				h.ok("DELETE", "/credentials/diagnostic-local", admin, nil)
			})
		}
	}
}
