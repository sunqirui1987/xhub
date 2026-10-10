package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCodexAgentConversation 验证 Codex 客户端通过个人密钥进行三轮增量会话。
// 参数 t 为测试上下文；前置真实网关及隔离数据库，核对完整历史、客户端头、五级账单和失败无外发。
// 返回值为空，失败终止测试；harness 在结束时清理服务及私有 schema。
func TestCodexAgentConversation(t *testing.T) {
	dep := chatDeployment("codex-model")
	dep.ModelInfo["endpoint_types"] = []string{"chat", "responses"}
	dep.ModelInfo["id"] = "codex-agent-deployment"
	h := newHarness(t)
	// 默认夹具重复使用同一响应 ID；本例必须模拟真实供应商逐轮生成独立 ID，才能验证续接链。
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		h.replies = append(h.replies, upstreamCall{Path: r.URL.Path, Body: body})
		turn := len(h.replies)
		h.mu.Unlock()
		answer := chatAnswer("codex-model", nil)
		answer["id"] = fmt.Sprintf("chatcmpl-codex-%d", turn)
		writeJSON(w, answer)
	}))
	t.Cleanup(upstream.Close)
	admin := h.adminSession()
	// 通过管理接口创建带独立供应商的数据库部署，保留真实持久化与端点契约。
	dep.LiteLLMParams["api_base"] = upstream.URL + "/v1"
	dep.LiteLLMParams["litellm_credential_name"] = "codex-upstream"
	dep.ModelInfo["pricing_source"] = "manual"
	h.ok("POST", "/credentials", admin, map[string]any{"credential_name": "codex-upstream", "credential_info": map[string]any{"custom_llm_provider": "openai"}, "credential_values": map[string]any{"api_base": upstream.URL + "/v1", "api_key": "sk-fake"}})
	h.ok("POST", "/model/new", admin, map[string]any{"model_name": dep.ModelName, "litellm_params": dep.LiteLLMParams, "model_info": dep.ModelInfo})
	owner := h.openScope(t, admin, "codex-agent")
	headers := map[string]string{"User-Agent": "codex_cli_rs/0.1.0 (xhub acceptance simulator)", "Session_id": "codex-regression", "Originator": "codex_cli_rs"}
	previous := ""
	ids := map[string]bool{}
	prompts := []string{"Remember project-alpha", "Recall the project", "Confirm the project"}
	for index, prompt := range prompts {
		body := map[string]any{"model": "codex-model", "instructions": "You are Codex, a coding agent.", "input": []any{map[string]any{"role": "user", "content": prompt}}, "store": false, "stream": false, "max_output_tokens": 128}
		if previous != "" {
			body["previous_response_id"] = previous
		}
		reply := h.doHeaders(http.MethodPost, "/v1/responses", owner.key, body, headers)
		if reply.status != 200 {
			t.Fatalf("Codex 第 %d 轮失败：%s", index+1, reply.describe())
		}
		answer := reply.json()
		next := stringField(answer, "id")
		if next == "" || next == previous || answer["status"] != "completed" {
			t.Fatalf("Codex 响应契约错误：%v", answer)
		}
		calls := h.upstreamCalls()
		if len(calls) != index+1 {
			t.Fatalf("Codex 每轮应外发一次：%v", calls)
		}
		messages := calls[index].Body["messages"].([]any)
		if len(messages) != 2*(index+1) || messages[0].(map[string]any)["role"] != "system" {
			t.Fatalf("Codex 历史长度或指令丢失：%v", messages)
		}
		for j := 0; j <= index; j++ {
			if messages[j*2+1].(map[string]any)["content"] != prompts[j] {
				t.Fatalf("Codex 用户历史丢失：%v", messages)
			}
			if j < index && messages[j*2+2].(map[string]any)["content"] != defaultReply.Content {
				t.Fatalf("Codex 助手历史丢失：%v", messages)
			}
		}
		callID := reply.headers.Get("x-litellm-call-id")
		if callID == "" || ids[callID] {
			t.Fatal("Codex 三轮应有独立日志 ID")
		}
		ids[callID] = true
		h.flushSpend()
		bill := h.ok("GET", "/spend/logs/ui/"+callID, admin, nil).json()
		for field, want := range map[string]string{"session_id": "codex-regression", "api_key": owner.keyID, "user": owner.userID, "team_id": owner.teamID, "project_id": owner.projectID, "organization_id": owner.orgID} {
			if bill[field] != want {
				t.Fatalf("Codex 第 %d 轮归属 %s 错误：%v", index+1, field, bill)
			}
		}
		cost, _ := floatField(bill, "spend")
		if cost != expectedCost() {
			t.Fatalf("Codex 逐轮金额错误：%v", bill)
		}
		proxy := bill["proxy_server_request"].(map[string]any)
		storedBody := proxy["body"].(map[string]any)
		if len(storedBody["input"].([]any)) != 2*index+1 || storedBody["store"] != nil || storedBody["previous_response_id"] != nil {
			t.Fatalf("Codex 日志未恢复完整历史或泄漏代理字段：%v", proxy)
		}
		previous = next
	}
	stranger := h.openScope(t, admin, "codex-stranger")
	for _, token := range []string{owner.key, stranger.key} {
		prior := previous
		if token == owner.key {
			prior = "missing-response"
		}
		reply := h.doHeaders("POST", "/v1/responses", token, map[string]any{"model": "codex-model", "input": "next", "previous_response_id": prior}, headers)
		if reply.status != 400 || reply.json()["error"] == nil {
			t.Fatalf("Codex 非法或跨身份续接未拒绝：%s", reply.describe())
		}
	}
	if len(h.upstreamCalls()) != 3 {
		t.Fatal("Codex 拒绝请求不应外发")
	}
	successes := 0
	for _, row := range h.spendLogs(t, admin) {
		if row["status"] == "success" {
			successes++
		} else if cost, _ := floatField(row, "spend"); cost != 0 {
			t.Fatal("Codex 拒绝请求不应记费")
		}
	}
	if successes != 3 {
		t.Fatalf("Codex 成功账单应恰好三笔：%d", successes)
	}
}
