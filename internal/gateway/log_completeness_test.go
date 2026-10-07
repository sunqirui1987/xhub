package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
)

// TestLogRecordRoundTripKeepsTheFactsTheConsoleReads writes one call the way
// the spend path does, then reads it back through the log list and the log
// detail the console opens. A missing header, body, TTFT, team name, or key
// snapshot fails here instead of showing up as a blank cell.
func TestLogRecordRoundTripKeepsTheFactsTheConsoleReads(t *testing.T) {
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	db := testIdentityStore(t)
	ctx := context.Background()
	admin, err := db.CreateUser(ctx, iam.Actor{Kind: "system"}, iam.UserInput{
		Email: "log-admin@example.com", Name: "Log Admin", Password: "password123", Role: iam.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	teamID := testTeam(t, db, admin.ID)
	key, _, err := db.CreateKey(ctx, iam.Actor{ID: admin.ID, Kind: "session"}, iam.KeyInput{
		Name: "probe-key", TeamID: teamID, OwnerType: iam.OwnerService,
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(9770 * time.Millisecond)
	ttft := 420
	cached := 20992
	messages := `[{"role":"user","content":"hello"}]`
	response := `{"id":"chatcmpl-probe","choices":[{"message":{"role":"assistant","content":"pong"}}]}`
	proxy := `{"method":"POST","url":"/v1/chat/completions","headers":{"Authorization":"***","Content-Type":"application/json","X-Request-Id":"trace-probe"},"body":{"model":"gpt-oss-120b"}}`

	gw := &Server{IAM: db}
	for i, id := range []string{"log-probe-a", "log-probe-b"} {
		hit := i == 1
		gw.persistSpend(live.SpendLog{
			RequestID: id, CallType: "chat", Model: "gpt-oss-120b",
			KeyID: key.ID, KeyHash: key.TokenHash, KeyAlias: key.Name,
			TeamID: teamID, TeamAlias: "Activity Team",
			UserID: admin.ID, OwnerType: iam.OwnerService,
			Prompt: 96, Completion: 5, Spend: 0.000384, SpendValid: true,
			Start: start.Format(time.RFC3339Nano), End: end.Format(time.RFC3339Nano),
			CacheHit: hit, Status: "success",
			TTFTMs: &ttft, Provider: "openai", CachedTokens: &cached,
			SessionID: "sess-probe", CacheKey: "cache-probe",
		}, 0.000384, promptExchange{messages: messages, response: response, proxy: proxy}, start, end)
	}

	srv := httptest.NewServer(New(cfg, nil, db).Handler())
	t.Cleanup(srv.Close)
	sess := loginAs(t, srv.URL, admin.Email, "password123")

	status, body := authed(t, srv.URL, sess, http.MethodGet, "/spend/logs/ui?group_by_session=true&page_size=50", nil)
	if status != http.StatusOK {
		t.Fatalf("list %d %s", status, trim(body))
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	for _, item := range page.Data {
		if item["session_id"] == "sess-probe" {
			row = item
		}
	}
	if row == nil {
		t.Fatalf("session row missing: %s", trim(body))
	}
	if row["session_total_count"] != float64(2) {
		t.Fatalf("session count %#v", row["session_total_count"])
	}
	meta, _ := row["metadata"].(map[string]any)
	if meta["user_api_key_team_alias"] != "Activity Team" {
		t.Fatalf("team alias %#v", meta["user_api_key_team_alias"])
	}
	if meta["user_api_key"] != key.TokenHash || meta["user_api_key_alias"] != "probe-key" {
		t.Fatalf("key snapshot %#v", meta)
	}
	if row["completionStartTime"] == nil || row["completionStartTime"] == row["startTime"] {
		t.Fatalf("ttft start %#v end-start %#v", row["completionStartTime"], row["startTime"])
	}
	if row["custom_llm_provider"] != "openai" || row["cache_key"] != "cache-probe" {
		t.Fatalf("provider/cache %#v %#v", row["custom_llm_provider"], row["cache_key"])
	}
	bill, _ := meta["cost_breakdown"].(map[string]any)
	if bill["input_cost"] == nil || bill["output_cost"] == nil || bill["total_cost"] == nil {
		t.Fatalf("bill %#v", bill)
	}

	status, body = authed(t, srv.URL, sess, http.MethodGet, "/spend/logs/session/ui?session_id=sess-probe", nil)
	if status != http.StatusOK {
		t.Fatalf("session %d %s", status, trim(body))
	}
	var sessionPage struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(body, &sessionPage); err != nil {
		t.Fatal(err)
	}
	if sessionPage.Total != 2 || len(sessionPage.Data) != 2 {
		t.Fatalf("session page total=%d rows=%d", sessionPage.Total, len(sessionPage.Data))
	}

	status, body = authed(t, srv.URL, sess, http.MethodGet, "/spend/logs/ui/log-probe-a", nil)
	if status != http.StatusOK {
		t.Fatalf("detail %d %s", status, trim(body))
	}
	var detail map[string]any
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	msgs, _ := detail["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages %#v", detail["messages"])
	}
	resp, _ := detail["response"].(map[string]any)
	if resp["id"] != "chatcmpl-probe" {
		t.Fatalf("response %#v", detail["response"])
	}
	px, _ := detail["proxy_server_request"].(map[string]any)
	headers, _ := px["headers"].(map[string]any)
	if px["method"] != "POST" || headers["X-Request-Id"] != "trace-probe" || headers["Authorization"] != "***" {
		t.Fatalf("proxy %#v", px)
	}
	if px["body"] == nil {
		t.Fatal("request body missing from proxy document")
	}
}

func TestPlanRoutePinsTheSameDeploymentForOnePrompt(t *testing.T) {
	s := &Server{}
	body := map[string]any{
		"model": "gpt-5.6-sol",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Codex."},
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	p := &auth.Principal{UserID: "user-1"}
	first := s.PlanRoute(&http.Request{Header: http.Header{}}, "gpt-5.6-sol", body, p)
	if first.SessionID == "" || first.Pinned != "" {
		t.Fatalf("first plan %+v", first)
	}
	s.CommitRoute(first, "dep-a", "resp-1")
	second := s.PlanRoute(&http.Request{Header: http.Header{}}, "gpt-5.6-sol", body, p)
	if second.SessionID != first.SessionID || second.Pinned != "dep-a" {
		t.Fatalf("second plan %+v", second)
	}
	continued := s.PlanRoute(&http.Request{Header: http.Header{}}, "gpt-5.6-sol", map[string]any{
		"previous_response_id": "resp-1",
	}, p)
	if continued.Pinned != "dep-a" {
		t.Fatalf("previous response pin %+v", continued)
	}
}
