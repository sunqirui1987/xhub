package suite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/store"
)

func testRedisURL() string {
	if u := os.Getenv("XHUB_TEST_REDIS_URL"); u != "" {
		return u
	}
	return "redis://127.0.0.1:6379/1"
}

func nearSpend(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-8
}

func chargedSpend(t *testing.T, body map[string]any) float64 {
	t.Helper()
	switch v := body["spend"].(type) {
	case float64:
		return v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			t.Fatal(err)
		}
		return f
	default:
		t.Fatalf("spend %T %v", body["spend"], body["spend"])
		return 0
	}
}

func logIDs(t *testing.T, h http.Handler, master string) map[string]bool {
	t.Helper()
	rec := doJSON(t, h, "GET", "/spend/logs?page=1&page_size=100", master, nil)
	if rec.Code != 200 {
		t.Fatalf("spend logs %d %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, row := range page.Data {
		out[str(row["request_id"])] = true
	}
	return out
}

func keySpend(t *testing.T, h http.Handler, master, plain string) float64 {
	t.Helper()
	rec := doJSON(t, h, "GET", "/key/info?key="+plain, master, nil)
	if rec.Code != 200 {
		t.Fatalf("key info %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Info map[string]any `json:"info"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return chargedSpend(t, body.Info)
}

func TestRedisSpendBatchFlush(t *testing.T) {
	redisURL := testRedisURL()
	dbURL := testDatabaseURL(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl_batch", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	st, err := store.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	cfg := &config.Config{
		ModelList: []config.ModelEntry{{
			ModelName:     "gpt-4o-mini",
			LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "sk-upstream", "api_base": up.URL},
		}},
		RouterSettings:  config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 0, Timeout: 15},
		GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: dbURL, RedisURL: redisURL},
	}
	s := gateway.New(cfg, st)
	if s.Live == nil {
		t.Fatal("redis client was not opened")
	}
	t.Cleanup(func() { _ = s.Live.Close() })
	if err := s.Live.ClearSpendQueue(); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	master := "sk-master"

	org := doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "batch-org"})
	if org.Code != 200 {
		t.Fatalf("org %d %s", org.Code, org.Body.String())
	}
	orgID := str(decodeBody(t, org.Body.Bytes())["organization_id"])
	user := doJSON(t, h, "POST", "/user/new", master, map[string]any{"user_email": "batch@xhub.dev", "user_role": "internal_user"})
	if user.Code != 200 {
		t.Fatalf("user %d %s", user.Code, user.Body.String())
	}
	userID := str(decodeBody(t, user.Body.Bytes())["user_id"])
	team := doJSON(t, h, "POST", "/team/new", master, map[string]any{"team_alias": "batch-team", "organization_id": orgID})
	if team.Code != 200 {
		t.Fatalf("team %d %s", team.Code, team.Body.String())
	}
	teamID := str(decodeBody(t, team.Body.Bytes())["team_id"])
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "models": []any{"gpt-4o-mini"},
		"team_id": teamID, "user_id": userID, "organization_id": orgID,
	})
	if gen.Code != 200 {
		t.Fatalf("key %d %s", gen.Code, gen.Body.String())
	}
	plain := str(decodeBody(t, gen.Body.Bytes())["key"])

	const n = 10
	ids := make([]string, 0, n)
	var charged float64
	for i := 0; i < n; i++ {
		rec := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
			"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
		if rec.Code != 200 {
			t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
		}
		cost := rec.Header().Get("x-litellm-response-cost")
		if cost == "" {
			t.Fatal("missing cost header")
		}
		v, err := strconv.ParseFloat(cost, 64)
		if err != nil {
			t.Fatal(err)
		}
		charged += v
		id := rec.Header().Get("x-litellm-call-id")
		if id == "" {
			t.Fatal("missing call id")
		}
		ids = append(ids, id)
	}

	listed := logIDs(t, h, master)
	for _, id := range ids {
		if listed[id] {
			t.Fatalf("request %s was written before flush", id)
		}
	}
	if got := keySpend(t, h, master, plain); got != 0 {
		t.Fatalf("key spend %v written before flush", got)
	}
	if teamRow, err := s.Store.GetTeam(teamID); err != nil || teamRow.Spend != 0 {
		t.Fatalf("team spend before flush %v %v", teamRow, err)
	}

	if _, err := s.Store.DB.Exec(`ALTER TABLE spend_logs RENAME TO spend_logs_hold`); err != nil {
		t.Fatal(err)
	}
	s.FlushSpend()
	if _, err := s.Store.DB.Exec(`ALTER TABLE spend_logs_hold RENAME TO spend_logs`); err != nil {
		t.Fatal(err)
	}
	still := logIDs(t, h, master)
	for _, id := range ids {
		if still[id] {
			t.Fatalf("failed flush persisted %s", id)
		}
	}
	if got := keySpend(t, h, master, plain); got != 0 {
		t.Fatalf("failed flush wrote key spend %v", got)
	}

	s.Store.ResetStatements()
	s.FlushSpend()
	stmts := s.Store.Statements()
	if stmts < 1 || stmts >= n {
		t.Fatalf("flush used %d statements for %d requests", stmts, n)
	}
	flushed := logIDs(t, h, master)
	for _, id := range ids {
		if !flushed[id] {
			t.Fatalf("flush missed %s in %v", id, flushed)
		}
	}
	if got := keySpend(t, h, master, plain); !nearSpend(got, charged) {
		t.Fatalf("key spend %v want %v", got, charged)
	}
	keys := doJSON(t, h, "GET", "/global/spend/keys", master, nil)
	if keys.Code != 200 || !strings.Contains(keys.Body.String(), store.HashKey(plain)) {
		t.Fatalf("usage key total %d %s", keys.Code, keys.Body.String())
	}
	teamRow, err := s.Store.GetTeam(teamID)
	if err != nil || !nearSpend(teamRow.Spend, charged) {
		t.Fatalf("team spend %v %v want %v", teamRow, err, charged)
	}
	userRow, err := s.Store.GetUser(userID)
	if err != nil || !nearSpend(userRow.Spend, charged) {
		t.Fatalf("user spend %v %v want %v", userRow, err, charged)
	}
	orgRow, err := s.Store.GetOrg(orgID)
	if err != nil || !nearSpend(orgRow.Spend, charged) {
		t.Fatalf("org spend %v %v want %v", orgRow, err, charged)
	}
}

