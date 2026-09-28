package gateway

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

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
}

func TestSpendLogRoundTripReturnsPromptPayload(t *testing.T) {
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	id := "prompt-probe-" + time.Now().UTC().Format("20060102150405.000000000")
	start := time.Now().UTC().Add(-time.Minute)
	messages := `[{"role":"user","content":"hello"}]`
	response := `{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}]}`
	proxy := `{"method":"POST","url":"/v1/chat/completions","headers":{"Content-Type":"application/json","Authorization":"***"},"body":{"messages":[{"role":"user","content":"hello"}]}}`
	err = st.InsertSpendLogWithPrompt(id, "chat", "gpt-4o-mini", "hash", 5, 4, sql.NullFloat64{Float64: 0.1, Valid: true}, start, start.Add(time.Second), false, "success", "admin", messages, response, proxy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.Exec(`DELETE FROM spend_logs WHERE request_id = $1`, id)
	})
	list, err := st.ListSpendLogs()
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	for _, item := range list {
		if item["request_id"] == id {
			row = item
			break
		}
	}
	if row == nil {
		t.Fatal("stored log was not listed")
	}
	got, _ := row["messages"].([]any)
	if len(got) != 1 {
		t.Fatalf("messages %#v", row["messages"])
	}
	resp, _ := row["response"].(map[string]any)
	if resp["id"] != "chatcmpl-1" {
		t.Fatalf("response %#v", row["response"])
	}
	px, _ := row["proxy_server_request"].(map[string]any)
	if px["url"] != "/v1/chat/completions" {
		t.Fatalf("proxy %#v", row["proxy_server_request"])
	}
	hdrs, _ := px["headers"].(map[string]any)
	if hdrs["Authorization"] != "***" {
		t.Fatalf("headers %#v", hdrs)
	}
}
