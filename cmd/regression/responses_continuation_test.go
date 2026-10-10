package regression

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestResponsesIncrementalContinuation 验证真实网关到 Chat 上游的三轮增量续接及计费。
// 参数 t 为测试上下文；隔离 PostgreSQL、真实鉴权，覆盖省略 store 和 store=false、JSON/SSE。
// 验证上游完整历史、错误无上游副作用；harness 关闭服务并删除私有 schema。
func TestResponsesIncrementalContinuation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			dep := chatDeployment("continuation-model")
			dep.ModelInfo["endpoint_types"] = []string{"chat", "responses"}
			dep.ModelInfo["id"] = "continuation-deployment"
			other := chatDeployment("other-model")
			other.ModelInfo["endpoint_types"] = []string{"chat", "responses"}
			h := newHarness(t, dep, other)
			admin := h.adminSession()
			prev := ""
			for i, text := range []string{"first", "second", "third"} {
				body := map[string]any{"model": "continuation-model", "input": []any{map[string]any{"role": "user", "content": text}}, "stream": stream, "litellm_trace_id": "local-test"}
				if prev != "" {
					body["previous_response_id"] = prev
					body["store"] = false
				}
				reply := h.ok(http.MethodPost, "/responses", admin, body)
				if stream {
					for _, line := range bytes.Split(reply.body, []byte("\n")) {
						var event map[string]any
						if bytes.HasPrefix(line, []byte("data: ")) && json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event) == nil && event["type"] == "response.completed" {
							response := event["response"].(map[string]any)
							prev, _ = response["id"].(string)
							if response["store"] != false {
								t.Fatal("上游 store 必须保持 false")
							}
						}
					}
				} else {
					prev = stringField(reply.json(), "id")
				}
				if prev == "" {
					t.Fatalf("第 %d 轮缺少响应 ID: %s", i+1, reply.describe())
				}
				calls := h.upstreamCalls()
				if len(calls) != i+1 {
					t.Fatalf("续接错误地命中响应缓存: %d", len(calls))
				}
				messages := calls[i].Body["messages"].([]any)
				if len(messages) != 2*i+1 || calls[i].Body["previous_response_id"] != nil || calls[i].Body["store"] != nil {
					t.Fatalf("第 %d 轮上游未恢复历史: %+v", i+1, calls[i])
				}
				for j := 0; j < i; j++ {
					if messages[j*2].(map[string]any)["content"] != []string{"first", "second"}[j] || messages[j*2+1].(map[string]any)["content"] != defaultReply.Content {
						t.Fatalf("第 %d 轮历史内容丢失: %v", i+1, messages)
					}
				}
			}
			for _, invalid := range []map[string]any{
				{"model": "continuation-model", "input": "next", "previous_response_id": "unknown"},
				{"model": "other-model", "input": "next", "previous_response_id": prev},
				{"model": "continuation-model", "input": "next", "store": "false"},
			} {
				reply := h.do(http.MethodPost, "/responses", admin, invalid)
				if reply.status != http.StatusBadRequest {
					t.Fatalf("无效续接未返回 400: %s", reply.describe())
				}
			}
			if len(h.upstreamCalls()) != 3 {
				t.Fatal("拒绝续接仍调用上游")
			}
			rows := h.spendLogs(t, admin)
			successes := 0
			for _, row := range rows {
				amount, _ := floatField(row, "spend")
				// 拒绝请求也会保留诊断日志；只统计成功三轮，并验证失败金额始终为零。
				if row["status"] != "success" {
					if amount != 0 {
						t.Fatalf("拒绝续接不应收费: %v", row)
					}
					continue
				}
				successes++
				if amount != expectedCost() {
					t.Fatalf("续接计费错误: %v", row)
				}
			}
			if successes != 3 {
				t.Fatalf("三轮成功续接账单数量错误: %d", successes)
			}
			stranger := h.provision(t, admin, "continuation-stranger")
			cross := h.do("POST", "/responses", stranger.key, map[string]any{"model": "continuation-model", "input": "next", "previous_response_id": prev})
			if cross.status != 400 {
				t.Fatalf("跨调用方续接被接受: %s", cross.describe())
			}
			// 新规则必须检查恢复历史里的 first，不能只检查本轮无敏感词的 next。
			h.putGuardrail(t, "continuation-rule", guardrailRow("continuation-rule", kindBlockedWords, []string{"first"}, ""))
			blocked := h.do("POST", "/responses", admin, map[string]any{"model": "continuation-model", "input": "next", "previous_response_id": prev})
			if blocked.status != 400 {
				t.Fatalf("恢复历史绕过护栏: %s", blocked.describe())
			}
			if len(h.upstreamCalls()) != 3 {
				t.Fatal("跨身份或护栏拒绝仍调用上游")
			}
		})
	}
}