func TestSpendWithoutRedis(t *testing.T) {
	s, master := testEnv(t)
	if s.Live != nil {
		t.Fatal("test env opened redis")
	}
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "models": []any{"gpt-4o-mini"}})
	if gen.Code != 200 {
		t.Fatalf("key %d %s", gen.Code, gen.Body.String())
	}
	plain := str(decodeBody(t, gen.Body.Bytes())["key"])
	rec := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	id := rec.Header().Get("x-litellm-call-id")
	if id == "" || rec.Header().Get("x-litellm-response-cost") == "" {
		t.Fatalf("headers call %q cost %q", id, rec.Header().Get("x-litellm-response-cost"))
	}
	if !logIDs(t, h, master)[id] {
		t.Fatal("spend log missing before any flush")
	}
	cost, err := strconv.ParseFloat(rec.Header().Get("x-litellm-response-cost"), 64)
	if err != nil {
		t.Fatal(err)
	}
	if got := keySpend(t, h, master, plain); !nearSpend(got, cost) {
		t.Fatalf("key spend %v want %v", got, cost)
	}
}

func TestFlushReplayDoesNotDoubleSpend(t *testing.T) {
	s, h, master, plain, teamID, userID, orgID := openSpendRedis(t)
	rec := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	id := rec.Header().Get("x-litellm-call-id")
	cost, err := strconv.ParseFloat(rec.Header().Get("x-litellm-response-cost"), 64)
	if err != nil || id == "" {
		t.Fatalf("cost %q id %q %v", rec.Header().Get("x-litellm-response-cost"), id, err)
	}
	s.FlushSpend()
	before := keySpend(t, h, master, plain)
	teamBefore, _ := s.Store.GetTeam(teamID)
	userBefore, _ := s.Store.GetUser(userID)
	orgBefore, _ := s.Store.GetOrg(orgID)
	if !nearSpend(before, cost) || !nearSpend(teamBefore.Spend, cost) || !nearSpend(userBefore.Spend, cost) || !nearSpend(orgBefore.Spend, cost) {
		t.Fatalf("after flush key %v team %v user %v org %v want %v", before, teamBefore.Spend, userBefore.Spend, orgBefore.Spend, cost)
	}
	// The queue and hot counters are restored as if the ack after commit did not happen.
	hash := store.HashKey(plain)
	if err := s.Live.EnqueueLog(liveSpend(id, hash, teamID, userID, orgID, cost)); err != nil {
		t.Fatal(err)
	}
	_ = s.Live.ChargeSpend(hash, cost)
	_ = s.Live.ChargeSpend(teamID, cost)
	_ = s.Live.ChargeSpend(userID, cost)
	_ = s.Live.ChargeSpend(orgID, cost)
	s.FlushSpend()
	if got := keySpend(t, h, master, plain); !nearSpend(got, before) {
		t.Fatalf("replay doubled key spend %v -> %v", before, got)
	}
	teamAfter, _ := s.Store.GetTeam(teamID)
	userAfter, _ := s.Store.GetUser(userID)
	orgAfter, _ := s.Store.GetOrg(orgID)
	if !nearSpend(teamAfter.Spend, teamBefore.Spend) || !nearSpend(userAfter.Spend, userBefore.Spend) || !nearSpend(orgAfter.Spend, orgBefore.Spend) {
		t.Fatalf("replay doubled team %v user %v org %v", teamAfter.Spend, userAfter.Spend, orgAfter.Spend)
	}
}

