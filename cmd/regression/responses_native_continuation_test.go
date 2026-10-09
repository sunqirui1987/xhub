package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestResponsesNativeToolContinuation 验证统一入口到原生 Responses 的工具续接，无需原厂 store。
// 参数 t 为测试上下文；本地上游返回函数调用，第二轮检查完整工具关联和当前 instructions。
// JSON/SSE 都经过真实鉴权、数据库及路由；上游服务及私有 schema 由 cleanup 清理。
func TestResponsesNativeToolContinuation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			var mu sync.Mutex
			var requests []map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					w.WriteHeader(400)
					return
				}
				mu.Lock()
				requests = append(requests, body)
				turn := len(requests)
				mu.Unlock()
				response := map[string]any{"id": fmt.Sprintf("resp_tool_%d", turn), "object": "response", "status": "completed", "usage": map[string]any{"input_tokens": 11, "output_tokens": 5}}
				if turn == 1 {
					response["output"] = []any{map[string]any{"type": "function_call", "call_id": "call_lookup", "name": "lookup", "arguments": "{}"}}
				} else {
					response["output"] = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "found"}}}}
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					// 按原生 SSE 生命周期发送增量；终态只汇总结果，不能替代内容事件。
					created, _ := json.Marshal(map[string]any{"type": "response.created", "response": map[string]any{"id": response["id"]}})
					fmt.Fprintf(w, "data: %s\n\n", created)
					if turn == 1 {
						item, _ := json.Marshal(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "function_call", "call_id": "call_lookup", "name": "lookup", "arguments": "{}"}})
						fmt.Fprintf(w, "data: %s\n\n", item)
					} else {
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"found\"}\n\n")
					}
					raw, _ := json.Marshal(map[string]any{"type": "response.completed", "response": response})
					fmt.Fprintf(w, "data: %s\n\n", raw)
				} else {
					writeJSON(w, response)
				}
			}))
			t.Cleanup(upstream.Close)
			dep := chatDeployment("native-continuation")
			dep.ModelInfo = map[string]any{"id": "native-continuation-deployment", "transport": "bypass_openai_responses", "endpoint_types": []string{"responses"}}
			dep.LiteLLMParams["api_base"] = upstream.URL + "/v1"
			h := newHarness(t)
			admin := h.adminSession()
			h.ok("POST", "/credentials", admin, map[string]any{"credential_name": "continuation-native", "credential_info": map[string]any{"custom_llm_provider": "openai"}, "credential_values": map[string]any{"api_base": upstream.URL + "/v1", "api_key": "local-test"}})
			dep.LiteLLMParams["litellm_credential_name"] = "continuation-native"
			dep.ModelInfo["pricing_source"] = "manual"
			created := h.ok("POST", "/model/new", admin, map[string]any{"model_name": dep.ModelName, "litellm_params": dep.LiteLLMParams, "model_info": dep.ModelInfo}).json()
			deploymentID := stringField(created["model_info"].(map[string]any), "id")
			first := h.ok("POST", "/responses", admin, map[string]any{"model": dep.ModelName, "input": "lookup", "instructions": "old instructions", "stream": stream, "store": false})
			if !strings.Contains(string(first.body), "resp_tool_1") {
				t.Fatalf("首轮工具响应缺失: %s", first.describe())
			}
			second := h.ok("POST", "/responses", admin, map[string]any{"model": dep.ModelName, "input": []any{map[string]any{"type": "function_call_output", "call_id": "call_lookup", "output": "result"}}, "instructions": "new instructions", "stream": stream, "previous_response_id": "resp_tool_1"})
			if !strings.Contains(string(second.body), "found") {
				t.Fatalf("工具续接失败: %s", second.describe())
			}
			mu.Lock()
			calls := append([]map[string]any(nil), requests...)
			mu.Unlock()
			if len(calls) != 2 || calls[1]["store"] != false || calls[1]["previous_response_id"] != nil {
				t.Fatalf("原厂 store/续接字段错误: %v", calls)
			}
			input := calls[1]["input"].([]any)
			if len(input) != 4 || input[0].(map[string]any)["content"] != "new instructions" || input[1].(map[string]any)["content"] != "lookup" || input[2].(map[string]any)["call_id"] != "call_lookup" || input[3].(map[string]any)["output"] != "result" {
				t.Fatalf("工具关联或请求指令错误: %v", input)
			}
			h.ok("PATCH", "/model/"+deploymentID+"/update", admin, map[string]any{"model_info": map[string]any{"disabled": true}})
			unavailable := h.do("POST", "/responses", admin, map[string]any{"model": dep.ModelName, "input": "next", "previous_response_id": "resp_tool_2"})
			if unavailable.status != 409 || !strings.Contains(string(unavailable.body), "continuation_unavailable") {
				t.Fatalf("原部署不可用未阻止续接: %s", unavailable.describe())
			}
			mu.Lock()
			count := len(requests)
			mu.Unlock()
			if count != 2 {
				t.Fatal("原部署禁用后仍调用上游")
			}
			h.ok("POST", "/model/delete", admin, map[string]any{"id": deploymentID})
			h.ok("DELETE", "/credentials/continuation-native", admin, nil)
		})
	}
}

// TestResponsesFailedStreamCannotContinue 验证已发 created 但缺终态的失败流不生成续接归属。
// 参数 t 为测试上下文；真实网关调用本地截断 Chat SSE，失败后 previous_response_id 返回 400。
// 服务和私有 schema 由 cleanup 清理，不使用外部供应商凭据。
func TestResponsesFailedStreamCannotContinue(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl_failed\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
	}))
	t.Cleanup(upstream.Close)
	dep := chatDeployment("failed-continuation")
	dep.LiteLLMParams["api_base"] = upstream.URL
	h := newHarness(t, dep)
	admin := h.adminSession()
	failed := h.ok("POST", "/responses", admin, map[string]any{"model": dep.ModelName, "input": "first", "stream": true})
	if !strings.Contains(string(failed.body), "response.failed") || strings.Contains(string(failed.body), "response.completed") {
		t.Fatalf("截断流被标记成功: %s", failed.describe())
	}
	next := h.do("POST", "/responses", admin, map[string]any{"model": dep.ModelName, "input": "next", "previous_response_id": "chatcmpl_failed"})
	if next.status != 400 || !strings.Contains(string(next.body), "response ownership is unknown") {
		t.Fatalf("失败流发布了响应归属: %s", next.describe())
	}
}
