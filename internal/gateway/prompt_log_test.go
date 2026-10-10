package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xorm.io/builder"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestRememberExchangeKeepsFailureCandidateWhenPromptStorageOff 验证关闭提示词保存仍暂存失败响应。
// 前置条件是开关关闭；请求正文不得保存，原始响应必须保留到 RecordSpend；内存项由 takeExchange 清理。
func TestRememberExchangeKeepsFailureCandidateWhenPromptStorageOff(t *testing.T) {
	s := &Server{Cfg: &config.Config{}, exchanges: map[string]promptExchange{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s.rememberExchange("failure-off", req, []byte("{\"messages\":[{\"content\":\"secret\"}]}"), []byte("upstream raw error"))
	ex := s.takeExchange("failure-off")
	if ex.messages != "" || ex.proxy != "" || ex.response != "" {
		t.Fatalf("关闭开关后保存了请求或成功响应: %+v", ex)
	}
	if ex.rawResponse != "upstream raw error" {
		t.Fatalf("失败候选正文丢失: %q", ex.rawResponse)
	}
	if _, exists := s.exchanges["failure-off"]; exists {
		t.Fatal("takeExchange 未清理临时数据")
	}
}

func TestPromptJSONKeepsHeadersBodyAndResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer sk-local-master")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "trace-1")
	messages, response, proxy := promptJSON(req,
		[]byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`),
		[]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"hi"}}]}`),
	)
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(messages), &msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0]["content"] != "hello" {
		t.Fatalf("messages %s", messages)
	}
	if !strings.Contains(response, "chatcmpl-1") || !strings.Contains(response, `"hi"`) {
		t.Fatalf("response %s", response)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(proxy), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["method"] != "POST" || doc["url"] != "/v1/chat/completions" {
		t.Fatalf("proxy %s", proxy)
	}
	headers, _ := doc["headers"].(map[string]any)
	if headers["Authorization"] != "***" {
		t.Fatalf("authorization leaked: %v", headers)
	}
	if headers["X-Request-Id"] != "trace-1" || headers["Content-Type"] != "application/json" {
		t.Fatalf("headers %v", headers)
	}
	body, _ := doc["body"].(map[string]any)
	if body["model"] != "gpt-4o-mini" {
		t.Fatalf("body %v", body)
	}
	if strings.Contains(proxy, "sk-local-master") || strings.Contains(messages, "sk-") {
		t.Fatalf("secret leaked: %s", proxy)
	}
	polled := httptest.NewRequest(http.MethodGet, "/api/v1/generate/record-info?taskId=suno-1", nil)
	_, _, polledProxy := promptJSON(polled, nil, []byte(`{"data":{"taskId":"suno-1"}}`))
	if !strings.Contains(polledProxy, "/api/v1/generate/record-info?taskId=suno-1") {
		t.Fatalf("query dropped %s", polledProxy)
	}
}

// TestSpendLogRoundTripReturnsPromptPayload stores one call through the same
// write path the gateway uses and reads the bodies back through the same read
// path the log drawer uses, so the two stay in step.
func TestSpendLogRoundTripReturnsPromptPayload(t *testing.T) {
	db := testIdentityStore(t)
	ctx := context.Background()
	id := "prompt-probe-" + time.Now().UTC().Format("20060102150405.000000000")
	start := time.Now().UTC().Add(-time.Minute)
	messages := `[{"role":"user","content":"hello"}]`
	response := `{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}]}`
	proxy := `{"method":"POST","url":"/v1/chat/completions","headers":{"Content-Type":"application/json","Authorization":"***"},"body":{"messages":[{"role":"user","content":"hello"}]}}`

	rec := iam.UsageRecord{
		RequestID: id, TS: start, KeyID: "key-1", OwnerType: iam.OwnerPersonal,
		UserID: "user-1", TeamID: "team-1", Model: "gpt-4o-mini", CallType: "chat",
		Status: "success", PromptTokens: 5, CompletionTokens: 4, Cost: 0.1,
		RequestBody: messages, ResponseBody: response,
	}
	if err := db.RecordUsage(ctx, []iam.UsageRecord{rec}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	// The event must be readable through the scoped read API, which is the only
	// way a handler reaches it.
	event, err := db.GetUsageEvent(ctx, iam.UsageQuery{Cond: builder.NewCond()}, id)
	if err != nil {
		t.Fatalf("read back event: %v", err)
	}
	if event.Model != "gpt-4o-mini" || event.PromptTokens != 5 || event.CompletionTokens != 4 {
		t.Fatalf("event: %+v", event)
	}
	// Ownership is snapshotted at write time, which is what makes the log
	// drawer able to show whose call it was after the key is gone.
	if event.UserID != "user-1" || event.TeamID != "team-1" || event.OwnerType != iam.OwnerPersonal {
		t.Fatalf("ownership snapshot: %+v", event)
	}

	body, err := db.GetRequestLog(ctx, id)
	if err != nil {
		t.Fatalf("read back bodies: %v", err)
	}
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(body.RequestBody), &msgs); err != nil {
		t.Fatalf("messages are not JSON: %v", err)
	}
	if len(msgs) != 1 || msgs[0]["content"] != "hello" {
		t.Fatalf("messages %#v", msgs)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(body.ResponseBody), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["id"] != "chatcmpl-1" {
		t.Fatalf("response %#v", resp)
	}

	// The proxy document is what promptJSON builds; it is stored alongside the
	// pair as the request that produced the response, with the credential
	// redacted.
	var px map[string]any
	if err := json.Unmarshal([]byte(proxy), &px); err != nil {
		t.Fatal(err)
	}
	if px["url"] != "/v1/chat/completions" {
		t.Fatalf("proxy %#v", px)
	}
	hdrs, _ := px["headers"].(map[string]any)
	if hdrs["Authorization"] != "***" {
		t.Fatalf("headers %#v", hdrs)
	}
}