func TestFlushAckFailureRetriesWithoutDoubleSpend(t *testing.T) {
	s, h, master, plain, teamID, userID, orgID := openSpendRedis(t)
	rec := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	id := rec.Header().Get("x-litellm-call-id")
	cost, err := strconv.ParseFloat(rec.Header().Get("x-litellm-response-cost"), 64)
	if err != nil || id == "" || cost <= 0 {
		t.Fatalf("cost %q id %q %v", rec.Header().Get("x-litellm-response-cost"), id, err)
	}
	hash := store.HashKey(plain)
	orig := dataplane.SpendAck
	calls := 0
	dataplane.SpendAck = func(c *live.Client, deltas map[string]float64, n int, head string) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("ack failed")
		}
		return orig(c, deltas, n, head)
	}
	t.Cleanup(func() { dataplane.SpendAck = orig })

	s.FlushSpend()
	if calls != 1 {
		t.Fatalf("ack calls %d", calls)
	}
	if got := keySpend(t, h, master, plain); !nearSpend(got, cost) {
		t.Fatalf("postgres key spend %v want %v", got, cost)
	}
	if !nearSpend(s.Live.HotSpend(hash), cost) || !nearSpend(s.Live.HotSpend(teamID), cost) {
		t.Fatalf("failed ack cleared hot key %v team %v", s.Live.HotSpend(hash), s.Live.HotSpend(teamID))
	}
	queued, raw := s.Live.PeekLogs(10)
	if len(raw) == 0 || len(queued) == 0 || queued[0].RequestID != id {
		t.Fatalf("ack failure dropped logs %#v", queued)
	}
	head := raw[0]

	s.FlushSpend()
	if !nearSpend(s.Live.HotSpend(hash), 0) || !nearSpend(s.Live.HotSpend(teamID), 0) || !nearSpend(s.Live.HotSpend(userID), 0) || !nearSpend(s.Live.HotSpend(orgID), 0) {
		t.Fatalf("retry left hot key %v team %v user %v org %v", s.Live.HotSpend(hash), s.Live.HotSpend(teamID), s.Live.HotSpend(userID), s.Live.HotSpend(orgID))
	}
	if got := keySpend(t, h, master, plain); !nearSpend(got, cost) {
		t.Fatalf("retry doubled key spend %v", got)
	}
	teamAfter, _ := s.Store.GetTeam(teamID)
	if !nearSpend(teamAfter.Spend, cost) {
		t.Fatalf("retry doubled team spend %v", teamAfter.Spend)
	}
	if err := s.Live.AckFlushed(map[string]float64{hash: cost, teamID: cost}, 1, head); err != nil {
		t.Fatal(err)
	}
	if !nearSpend(s.Live.HotSpend(hash), 0) || !nearSpend(s.Live.HotSpend(teamID), 0) {
		t.Fatalf("repeated ack subtracted hot key %v team %v", s.Live.HotSpend(hash), s.Live.HotSpend(teamID))
	}
}

func TestTeamHotBudgetBlocksSecondChat(t *testing.T) {
	s, h, master, plain, teamID, _, _ := openSpendRedis(t)
	first := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if first.Code != 200 {
		t.Fatalf("first chat %d %s", first.Code, first.Body.String())
	}
	cost, err := strconv.ParseFloat(first.Header().Get("x-litellm-response-cost"), 64)
	if err != nil || cost <= 0 {
		t.Fatalf("cost %q", first.Header().Get("x-litellm-response-cost"))
	}
	upd := doJSON(t, h, "POST", "/team/update", master, map[string]any{"team_id": teamID, "max_budget": cost})
	if upd.Code != 200 {
		t.Fatalf("team update %d %s", upd.Code, upd.Body.String())
	}
	second := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if second.Code != 429 {
		t.Fatalf("second chat want 429 got %d %s", second.Code, second.Body.String())
	}
	_ = s
}

func TestEnqueueFailureWritesPostgres(t *testing.T) {
	s, h, master, plain, _, _, _ := openSpendRedis(t)
	if err := s.Live.Close(); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, h, "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	id := rec.Header().Get("x-litellm-call-id")
	cost, err := strconv.ParseFloat(rec.Header().Get("x-litellm-response-cost"), 64)
	if err != nil || id == "" {
		t.Fatal(rec.Header())
	}
	if !logIDs(t, h, master)[id] {
		t.Fatal("enqueue failure dropped the spend log")
	}
	if got := keySpend(t, h, master, plain); !nearSpend(got, cost) {
		t.Fatalf("key spend %v want %v", got, cost)
	}
}

func liveSpend(id, hash, teamID, userID, orgID string, cost float64) live.SpendLog {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return live.SpendLog{
		RequestID: id, CallType: "chat", Model: "gpt-4o-mini", APIKey: hash,
		Prompt: 8, Completion: 2, Spend: cost, SpendValid: true,
		Start: now, End: now, Status: "success", TeamID: teamID, UserID: userID, OrgID: orgID,
	}
}

func openSpendRedis(t *testing.T) (s *gateway.Server, h http.Handler, master, plain, teamID, userID, orgID string) {
	t.Helper()
	redisURL := testRedisURL()
	dbURL := testDatabaseURL(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl_batch", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	st, err := store.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	cfg := &config.Config{
		ModelList: []config.ModelEntry{{
			ModelName:     "gpt-4o-mini",
			LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "sk-upstream", "api_base": up.URL},
		}},
		RouterSettings:  config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 0, Timeout: 15},
		GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: dbURL, RedisURL: redisURL},
	}
	s = gateway.New(cfg, st)
	if s.Live == nil {
		t.Fatal("redis client was not opened")
	}
	t.Cleanup(func() { _ = s.Live.Close() })
	if err := s.Live.ClearSpendQueue(); err != nil {
		t.Fatal(err)
	}
	h = s.Handler()
	master = "sk-master"
	org := doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "hot-org"})
	if org.Code != 200 {
		t.Fatalf("org %d %s", org.Code, org.Body.String())
	}
	orgID = str(decodeBody(t, org.Body.Bytes())["organization_id"])
	user := doJSON(t, h, "POST", "/user/new", master, map[string]any{"user_email": "hot@xhub.dev", "user_role": "internal_user"})
	if user.Code != 200 {
		t.Fatalf("user %d %s", user.Code, user.Body.String())
	}
	userID = str(decodeBody(t, user.Body.Bytes())["user_id"])
	team := doJSON(t, h, "POST", "/team/new", master, map[string]any{"team_alias": "hot-team", "organization_id": orgID})
	if team.Code != 200 {
		t.Fatalf("team %d %s", team.Code, team.Body.String())
	}
	teamID = str(decodeBody(t, team.Body.Bytes())["team_id"])
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "models": []any{"gpt-4o-mini"},
		"team_id": teamID, "user_id": userID, "organization_id": orgID,
	})
	if gen.Code != 200 {
		t.Fatalf("key %d %s", gen.Code, gen.Body.String())
	}
	plain = str(decodeBody(t, gen.Body.Bytes())["key"])
	return s, h, master, plain, teamID, userID, orgID
}

// 控制台组织没有 spend 列。两路同时刷盘时，extra_json 里的花费应是两次增量之和。
func TestControlOrgSpendConcurrentDeltas(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	if _, err := st.DB.Exec(`DROP TABLE organizations`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`CREATE TABLE organizations (
		id text PRIMARY KEY,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		extra_json text
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO organizations (id, name, extra_json) VALUES ('org_race', 'race', '{"keep":"yes","spend":1}')`); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	run := func(tag string, delta float64) {
		defer wg.Done()
		for i := 0; i < n; i++ {
			err := st.ApplySpendBatch([]store.SpendLogRow{{
				RequestID: fmt.Sprintf("%s-%d", tag, i),
				Spend:     sql.NullFloat64{Float64: delta, Valid: true},
				OrgID:     "org_race",
				Start:     time.Now().UTC(),
				End:       time.Now().UTC(),
			}})
			if err != nil {
				errCh <- err
				return
			}
		}
	}
	wg.Add(2)
	go run("a", 1)
	go run("b", 2)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	got, err := st.GetOrg("org_race")
	if err != nil {
		t.Fatal(err)
	}
	if !nearSpend(got.Spend, 1+float64(n)*1+float64(n)*2) {
		t.Fatalf("spend %v, want %v extra %s", got.Spend, 1+float64(n)*3, got.ExtraJSON)
	}
	if got.Extra()["keep"] != "yes" {
		t.Fatalf("extra lost keep: %s", got.ExtraJSON)
	}
}
